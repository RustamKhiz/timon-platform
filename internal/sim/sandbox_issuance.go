package sim

import (
	"context"
	"errors"
	"time"
)

const KeepgoSandboxProviderID = "keepgo-sandbox"

var ErrSandboxIssuance = errors.New("sandbox eSIM issuance unavailable")

type SandboxIssuer interface {
	IssueSandboxLine(context.Context, Quote) (SandboxLine, error)
}

type SandboxLine struct {
	ICCID      string
	Activation ActivationData
}

type sandboxStore interface {
	BeginKeepgoSandboxIssuance(context.Context, string, int64, time.Time) (Order, bool, error)
	FinishKeepgoSandboxIssuance(context.Context, string, SandboxLine) (Order, error)
	ReviewKeepgoSandboxIssuance(context.Context, string) error
}

func (s *Service) EnableSandboxIssuance(issuer SandboxIssuer) error {
	if _, ok := s.store.(sandboxStore); !ok || issuer == nil {
		return ErrSandboxIssuance
	}
	s.sandbox = issuer
	return nil
}

func (s *Service) SandboxIssuanceEnabled() bool { return s.sandbox != nil }

func (s *Service) ConfirmTestPayment(ctx context.Context, orderID string, userID int64) (Order, error) {
	if s.sandbox == nil {
		return s.MockPayAndProvision(ctx, orderID, userID)
	}
	order, err := s.GetOrder(ctx, orderID, userID)
	if err != nil {
		return Order{}, err
	}
	if order.Quote.ProviderID != KeepgoSandboxProviderID {
		return s.MockPayAndProvision(ctx, orderID, userID)
	}
	store, ok := s.store.(sandboxStore)
	if !ok {
		return Order{}, ErrSandboxIssuance
	}
	order, started, err := store.BeginKeepgoSandboxIssuance(ctx, orderID, userID, s.now().UTC())
	if err != nil || !started {
		return order, err
	}
	line, issueErr := s.sandbox.IssueSandboxLine(ctx, order.Quote)
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if issueErr != nil {
		if err := store.ReviewKeepgoSandboxIssuance(saveCtx, orderID); err != nil {
			return Order{}, ErrSandboxIssuance
		}
		order.OrderStatus, order.FulfillmentStatus = "manual_review", "manual_review"
		return order, nil
	}
	order, err = store.FinishKeepgoSandboxIssuance(saveCtx, orderID, line)
	if err != nil {
		return Order{}, ErrSandboxIssuance
	}
	return order, nil
}
