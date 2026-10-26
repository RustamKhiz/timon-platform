package sim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidSelection    = errors.New("invalid selection")
	ErrNotFound            = errors.New("not found")
	ErrQuoteExpired        = errors.New("quote expired")
	ErrIdempotencyRequired = errors.New("idempotency key is required")
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
	ErrForbidden           = errors.New("order does not belong to user")
)

const mockProviderID = "mock-provider"

type PricingConfig struct {
	FixedMarkupRUB int
	ExpectedFeeRUB int
	MinimumMargin  int
	QuoteTTL       time.Duration
}

type memoryStore struct {
	mu             sync.RWMutex
	quotes         map[string]Quote
	orders         map[string]Order
	idempotencyMap map[string]string
}

func idempotencyScope(telegramUserID int64, idempotencyKey string) string {
	return fmt.Sprintf("%d:%s", telegramUserID, idempotencyKey)
}

type Repository interface {
	SaveQuote(context.Context, Quote) error
	CreateOrder(context.Context, string, int64, string, time.Time) (Order, error)
	PayAndProvision(context.Context, string, int64, time.Time, time.Time, ActivationData) (Order, error)
	MarkDelivered(context.Context, string, int64, time.Time) (Order, error)
	GetOrder(context.Context, string, int64) (Order, error)
	OrdersForUser(context.Context, int64) ([]Order, error)
}

type Service struct {
	now     func() time.Time
	pricing PricingConfig
	store   Repository
}

func NewService(pricing PricingConfig) *Service {
	return NewServiceWithRepository(pricing, &memoryStore{
		quotes:         make(map[string]Quote),
		orders:         make(map[string]Order),
		idempotencyMap: make(map[string]string),
	})
}

func NewServiceWithRepository(pricing PricingConfig, repository Repository) *Service {
	if pricing.FixedMarkupRUB <= 0 {
		pricing.FixedMarkupRUB = 100
	}
	if pricing.MinimumMargin <= 0 {
		pricing.MinimumMargin = 50
	}
	if pricing.QuoteTTL <= 0 {
		pricing.QuoteTTL = 15 * time.Minute
	}
	return &Service{
		now:     time.Now,
		pricing: pricing,
		store:   repository,
	}
}

func (s *Service) Countries() []Country {
	return []Country{
		{Code: "TR", Name: "Турция"},
		{Code: "EG", Name: "Египет"},
		{Code: "AE", Name: "ОАЭ"},
		{Code: "IN", Name: "Индия"},
		{Code: "TH", Name: "Таиланд"},
		{Code: "VN", Name: "Вьетнам"},
		{Code: "CN", Name: "Китай"},
	}
}

func (s *Service) CreateQuote(ctx context.Context, selection Selection) (Quote, error) {
	if !s.validCountry(selection.CountryCode) || selection.DurationDays < 1 || selection.DurationDays > 30 || selection.GigabytesPerDay < 1 || selection.GigabytesPerDay > 5 {
		return Quote{}, fmt.Errorf("%w: country must be supported, days 1..30, GB/day 1..5", ErrInvalidSelection)
	}
	if s.pricing.FixedMarkupRUB-s.pricing.ExpectedFeeRUB < s.pricing.MinimumMargin {
		return Quote{}, fmt.Errorf("pricing policy leaves less than %d RUB margin", s.pricing.MinimumMargin)
	}

	createdAt := s.now().UTC()
	providerCost := mockDailyRate(selection.CountryCode) * selection.DurationDays * selection.GigabytesPerDay
	quote := Quote{
		ID:                  newID("q"),
		ProviderID:          mockProviderID,
		Selection:           selection,
		ProviderCostRUB:     providerCost,
		FixedMarkupRUB:      s.pricing.FixedMarkupRUB,
		ExpectedFeeRUB:      s.pricing.ExpectedFeeRUB,
		RetailRUB:           providerCost + s.pricing.FixedMarkupRUB,
		ProviderRateVersion: "mock-rates-v1",
		CreatedAt:           createdAt,
		ExpiresAt:           createdAt.Add(s.pricing.QuoteTTL),
	}
	if err := s.store.SaveQuote(ctx, quote); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func (s *Service) CreateOrder(ctx context.Context, quoteID string, telegramUserID int64, idempotencyKey string) (Order, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return Order{}, ErrIdempotencyRequired
	}
	return s.store.CreateOrder(ctx, quoteID, telegramUserID, idempotencyKey, s.now().UTC())
}

func (s *Service) MockPayAndProvision(ctx context.Context, orderID string, telegramUserID int64) (Order, error) {
	now := s.now().UTC()
	deadline := now.Add(24 * time.Hour)
	activation := ActivationData{
		SMAddress:      "mock.smdp.invalid",
		ActivationCode: "MOCK-" + strings.ToUpper(newID("")),
		ManualCode:     "LPA:1$mock.smdp.invalid$MOCK-ONLY",
		IsMock:         true,
	}
	return s.store.PayAndProvision(ctx, orderID, telegramUserID, now, deadline, activation)
}

