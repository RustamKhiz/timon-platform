package fx

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

const CBRDailyURL = "https://www.cbr.ru/scripts/XML_daily_eng.asp"

type Config struct {
	Endpoint     string
	BufferBPS    int
	FallbackRate float64
	CacheTTL     time.Duration
	HTTPClient   *http.Client
}

type CBRUSDProvider struct {
	endpoint     string
	bufferBPS    int
	fallbackRate float64
	cacheTTL     time.Duration
	httpClient   *http.Client
	now          func() time.Time

	mu       sync.Mutex
	cachedAt time.Time
	rate     float64
	version  string
}

type dailyRates struct {
	Date    string   `xml:"Date,attr"`
	Valutes []valute `xml:"Valute"`
}

type valute struct {
	CharCode string `xml:"CharCode"`
	Nominal  int    `xml:"Nominal"`
	Value    string `xml:"Value"`
}

func NewCBRUSDProvider(config Config) (*CBRUSDProvider, error) {
	if config.BufferBPS < 0 || config.BufferBPS >= 10000 {
		return nil, errors.New("CBR FX buffer must be between 0 and 9999 basis points")
	}
	if config.FallbackRate < 0 {
		return nil, errors.New("CBR FX fallback rate cannot be negative")
	}
	if config.Endpoint == "" {
		config.Endpoint = CBRDailyURL
	}
	if !strings.HasPrefix(config.Endpoint, "https://") {
		return nil, errors.New("CBR FX endpoint must use HTTPS")
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 6 * time.Hour
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &CBRUSDProvider{endpoint: config.Endpoint, bufferBPS: config.BufferBPS, fallbackRate: config.FallbackRate, cacheTTL: config.CacheTTL, httpClient: config.HTTPClient, now: time.Now}, nil
}

func (p *CBRUSDProvider) USDToRUB(ctx context.Context) (float64, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now().UTC()
	if p.rate > 0 && now.Sub(p.cachedAt) < p.cacheTTL {
		return p.rate, p.version, nil
	}
	rate, version, err := p.fetch(ctx)
	if err != nil {
		if p.rate > 0 && now.Sub(p.cachedAt) < 72*time.Hour {
			return p.rate, p.version + ":stale", nil
		}
		if p.fallbackRate > 0 {
			return p.fallbackRate, "configured-fallback", nil
		}
		return 0, "", err
	}
	p.rate = rate
	p.version = version
	p.cachedAt = now
	return rate, version, nil
}

func (p *CBRUSDProvider) fetch(ctx context.Context) (float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return 0, "", fmt.Errorf("build CBR FX request: %w", err)
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", "TIMON-SIM/1.0 (+https://github.com/RustamKhiz/timon-platform)")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("call CBR FX endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("CBR FX endpoint returned HTTP %d", resp.StatusCode)
	}
	var document dailyRates
	decoder := xml.NewDecoder(transform.NewReader(resp.Body, charmap.Windows1251.NewDecoder()))
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	if err := decoder.Decode(&document); err != nil {
		return 0, "", fmt.Errorf("decode CBR FX response: %w", err)
	}
	for _, item := range document.Valutes {
		if item.CharCode != "USD" || item.Nominal <= 0 {
			continue
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(item.Value, ",", "."), 64)
		if err != nil || value <= 0 {
			return 0, "", errors.New("CBR returned an invalid USD rate")
		}
		base := value / float64(item.Nominal)
		buffered := base * float64(10000+p.bufferBPS) / 10000
		return buffered, fmt.Sprintf("cbr:%s:+%dbps", document.Date, p.bufferBPS), nil
	}
	return 0, "", errors.New("CBR response contains no USD rate")
}
