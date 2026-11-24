package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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

	if _, err := firstService.RequestManualReview(ctx, firstOrder.ID, "owner", "payment_check"); !errors.Is(err, sim.ErrReviewConflict) {
		t.Fatalf("review of a provisioned order = %v, want conflict", err)
	}
	reviewed, err := firstService.RequestManualReview(ctx, otherUserOrder.ID, "owner", "payment_check")
	if err != nil || reviewed.OrderStatus != "manual_review" || reviewed.ManualReview == nil {
		t.Fatalf("reviewed order = %+v, %v", reviewed, err)
	}
	retry, err := restartedService.RequestManualReview(ctx, otherUserOrder.ID, "owner", "payment_check")
	if err != nil || retry.ManualReview == nil || *retry.ManualReview != *reviewed.ManualReview {
		t.Fatalf("retried review = %+v, %v", retry, err)
	}
	detail, err := restartedService.AdminOrder(ctx, otherUserOrder.ID)
	if err != nil || detail.ManualReview == nil || *detail.ManualReview != *reviewed.ManualReview {
		t.Fatalf("persisted review = %+v, %v", detail, err)
	}
	if _, err := restartedService.MockPayAndProvision(ctx, otherUserOrder.ID, 43); !errors.Is(err, sim.ErrReviewConflict) {
		t.Fatalf("mock payment during manual review = %v, want conflict", err)
	}

	reviewID := reviewed.ManualReview.ID
	if err := restartedService.ReleaseManualReview(ctx, otherUserOrder.ID, "owner", reviewID); err != nil {
		t.Fatal(err)
	}
	detail, err = firstService.AdminOrder(ctx, otherUserOrder.ID)
	if err != nil || detail.OrderStatus != "awaiting_payment" || detail.PaymentStatus != "pending" ||
		detail.ManualReview != nil || len(detail.ManualReviews) != 1 ||
		detail.ManualReviews[0].ReleasedAt == nil || detail.ManualReviews[0].ReleasedBy != "owner" {
		t.Fatalf("released order = %+v, %v", detail, err)
	}
	newReview, err := firstService.RequestManualReview(ctx, otherUserOrder.ID, "owner", "payment_check")
	if err != nil || newReview.ManualReview == nil || newReview.ManualReview.ID == reviewID {
		t.Fatalf("second review = %+v, %v", newReview, err)
	}
	if err := restartedService.ReleaseManualReview(ctx, otherUserOrder.ID, "owner", reviewID); err != nil {
		t.Fatal(err)
	}
	detail, err = restartedService.AdminOrder(ctx, otherUserOrder.ID)
	if err != nil || detail.OrderStatus != "manual_review" || len(detail.ManualReviews) != 2 ||
		detail.ManualReview == nil || detail.ManualReview.ID != newReview.ManualReview.ID {
		t.Fatalf("stale release changed the active review: %+v, %v", detail, err)
	}
	if err := restartedService.ReleaseManualReview(ctx, otherUserOrder.ID, "owner", newReview.ManualReview.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := restartedService.MockPayAndProvision(ctx, otherUserOrder.ID, 43); err != nil {
		t.Fatalf("mock payment after release: %v", err)
	}

	t.Run("instruction resends", func(t *testing.T) {
		testInstructionResends(t, ctx, pool, firstService, restartedService, firstOrder.ID)
	})
}

func testInstructionResends(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service, restarted *sim.Service, orderID string) {
	t.Helper()
	const firstID = "22d03db0-31e0-4f55-8bb8-d57d133cfeaf"
	const secondID = "660320b9-827a-46e9-af74-b490b1cbb1a3"
	if err := service.QueueInstructionResend(ctx, orderID, "owner", firstID); !errors.Is(err, sim.ErrResendConflict) {
		t.Fatalf("resend before first delivery = %v", err)
	}
	if _, err := service.MarkDelivered(ctx, orderID, 42); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []*sim.Service{service, restarted} {
		if err := attempt.QueueInstructionResend(ctx, orderID, "owner", firstID); err != nil {
			t.Fatal(err)
		}
	}
	if err := restarted.QueueInstructionResend(ctx, orderID, "owner", secondID); !errors.Is(err, sim.ErrResendConflict) {
		t.Fatalf("parallel send should be refused: %v", err)
	}
	job, err := restarted.ClaimInstructionResend(ctx)
	if err != nil || job == nil || job.ID != firstID || job.Order.ID != orderID || job.Order.TelegramUserID != 42 {
		t.Fatalf("claimed job = %+v, %v", job, err)
	}
	if job, err := service.ClaimInstructionResend(ctx); err != nil || job != nil {
		t.Fatalf("still-leased job was claimed: %+v, %v", job, err)
	}
	if err := restarted.FinishInstructionResend(ctx, firstID, "sent"); err != nil {
		t.Fatal(err)
	}
	detail, err := service.AdminOrder(ctx, orderID)
	if err != nil || detail.OrderStatus != "completed" || len(detail.InstructionResends) != 1 ||
		detail.InstructionResends[0].State != "sent" || detail.InstructionResends[0].FinishedAt == nil {
		t.Fatalf("sent status missing from admin order: %+v, %v", detail, err)
	}
	userOrder, err := restarted.GetOrder(ctx, orderID, 42)
	if err != nil || len(userOrder.InstructionResends) != 0 {
		t.Fatalf("bot order exposed admin history: %+v, %v", userOrder, err)
	}

	store := NewStore(pool)
	later := time.Now().Add(2 * time.Minute)
	if err := store.QueueInstructionResend(ctx, orderID, "owner", secondID, later); err != nil {
		t.Fatal(err)
	}
	job, err = store.ClaimInstructionResend(ctx, later)
	if err != nil || job == nil || job.ID != secondID {
		t.Fatalf("claimed second job = %+v, %v", job, err)
	}
	if job, err := store.ClaimInstructionResend(ctx, later.Add(2*time.Minute)); err != nil || job != nil {
		t.Fatalf("expired send must not be repeated: %+v, %v", job, err)
	}
	detail, err = service.AdminOrder(ctx, orderID)
	if err != nil || len(detail.InstructionResends) != 2 || detail.InstructionResends[0].State != "unknown" {
		t.Fatalf("expired send outcome missing: %+v, %v", detail, err)
	}
}
