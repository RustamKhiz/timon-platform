package sim

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidReviewReason = errors.New("invalid manual review reason")
	ErrReviewConflict      = errors.New("order cannot be moved to manual review")
)

type ManualReviewEvent struct {
	ID             int64      `json:"id"`
	Actor          string     `json:"actor"`
	Reason         string     `json:"reason"`
	PreviousStatus string     `json:"previous_status"`
	CreatedAt      time.Time  `json:"created_at"`
	ReleasedBy     string     `json:"released_by,omitempty"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
}

func (o Order) CanReleaseManualReview() bool {
	return o.OrderStatus == "manual_review" && o.ManualReview != nil &&
		o.ManualReview.ReleasedAt == nil && o.ManualReview.PreviousStatus == "awaiting_payment" &&
		o.PaymentStatus == "pending" && o.FulfillmentStatus == "not_started" && o.DeliveryStatus == "not_ready"
}

func (s *Service) ReleaseManualReview(ctx context.Context, id, actor string, reviewID int64) error {
	if reviewID <= 0 {
		return ErrReviewConflict
	}
	return s.store.ReleaseManualReview(ctx, id, actor, reviewID, s.now().UTC())
}

func (o Order) CanRequestManualReview() bool {
	return o.OrderStatus == "awaiting_payment" && o.PaymentStatus == "pending" &&
		o.FulfillmentStatus == "not_started" && o.DeliveryStatus == "not_ready"
}

func (s *Service) RequestManualReview(ctx context.Context, id, actor, reason string) (Order, error) {
	switch reason {
	case "payment_check", "order_data_check", "support_request":
		return s.store.RequestManualReview(ctx, id, actor, reason, s.now().UTC())
	default:
		return Order{}, ErrInvalidReviewReason
	}
}

func (s *memoryStore) RequestManualReview(ctx context.Context, id, actor, reason string, now time.Time) (Order, error) {
	if err := ctx.Err(); err != nil {
		return Order{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	history := s.reviews[id]
	order.ManualReview = activeReview(history)
	if event := order.ManualReview; order.OrderStatus == "manual_review" && event != nil && event.Actor == actor && event.Reason == reason {
		return order, nil
	}
	if !order.CanRequestManualReview() {
		return Order{}, ErrReviewConflict
	}
	event := ManualReviewEvent{ID: int64(len(history) + 1), Actor: actor, Reason: reason, PreviousStatus: order.OrderStatus, CreatedAt: now}
	order.OrderStatus = "manual_review"
	order.ManualReview = nil
	s.orders[id] = order
	if s.reviews == nil {
		s.reviews = make(map[string][]ManualReviewEvent)
	}
	s.reviews[id] = append(history, event)
	order.ManualReview = &event
	return order, nil
}

func activeReview(history []ManualReviewEvent) *ManualReviewEvent {
	if len(history) > 0 && history[len(history)-1].ReleasedAt == nil {
		event := history[len(history)-1]
		return &event
	}
	return nil
}

func (s *memoryStore) ReleaseManualReview(ctx context.Context, id, actor string, reviewID int64, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[id]
	if !ok {
		return ErrNotFound
	}
	history := s.reviews[id]
	if reviewID <= 0 || reviewID > int64(len(history)) {
		return ErrReviewConflict
	}
	event := &history[reviewID-1]
	if event.ReleasedAt != nil {
		return nil
	}
	order.ManualReview = event
	if !order.CanReleaseManualReview() {
		return ErrReviewConflict
	}
	order.OrderStatus = event.PreviousStatus
	order.ManualReview = nil
	s.orders[id] = order
	event.ReleasedBy, event.ReleasedAt = actor, &now
	return nil
}
