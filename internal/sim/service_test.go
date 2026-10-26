package sim

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
