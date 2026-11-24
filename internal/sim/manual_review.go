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
	Actor          string    `json:"actor"`
	Reason         string    `json:"reason"`
	PreviousStatus string    `json:"previous_status"`
	CreatedAt      time.Time `json:"created_at"`
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
	if event, ok := s.reviews[id]; ok && event.Actor == actor && event.Reason == reason {
		order.ManualReview = &event
		return order, nil
	}
	if !order.CanRequestManualReview() {
		return Order{}, ErrReviewConflict
	}
	event := ManualReviewEvent{Actor: actor, Reason: reason, PreviousStatus: order.OrderStatus, CreatedAt: now}
	order.OrderStatus = "manual_review"
	s.orders[id] = order
	if s.reviews == nil {
		s.reviews = make(map[string]ManualReviewEvent)
	}
	s.reviews[id] = event
	order.ManualReview = &event
	return order, nil
}
