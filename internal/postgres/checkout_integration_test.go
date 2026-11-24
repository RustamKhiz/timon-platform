package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/bot"
	"github.com/RustamKhiz/timon-platform/internal/httpapi"
	"github.com/RustamKhiz/timon-platform/internal/keepgo"
	"github.com/RustamKhiz/timon-platform/internal/sim"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testCheckoutRate struct{}

func (testCheckoutRate) USDToRUB(context.Context) (float64, string, error) {
	return 100, "checkout-test-rate", nil
}

func testTurkeyCheckout(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/bundles" {
			t.Errorf("unexpected Keepgo request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"ack":"success","bundles":[{"id":10,"bundle_type":"country",
			"product_type":"eSIM","coverage":["Turkey"],"refills":[
				{"title":"20 GB / 30 days","amount_mb":20480,"amount_days":30,"price_usd":9.65}
			]}]}`)
	}))
	defer provider.Close()
	keepgoClient, err := keepgo.NewClient(keepgo.Config{
		BaseURL: provider.URL + "/api/v2", APIKey: "fixture-key", AccessToken: "fixture-token", HTTPClient: provider.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := keepgo.NewCatalog(keepgoClient, testCheckoutRate{}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service := sim.NewServiceWithCatalog(sim.PricingConfig{FixedMarkupRUB: 100}, NewStore(pool), catalog)
	api := httptest.NewServer(httpapi.New(service, slog.New(slog.NewTextHandler(io.Discard, nil)), httpapi.Config{BotToken: "checkout-test"}))
	defer api.Close()
	client := bot.NewHTTPClient(api.URL, "checkout-test")
	engine := bot.NewEngine(client)
	const buyer = 123
	act := func(input string) bot.Response {
		t.Helper()
		response, err := engine.Handle(ctx, buyer, input)
		if err != nil {
			t.Fatalf("bot action %q: %v", input, err)
		}
		return response
	}

	options := act("country:TR")
	offer := options.Buttons[0][0]
	if !strings.Contains(offer.Text, "20 ГБ · 30 дней — 1065 ₽") {
		t.Fatalf("Turkey offer: %q", offer.Text)
	}
	quoteCard := act(offer.Data)
	if !strings.Contains(quoteCard.Text, "20 ГБ · 30 дней") || !strings.Contains(quoteCard.Text, "Цена: 1065 ₽") {
		t.Fatalf("quoted package: %q", quoteCard.Text)
	}
	compatibility := act(quoteCard.Buttons[0][0].Data)
	confirm := compatibility.Buttons[2][0].Data
	for i := 0; i < 2; i++ {
		confirmation := act(confirm)
		if !strings.Contains(confirmation.Text, "Тестовый заказ создан") {
			t.Fatalf("order confirmation: %q", confirmation.Text)
		}
	}
	orders, err := client.Orders(ctx, buyer)
	if err != nil || len(orders) != 1 {
		t.Fatalf("expected one test order: %d, %v", len(orders), err)
	}
	order := orders[0]
	if order.OrderStatus != "awaiting_payment" || order.PaymentStatus != "pending" || order.Activation != nil ||
		order.Quote.RetailRUB != 1065 || order.Quote.ProviderID != "keepgo-sandbox" ||
		order.Quote.ProviderBundleID != 10 || order.Quote.Selection.TotalDataMB != 20480 ||
		order.Quote.Selection.DurationDays != 30 || order.Quote.Selection.GigabytesPerDay != 0 {
		t.Fatalf("Keepgo test order did not preserve the quote: %+v", order)
	}

	legacyQuote, err := client.CreateQuote(ctx, sim.Selection{CountryCode: "TR", DurationDays: 3, GigabytesPerDay: 2})
	if err != nil {
		t.Fatal(err)
	}
	legacyOrder, err := client.CreateOrder(ctx, legacyQuote.ID, buyer, "legacy-daily-checkout")
	if err != nil {
		t.Fatal(err)
	}
	if legacyOrder.Quote.Selection.GigabytesPerDay != 2 || legacyOrder.Quote.Selection.TotalDataMB != 0 ||
		legacyOrder.Quote.Selection.PackageID != "" || legacyOrder.Quote.RetailRUB != 352 {
		t.Fatalf("legacy daily tariff changed: %+v", legacyOrder.Quote)
	}

	for _, invalid := range []struct {
		id  string
		sql string
	}{
		{legacyQuote.ID, `UPDATE sim_quotes SET gigabytes_per_day = 0 WHERE id = $1`},
		{order.Quote.ID, `UPDATE sim_quotes SET total_data_mb = 0 WHERE id = $1`},
		{order.Quote.ID, `UPDATE sim_quotes SET gigabytes_per_day = 1 WHERE id = $1`},
	} {
		_, err := pool.Exec(ctx, invalid.sql, invalid.id)
		var check *pgconn.PgError
		if !errors.As(err, &check) || check.ConstraintName != "sim_quotes_traffic_mode_check" {
			t.Fatalf("database accepted invalid traffic mode or reported unexpected error: %v", err)
		}
	}
	t.Log("Turkey Keepgo fixture: 20 GB / 30 days shown at 1065 RUB; one unpaid order saved; legacy 2 GB/day remains 352 RUB")
}
