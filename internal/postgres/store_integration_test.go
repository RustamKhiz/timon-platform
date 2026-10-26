package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/RustamKhiz/timon-platform/internal/sim"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRestartAndIdempotency(t *testing.T) {
	databaseURL := os.Getenv("SIM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SIM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE sim_outbox, sim_orders, sim_quotes RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	firstService := sim.NewServiceWithRepository(sim.PricingConfig{FixedMarkupRUB: 100}, NewStore(pool))
	quote, err := firstService.CreateQuote(ctx, sim.Selection{CountryCode: "TR", DurationDays: 3, GigabytesPerDay: 2})
	if err != nil {
		t.Fatal(err)
	}
	firstOrder, err := firstService.CreateOrder(ctx, quote.ID, 42, "restart-safe-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstService.MockPayAndProvision(ctx, firstOrder.ID, 42); err != nil {
		t.Fatal(err)
	}

	restartedService := sim.NewServiceWithRepository(sim.PricingConfig{FixedMarkupRUB: 100}, NewStore(pool))
	secondOrder, err := restartedService.CreateOrder(ctx, quote.ID, 42, "restart-safe-key")
	if err != nil {
		t.Fatal(err)
	}
	if secondOrder.ID != firstOrder.ID || secondOrder.Activation == nil || !secondOrder.Activation.IsMock {
		t.Fatalf("persisted order = %+v, want id %q with mock activation", secondOrder, firstOrder.ID)
	}
	otherUserOrder, err := restartedService.CreateOrder(ctx, quote.ID, 43, "restart-safe-key")
	if err != nil {
		t.Fatal(err)
	}
	if otherUserOrder.ID == firstOrder.ID {
		t.Fatal("idempotency key must be scoped to the Telegram user")
	}
	orders, err := restartedService.OrdersForUser(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 {
		t.Fatalf("orders after restart = %d, want 1", len(orders))
	}
	var outboxState string
	if err := pool.QueryRow(ctx, `SELECT state FROM sim_outbox WHERE event_type = 'deliver_esim' AND entity_id = $1`, firstOrder.ID).Scan(&outboxState); err != nil {
		t.Fatal(err)
	}
	if outboxState != "pending" {
		t.Fatalf("outbox state = %q, want pending", outboxState)
	}
}