func (s *Service) MarkDelivered(ctx context.Context, orderID string, telegramUserID int64) (Order, error) {
	return s.store.MarkDelivered(ctx, orderID, telegramUserID, s.now().UTC())
}

func (s *Service) GetOrder(ctx context.Context, orderID string, telegramUserID int64) (Order, error) {
	return s.store.GetOrder(ctx, orderID, telegramUserID)
}

func (s *Service) OrdersForUser(ctx context.Context, telegramUserID int64) ([]Order, error) {
	return s.store.OrdersForUser(ctx, telegramUserID)
}

func (s *memoryStore) SaveQuote(ctx context.Context, quote Quote) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.quotes[quote.ID] = quote
	s.mu.Unlock()
	return nil
}

func (s *memoryStore) CreateOrder(ctx context.Context, quoteID string, telegramUserID int64, idempotencyKey string, now time.Time) (Order, error) {
	if err := ctx.Err(); err != nil {
		return Order{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scope := idempotencyScope(telegramUserID, idempotencyKey)
	if orderID, ok := s.idempotencyMap[scope]; ok {
		order := s.orders[orderID]
		if order.Quote.ID != quoteID {
			return Order{}, ErrIdempotencyConflict
		}
		return order, nil
	}
	quote, ok := s.quotes[quoteID]
	if !ok {
		return Order{}, ErrNotFound
	}
	if !now.Before(quote.ExpiresAt) {
		return Order{}, ErrQuoteExpired
	}
	order := Order{ID: newID("o"), TelegramUserID: telegramUserID, Quote: quote, OrderStatus: "awaiting_payment", PaymentStatus: "pending", FulfillmentStatus: "not_started", DeliveryStatus: "not_ready", CreatedAt: now}
	s.orders[order.ID] = order
	s.idempotencyMap[scope] = order.ID
	return order, nil
}

func (s *memoryStore) PayAndProvision(ctx context.Context, orderID string, telegramUserID int64, paidAt, deadline time.Time, activation ActivationData) (Order, error) {
	if err := ctx.Err(); err != nil {
		return Order{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return Order{}, ErrNotFound
	}
	if order.TelegramUserID != telegramUserID {
		return Order{}, ErrForbidden
	}
	if order.FulfillmentStatus == "provisioned" {
		return order, nil
	}
	order.PaymentStatus = "succeeded"
	order.PaidAt = &paidAt
	order.FulfillmentDeadlineAt = &deadline
	order.OrderStatus = "ready"
	order.FulfillmentStatus = "provisioned"
	order.DeliveryStatus = "pending"
	order.Activation = &activation
	s.orders[order.ID] = order
	return order, nil
}

func (s *memoryStore) MarkDelivered(ctx context.Context, orderID string, telegramUserID int64, _ time.Time) (Order, error) {
	if err := ctx.Err(); err != nil {
		return Order{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return Order{}, ErrNotFound
	}
	if order.TelegramUserID != telegramUserID {
		return Order{}, ErrForbidden
	}
	if order.FulfillmentStatus != "provisioned" {
		return Order{}, errors.New("order is not provisioned")
	}
	order.DeliveryStatus = "sent"
	order.OrderStatus = "completed"
	s.orders[order.ID] = order
	return order, nil
}

func (s *memoryStore) GetOrder(ctx context.Context, orderID string, telegramUserID int64) (Order, error) {
	if err := ctx.Err(); err != nil {
		return Order{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[orderID]
	if !ok {
		return Order{}, ErrNotFound
	}
	if order.TelegramUserID != telegramUserID {
		return Order{}, ErrForbidden
	}
	return order, nil
}

func (s *memoryStore) OrdersForUser(ctx context.Context, telegramUserID int64) ([]Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	orders := make([]Order, 0)
	for _, order := range s.orders {
		if order.TelegramUserID == telegramUserID {
			orders = append(orders, order)
		}
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].CreatedAt.After(orders[j].CreatedAt) })
	return orders, nil
}

func (s *Service) validCountry(code string) bool {
	for _, country := range s.Countries() {
		if country.Code == code {
			return true
		}
	}
	return false
}

func mockDailyRate(countryCode string) int {
	rates := map[string]int{"TR": 42, "EG": 48, "AE": 55, "IN": 36, "TH": 39, "VN": 37, "CN": 52}
	return rates[countryCode]
}

func newID(prefix string) string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	if prefix == "" {
		return hex.EncodeToString(buffer)
	}
	return prefix + "_" + hex.EncodeToString(buffer)
}
