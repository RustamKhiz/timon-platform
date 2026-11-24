package sim

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	ErrResendUnavailable = errors.New("instruction resend requires durable storage")
	ErrResendConflict    = errors.New("instruction resend is not available for this order or another send is pending")
	ErrInvalidResend     = errors.New("invalid resend request")
	resendIDPattern      = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
)

type InstructionResend struct {
	ID         string     `json:"id"`
	Actor      string     `json:"actor"`
	State      string     `json:"state"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type InstructionDelivery struct {
	ID    string `json:"id"`
	Order Order  `json:"order"`
}

type instructionResendStore interface {
	QueueInstructionResend(context.Context, string, string, string, time.Time) error
	ClaimInstructionResend(context.Context, time.Time) (*InstructionDelivery, error)
	FinishInstructionResend(context.Context, string, string, time.Time) error
}

func (o Order) CanResendInstruction() bool {
	return o.OrderStatus == "completed" && o.PaymentStatus == "succeeded" &&
		o.FulfillmentStatus == "provisioned" && o.DeliveryStatus == "sent" &&
		o.TelegramUserID > 0 && o.Activation != nil && o.Activation.IsMock
}

func (s *Service) SupportsInstructionResend() bool {
	_, ok := s.store.(instructionResendStore)
	return ok
}

func (s *Service) QueueInstructionResend(ctx context.Context, id, actor, key string) error {
	if !resendIDPattern.MatchString(key) {
		return ErrInvalidResend
	}
	store, ok := s.store.(instructionResendStore)
	if !ok {
		return ErrResendUnavailable
	}
	return store.QueueInstructionResend(ctx, id, actor, key, s.now().UTC())
}

func (s *Service) ClaimInstructionResend(ctx context.Context) (*InstructionDelivery, error) {
	store, ok := s.store.(instructionResendStore)
	if !ok {
		return nil, nil
	}
	return store.ClaimInstructionResend(ctx, s.now().UTC())
}

func (s *Service) FinishInstructionResend(ctx context.Context, id, state string) error {
	if !resendIDPattern.MatchString(id) || (state != "sent" && state != "failed" && state != "unknown") {
		return ErrInvalidResend
	}
	store, ok := s.store.(instructionResendStore)
	if !ok {
		return ErrResendUnavailable
	}
	return store.FinishInstructionResend(ctx, id, state, s.now().UTC())
}
