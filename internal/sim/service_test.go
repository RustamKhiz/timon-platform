package sim

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeCatalog struct {
	packages []Package
}

func (f fakeCatalog) Packages(context.Context, string) ([]Package, error) { return f.packages, nil }
func (f fakeCatalog) Package(_ context.Context, id string) (Package, error) {
	for _, pkg := range f.packages {
		if pkg.ID == id {
			return pkg, nil
		}
	}
	return Package{}, ErrNotFound
}

func TestCreateQuoteUsesApprovedRangesAndPricingSnapshot(t *testing.T) {
	service := NewService(PricingConfig{FixedMarkupRUB: 100, MinimumMargin: 50, QuoteTTL: 15 * time.Minute})
	ctx := context.Background()
	quote, err := service.CreateQuote(ctx, Selection{CountryCode: "TR", DurationDays: 10, GigabytesPerDay: 2})
	if err != nil {
		t.Fatalf("CreateQuote() error = %v", err)
	}
	if quote.ProviderCostRUB != 840 || quote.RetailRUB != 940 || quote.FixedMarkupRUB != 100 {
		t.Fatalf("unexpected pricing snapshot: %+v", quote)
	}
	if got := quote.ExpiresAt.Sub(quote.CreatedAt); got != 15*time.Minute {
		t.Fatalf("quote TTL = %s, want 15m", got)
	}

	invalid := []Selection{
		{CountryCode: "US", DurationDays: 1, GigabytesPerDay: 1},
		{CountryCode: "TR", DurationDays: 0, GigabytesPerDay: 1},
		{CountryCode: "TR", DurationDays: 31, GigabytesPerDay: 1},
		{CountryCode: "TR", DurationDays: 1, GigabytesPerDay: 0},
		{CountryCode: "TR", DurationDays: 1, GigabytesPerDay: 6},
	}
	for _, selection := range invalid {
		if _, err := service.CreateQuote(ctx, selection); !errors.Is(err, ErrInvalidSelection) {
			t.Errorf("CreateQuote(%+v) error = %v, want ErrInvalidSelection", selection, err)
		}
	}
}

func TestOrderIsIdempotentAndMockProvisioningKeepsSnapshots(t *testing.T) {
	service := NewService(PricingConfig{FixedMarkupRUB: 100})
	ctx := context.Background()
	quote, err := service.CreateQuote(ctx, Selection{CountryCode: "AE", DurationDays: 7, GigabytesPerDay: 3})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.CreateOrder(ctx, quote.ID, 123, "same-request")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateOrder(ctx, quote.ID, 123, "same-request")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent create produced %q and %q", first.ID, second.ID)
	}
	otherUserOrder, err := service.CreateOrder(ctx, quote.ID, 456, "same-request")
	if err != nil {
		t.Fatal(err)
	}
	if otherUserOrder.ID == first.ID {
		t.Fatal("idempotency key must be scoped to the Telegram user")
	}
	otherQuote, err := service.CreateQuote(ctx, Selection{CountryCode: "TR", DurationDays: 2, GigabytesPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateOrder(ctx, otherQuote.ID, 123, "same-request"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting request error = %v", err)
	}

	order, err := service.MockPayAndProvision(ctx, first.ID, 123)
	if err != nil {
		t.Fatal(err)
	}
	if order.PaymentStatus != "succeeded" || order.FulfillmentStatus != "provisioned" || order.Activation == nil || !order.Activation.IsMock {
		t.Fatalf("unexpected provisioned order: %+v", order)
	}
	if order.Quote.RetailRUB != quote.RetailRUB || order.FulfillmentDeadlineAt.Sub(*order.PaidAt) != 24*time.Hour {
		t.Fatal("pricing snapshot or fulfillment deadline changed")
	}
	delivered, err := service.MarkDelivered(ctx, order.ID, 123)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.OrderStatus != "completed" || delivered.DeliveryStatus != "sent" {
		t.Fatalf("unexpected delivered order: %+v", delivered)
	}
}

func TestExpiredQuoteCannotBecomeOrder(t *testing.T) {
	service := NewService(PricingConfig{FixedMarkupRUB: 100})
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	quote, err := service.CreateQuote(ctx, Selection{CountryCode: "CN", DurationDays: 1, GigabytesPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return quote.ExpiresAt }
	if _, err := service.CreateOrder(ctx, quote.ID, 123, "expired"); !errors.Is(err, ErrQuoteExpired) {
		t.Fatalf("CreateOrder() error = %v, want ErrQuoteExpired", err)
	}
}

func TestProviderPackagePricingRaisesMarkupToProtectMinimumMargin(t *testing.T) {
	pkg := Package{ID: "keepgo:228:data:20480:30", CountryCode: "IN", Name: "20 GB / 30 days", DataMB: 20480, ValidityDays: 30, ProviderID: "keepgo-sandbox", ProviderBundleID: 228, ProviderPriceUSDCents: 3259, FXRateRUBPerUSD: 84.5572, ProviderRateVersion: "snapshot"}
	service := NewServiceWithCatalog(PricingConfig{FixedMarkupRUB: 100, ExpectedFeeBPS: 350, MinimumMargin: 50}, nil, fakeCatalog{packages: []Package{pkg}})
	packages, err := service.Packages(context.Background(), "IN")
	if err != nil || len(packages) != 1 {
		t.Fatalf("Packages() = %+v, %v", packages, err)
	}
	if packages[0].ProviderCostRUB != 2756 || packages[0].RetailRUB != 2908 || packages[0].AppliedMarkupRUB != 152 || packages[0].ExpectedFeeRUB != 102 {
		t.Fatalf("unexpected protected price: %+v", packages[0])
	}
	quote, err := service.CreateQuote(context.Background(), Selection{CountryCode: "IN", PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Selection.PackageName != pkg.Name || quote.ProviderBundleID != 228 || quote.ProviderPriceUSDCents != 3259 || quote.RetailRUB != 2908 {
		t.Fatalf("unexpected package quote snapshot: %+v", quote)
	}
}
