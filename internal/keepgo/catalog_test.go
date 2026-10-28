package keepgo

import "testing"

func TestNormalizePackagesChoosesCheapestCountryESIM(t *testing.T) {
	days := 30
	bundles := []Bundle{
		{ID: 1, BundleType: "country", ProductType: "eSIM", Coverage: []string{"Turkey"}, Refills: []Refill{{Title: "5 GB / 30 days", AmountMB: 5120, AmountDays: &days, PriceUSD: 9.76}}},
		{ID: 2, BundleType: "country", ProductType: "eSIM", Coverage: []string{"Turkey"}, Refills: []Refill{{Title: "5 GB / 30 days", AmountMB: 5120, AmountDays: &days, PriceUSD: 3.18}}},
		{ID: 3, BundleType: "country", ProductType: "pSIM", Coverage: []string{"Turkey"}, Refills: []Refill{{Title: "5 GB / 30 days", AmountMB: 5120, AmountDays: &days, PriceUSD: 1}}},
	}
	items := normalizePackages(bundles, 84.5572, "test")
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	for _, item := range items {
		if item.ProviderBundleID != 2 || item.ProviderPriceUSDCents != 318 || item.CountryCode != "TR" {
			t.Fatalf("unexpected package: %+v", item)
		}
	}
}
