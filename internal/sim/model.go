package sim

import "time"

type Country struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type Selection struct {
	CountryCode     string `json:"country_code"`
	DurationDays    int    `json:"duration_days"`
	GigabytesPerDay int    `json:"gigabytes_per_day"`
}

type Quote struct {
	ID                  string    `json:"id"`
	ProviderID          string    `json:"provider_id"`
	Selection           Selection `json:"selection"`
	ProviderCostRUB     int       `json:"provider_cost_rub"`
	FixedMarkupRUB      int       `json:"fixed_markup_rub"`
	ExpectedFeeRUB      int       `json:"expected_fee_rub"`
	RetailRUB           int       `json:"retail_rub"`
	ProviderRateVersion string    `json:"provider_rate_version"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type ActivationData struct {
	SMAddress      string `json:"sm_dp_address"`
	ActivationCode string `json:"activation_code"`
	ManualCode     string `json:"manual_code"`
	IsMock         bool   `json:"is_mock"`
}

type Order struct {
	ID                    string          `json:"id"`
	TelegramUserID        int64           `json:"telegram_user_id"`
	Quote                 Quote           `json:"quote_snapshot"`
	OrderStatus           string          `json:"order_status"`
	PaymentStatus         string          `json:"payment_status"`
	FulfillmentStatus     string          `json:"fulfillment_status"`
	DeliveryStatus        string          `json:"delivery_status"`
	Activation            *ActivationData `json:"activation,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	PaidAt                *time.Time      `json:"paid_at,omitempty"`
	FulfillmentDeadlineAt *time.Time      `json:"fulfillment_deadline_at,omitempty"`
}
