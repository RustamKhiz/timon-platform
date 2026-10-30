package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/adminauth"
	"github.com/RustamKhiz/timon-platform/internal/sim"
	"golang.org/x/crypto/bcrypt"
)

func TestMockVerticalSliceHTTP(t *testing.T) {
	service := sim.NewService(sim.PricingConfig{FixedMarkupRUB: 100})
	server := httptest.NewServer(New(service, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()

	quoteResponse := postJSON(t, server.URL+"/api/v1/sim/quotes", map[string]any{
		"country_code": "TH", "duration_days": 14, "gigabytes_per_day": 2,
	}, "")
	if quoteResponse.StatusCode != http.StatusCreated {
		t.Fatalf("quote status = %d", quoteResponse.StatusCode)
	}
	var quote sim.Quote
	decodeResponse(t, quoteResponse, &quote)

	orderResponse := postJSON(t, server.URL+"/api/v1/sim/orders", map[string]any{
		"quote_id": quote.ID, "telegram_user_id": 777,
	}, "test-key")
	if orderResponse.StatusCode != http.StatusCreated {
		t.Fatalf("order status = %d", orderResponse.StatusCode)
	}
	var order sim.Order
	decodeResponse(t, orderResponse, &order)

	paidResponse := postJSON(t, server.URL+"/api/v1/sim/orders/"+order.ID+"/mock-payment", map[string]any{
		"telegram_user_id": 777,
	}, "")
	if paidResponse.StatusCode != http.StatusOK {
		t.Fatalf("mock payment status = %d", paidResponse.StatusCode)
	}
	decodeResponse(t, paidResponse, &order)
	if order.Activation == nil || !order.Activation.IsMock {
		t.Fatalf("expected mock activation, got %+v", order.Activation)
	}

	response, err := http.Get(server.URL + "/api/v1/sim/orders?telegram_user_id=777")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", response.StatusCode)
	}
}

func TestPackageCatalogueAndPackageQuoteHTTP(t *testing.T) {
	service := sim.NewService(sim.PricingConfig{FixedMarkupRUB: 100, ExpectedFeeBPS: 350, MinimumMargin: 50})
	server := httptest.NewServer(New(service, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	response, err := http.Get(server.URL + "/api/v1/sim/packages?country_code=TR")
	if err != nil {
		t.Fatal(err)
	}
	var packagesResponse struct {
		Packages []sim.Package `json:"packages"`
	}
	decodeResponse(t, response, &packagesResponse)
	if len(packagesResponse.Packages) == 0 {
		t.Fatal("mock package catalogue is empty")
	}
	quoteResponse := postJSON(t, server.URL+"/api/v1/sim/quotes", map[string]any{
		"country_code": "TR", "package_id": packagesResponse.Packages[0].ID,
	}, "")
	if quoteResponse.StatusCode != http.StatusCreated {
		t.Fatalf("package quote status = %d", quoteResponse.StatusCode)
	}
	var quote sim.Quote
	decodeResponse(t, quoteResponse, &quote)
	if quote.Selection.PackageID == "" || quote.Selection.PackageName == "" || quote.AppliedMarkupRUB < 100 {
		t.Fatalf("unexpected package quote: %+v", quote)
	}
}

func TestBotAuthentication(t *testing.T) {
	service := sim.NewService(sim.PricingConfig{FixedMarkupRUB: 100})
	server := httptest.NewServer(New(service, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{BotToken: "test-secret", StorageMode: "memory"}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/sim/countries", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}

	request.Header.Set("Authorization", "Bearer test-secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status = %d", response.StatusCode)
	}
}

func TestAdminAuthenticationAndReadOnlyOrders(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := adminauth.New("owner", string(hash), "01234567890123456789012345678901", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service := sim.NewService(sim.PricingConfig{FixedMarkupRUB: 100})
	server := httptest.NewServer(New(service, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{AdminAuth: manager}))
	defer server.Close()

	response, err := http.Get(server.URL + "/api/v1/admin/orders")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin status = %d", response.StatusCode)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginBody, err := json.Marshal(map[string]string{"username": "owner", "password": "admin-password"})
	if err != nil {
		t.Fatal(err)
	}
	loginResponse, err := client.Post(server.URL+"/api/v1/admin/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}

	response, err = client.Get(server.URL + "/api/v1/admin/orders")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated admin status = %d", response.StatusCode)
	}

	invalidResponse, err := client.Get(server.URL + "/api/v1/admin/orders?status=unknown")
	if err != nil {
		t.Fatal(err)
	}
	invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid admin filter status = %d", invalidResponse.StatusCode)
	}
}

func postJSON(t *testing.T, endpoint string, body any, key string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
