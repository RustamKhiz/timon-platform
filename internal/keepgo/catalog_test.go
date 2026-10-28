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

func TestNormalizePackagesKeepsOnlyMVPPackageLine(t *testing.T) {
	day := 1
	week := 7
	month := 30
	bundles := []Bundle{{
		ID: 10, BundleType: "country", ProductType: "eSIM", Coverage: []string{"Turkey"},
		Refills: []Refill{
			{Title: "1 GB / 1 day", AmountMB: 1024, AmountDays: &day, PriceUSD: 0.74},
			{Title: "1 GB / 7 days", AmountMB: 1024, AmountDays: &week, PriceUSD: 0.82},
			{Title: "5 GB / 30 days", AmountMB: 5 * 1024, AmountDays: &month, PriceUSD: 3.18},
			{Title: "10 GB / 30 days", AmountMB: 10 * 1024, AmountDays: &month, PriceUSD: 5.53},
			{Title: "20 GB / 30 days", AmountMB: 20 * 1024, AmountDays: &month, PriceUSD: 9.65},
			{Title: "Unlimited / 7 days", AmountMB: 1024, AmountDays: &week, PriceUSD: 16.76},
		},
	}}
	items := normalizePackages(bundles, 84.9, "test")
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	for _, item := range items {
		if item.ValidityDays != 30 || (item.DataMB != 5*1024 && item.DataMB != 10*1024 && item.DataMB != 20*1024) {
			t.Fatalf("unexpected package: %+v", item)
		}
	}
}
