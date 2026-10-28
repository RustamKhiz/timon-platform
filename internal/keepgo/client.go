package keepgo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const SandboxBaseURL = "https://myaccount.sandbox.keepgo.com/api/v2"

type Config struct {
	BaseURL     string
	APIKey      string
	AccessToken string
	HTTPClient  *http.Client
}

type Client struct {
	baseURL     *url.URL
	apiKey      string
	accessToken string
	httpClient  *http.Client
}

type Balance struct {
	Acknowledgement string        `json:"ack"`
	AccountBalance  DecimalString `json:"account_balance"`
}

// DecimalString accepts the two formats observed in Keepgo documentation and
// sandbox responses: a quoted decimal ("8.63") or a JSON number (1000).
type DecimalString string

func (d *DecimalString) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	if strings.HasPrefix(value, "\"") {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		value = decoded
	}
	if value == "" {
		return errors.New("empty decimal value")
	}
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return fmt.Errorf("invalid decimal value: %w", err)
	}
	*d = DecimalString(value)
	return nil
}

type Refill struct {
	Title        string  `json:"title"`
	AmountMB     int     `json:"amount_mb"`
	AmountDays   *int    `json:"amount_days"`
	PriceUSD     float64 `json:"price_usd"`
	VoiceMinutes *int    `json:"voice_minutes"`
	SMSCount     *int    `json:"sms_count"`
}

type Bundle struct {
	ID          int64    `json:"id"`
	TypeID      int64    `json:"type_id"`
	BundleType  string   `json:"bundle_type"`
	ProductType string   `json:"product_type"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Coverage    []string `json:"coverage"`
	Refills     []Refill `json:"refills"`
	KYCRequired bool     `json:"kyc_required"`
	BlockTier   string   `json:"block_tier"`
}

type BundlesResponse struct {
	Acknowledgement string   `json:"ack"`
	Bundles         []Bundle `json:"bundles"`
}

func NewClient(config Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = SandboxBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("Keepgo base URL must be an absolute HTTPS URL")
	}
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.AccessToken) == "" {
		return nil, errors.New("Keepgo apiKey and accessToken are required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{baseURL: parsed, apiKey: config.APIKey, accessToken: config.AccessToken, httpClient: httpClient}, nil
}

func (c *Client) AccountBalance(ctx context.Context) (Balance, error) {
	var response Balance
	if err := c.get(ctx, "/account_balance", nil, &response); err != nil {
		return Balance{}, err
	}
	return response, nil
}

func (c *Client) Bundles(ctx context.Context, withAsync bool) (BundlesResponse, error) {
	query := url.Values{}
	if withAsync {
		query.Set("with_async", "true")
	}
	var response BundlesResponse
	if err := c.get(ctx, "/bundles", query, &response); err != nil {
		return BundlesResponse{}, err
	}
	return response, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, destination any) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build Keepgo request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apiKey", c.apiKey)
	req.Header.Set("accessToken", c.accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Keepgo API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		return fmt.Errorf("Keepgo API returned HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode Keepgo response: %w", err)
	}
	return nil
}
