package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

type SIMClient interface {
	Countries(context.Context) ([]sim.Country, error)
	CreateQuote(context.Context, sim.Selection) (sim.Quote, error)
	CreateOrder(context.Context, string, int64, string) (sim.Order, error)
	MockPay(context.Context, string, int64) (sim.Order, error)
	MarkDelivered(context.Context, string, int64) error
	Orders(context.Context, int64) ([]sim.Order, error)
}

type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPClient(baseURL string, tokens ...string) *HTTPClient {
	token := ""
	if len(tokens) > 0 {
		token = tokens[0]
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPClient) Countries(ctx context.Context) ([]sim.Country, error) {
	var response struct {
		Countries []sim.Country `json:"countries"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/sim/countries", nil, "", &response); err != nil {
		return nil, err
	}
	return response.Countries, nil
}

func (c *HTTPClient) CreateQuote(ctx context.Context, selection sim.Selection) (sim.Quote, error) {
	var quote sim.Quote
	err := c.do(ctx, http.MethodPost, "/api/v1/sim/quotes", selection, "", &quote)
	return quote, err
}

func (c *HTTPClient) CreateOrder(ctx context.Context, quoteID string, userID int64, idempotencyKey string) (sim.Order, error) {
	var order sim.Order
	body := map[string]any{"quote_id": quoteID, "telegram_user_id": userID}
	err := c.do(ctx, http.MethodPost, "/api/v1/sim/orders", body, idempotencyKey, &order)
	return order, err
}

func (c *HTTPClient) MockPay(ctx context.Context, orderID string, userID int64) (sim.Order, error) {
	var order sim.Order
	body := map[string]any{"telegram_user_id": userID}
	err := c.do(ctx, http.MethodPost, "/api/v1/sim/orders/"+url.PathEscape(orderID)+"/mock-payment", body, "", &order)
	return order, err
}

func (c *HTTPClient) MarkDelivered(ctx context.Context, orderID string, userID int64) error {
	body := map[string]any{"telegram_user_id": userID}
	return c.do(ctx, http.MethodPost, "/api/v1/sim/orders/"+url.PathEscape(orderID)+"/delivered", body, "", nil)
}

func (c *HTTPClient) Orders(ctx context.Context, userID int64) ([]sim.Order, error) {
	var response struct {
		Orders []sim.Order `json:"orders"`
	}
	path := "/api/v1/sim/orders?telegram_user_id=" + strconv.FormatInt(userID, 10)
	if err := c.do(ctx, http.MethodGet, path, nil, "", &response); err != nil {
		return nil, err
	}
	return response.Orders, nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, idempotencyKey string, target any) error {
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		requestBody = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var apiError struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&apiError)
		return fmt.Errorf("SIM API %s: %s", response.Status, apiError.Error)
	}
	if target == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(target)
}
