package keepgo

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

const sandboxProviderID = "keepgo-sandbox"

var countryNames = map[string]string{
	"TR": "Turkey",
	"EG": "Egypt",
	"AE": "United Arab Emirates",
	"IN": "India",
	"TH": "Thailand",
	"VN": "Vietnam",
	"CN": "China",
}

type Catalog struct {
	client     *Client
	fx         FXRateProvider
	ttl        time.Duration
	now        func() time.Time
	mu         sync.Mutex
	loaded     time.Time
	version    string
	items      map[string]sim.Package
	refreshing bool
}

type FXRateProvider interface {
	USDToRUB(context.Context) (float64, string, error)
}

func NewCatalog(client *Client, fx FXRateProvider, ttl time.Duration) (*Catalog, error) {
	if client == nil {
		return nil, fmt.Errorf("Keepgo client is required")
	}
	if fx == nil {
		return nil, fmt.Errorf("Keepgo FX rate provider is required")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Catalog{client: client, fx: fx, ttl: ttl, now: time.Now, items: make(map[string]sim.Package)}, nil
}

func (c *Catalog) Packages(ctx context.Context, countryCode string) ([]sim.Package, error) {
	if _, ok := countryNames[countryCode]; !ok {
		return nil, sim.ErrInvalidSelection
	}
	if err := c.refresh(ctx, false); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]sim.Package, 0)
	for _, pkg := range c.items {
		if pkg.CountryCode == countryCode {
			result = append(result, pkg)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Unlimited != result[j].Unlimited {
			return !result[i].Unlimited
		}
		if result[i].ValidityDays != result[j].ValidityDays {
			return result[i].ValidityDays < result[j].ValidityDays
		}
		if result[i].DataMB != result[j].DataMB {
			return result[i].DataMB < result[j].DataMB
		}
		return result[i].ProviderPriceUSDCents < result[j].ProviderPriceUSDCents
	})
	return result, nil
}

func (c *Catalog) Package(ctx context.Context, packageID string) (sim.Package, error) {
	if err := c.refresh(ctx, false); err != nil {
		return sim.Package{}, err
	}
	c.mu.Lock()
	pkg, ok := c.items[packageID]
	c.mu.Unlock()
	if ok {
		return pkg, nil
	}
	if err := c.refresh(ctx, true); err != nil {
		return sim.Package{}, err
	}
	c.mu.Lock()
	pkg, ok = c.items[packageID]
	c.mu.Unlock()
	if !ok {
		return sim.Package{}, sim.ErrNotFound
	}
	return pkg, nil
}

func (c *Catalog) refresh(ctx context.Context, force bool) error {
	c.mu.Lock()
	now := c.now().UTC()
	if !force && len(c.items) > 0 && now.Sub(c.loaded) < c.ttl {
		c.mu.Unlock()
		return nil
	}
	if !force && len(c.items) > 0 {
		if !c.refreshing {
			c.refreshing = true
			go c.refreshInBackground()
		}
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	return c.load(ctx)
}

func (c *Catalog) refreshInBackground() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = c.load(ctx)
	c.mu.Lock()
	c.refreshing = false
	c.mu.Unlock()
}

func (c *Catalog) load(ctx context.Context) error {
	now := c.now().UTC()
	response, err := c.client.Bundles(ctx, true)
	if err != nil {
		return err
	}
	fxRate, fxVersion, err := c.fx.USDToRUB(ctx)
	if err != nil {
		return fmt.Errorf("load USD/RUB quote rate: %w", err)
	}
	version := "keepgo-sandbox:" + now.Format(time.RFC3339) + ":" + fxVersion
	items := normalizePackages(response.Bundles, fxRate, version)
	if len(items) == 0 {
		return fmt.Errorf("Keepgo catalogue contains no supported country packages")
	}
	c.mu.Lock()
	c.items = items
	c.loaded = now
	c.version = version
	c.mu.Unlock()
	return nil
}

func normalizePackages(bundles []Bundle, fxRate float64, version string) map[string]sim.Package {
	result := make(map[string]sim.Package)
	byOffer := make(map[string]string)
	for _, bundle := range bundles {
		if bundle.BundleType != "country" || bundle.ProductType != "eSIM" || len(bundle.Coverage) != 1 {
			continue
		}
		countryCode := codeForCountry(bundle.Coverage[0])
		if countryCode == "" {
			continue
		}
		for _, refill := range bundle.Refills {
			if refill.AmountDays == nil || *refill.AmountDays < 1 || *refill.AmountDays > 30 || refill.PriceUSD <= 0 {
				continue
			}
			unlimited := strings.HasPrefix(strings.ToLower(refill.Title), "unlimited")
			if !unlimited && refill.AmountMB < 1024 {
				continue
			}
			priceCents := int(math.Round(refill.PriceUSD * 100))
			typeKey := "data"
			if unlimited {
				typeKey = "unlimited"
			}
			offerKey := fmt.Sprintf("%s:%s:%d:%d", countryCode, typeKey, refill.AmountMB, *refill.AmountDays)
			id := fmt.Sprintf("keepgo:%d:%s:%d:%d", bundle.ID, typeKey, refill.AmountMB, *refill.AmountDays)
			name := refill.Title
			if unlimited {
				name = fmt.Sprintf("Безлимит / %d дн. — 1 ГБ/день на полной скорости", *refill.AmountDays)
			}
			pkg := sim.Package{ID: id, CountryCode: countryCode, Name: name, DataMB: refill.AmountMB, ValidityDays: *refill.AmountDays, Unlimited: unlimited, ProviderID: sandboxProviderID, ProviderBundleID: bundle.ID, ProviderPriceUSDCents: priceCents, FXRateRUBPerUSD: fxRate, ProviderRateVersion: version}
			if previousID, ok := byOffer[offerKey]; ok {
				if result[previousID].ProviderPriceUSDCents <= priceCents {
					continue
				}
				delete(result, previousID)
			}
			byOffer[offerKey] = id
			result[id] = pkg
		}
	}
	return result
}

func codeForCountry(name string) string {
	for code, countryName := range countryNames {
		if countryName == name {
			return code
		}
	}
	return ""
}
