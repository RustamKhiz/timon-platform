package keepgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientUsesCredentialsAndDecodesCatalog(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apiKey") != "test-key" || r.Header.Get("accessToken") != "test-token" {
			t.Fatalf("credentials were not sent in Keepgo headers")
		}
		if r.URL.Path != "/api/v2/bundles" || r.URL.Query().Get("with_async") != "true" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ack":"success","bundles":[{"id":7,"name":"Turkey","coverage":["Turkey"],"refills":[{"title":"1 GB / 7 days","amount_mb":1024,"amount_days":7,"price_usd":4.25}]}]}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL + "/api/v2", APIKey: "test-key", AccessToken: "test-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Bundles(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Bundles) != 1 || response.Bundles[0].Refills[0].PriceUSD != 4.25 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestClientDoesNotExposeErrorBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"secret":"must-not-appear"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", AccessToken: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AccountBalance(context.Background())
	if err == nil || err.Error() != "Keepgo API returned HTTP 401" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientAcceptsNumericSandboxBalance(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ack":"success","account_balance":1000}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", AccessToken: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	balance, err := client.AccountBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(balance.AccountBalance) != "1000" {
		t.Fatalf("unexpected balance: %q", balance.AccountBalance)
	}
}

func TestClientRejectsInsecureBaseURLAndMissingCredentials(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "http://example.com", APIKey: "key", AccessToken: "token"}); err == nil {
		t.Fatal("expected insecure URL to be rejected")
	}
	if _, err := NewClient(Config{BaseURL: SandboxBaseURL}); err == nil {
		t.Fatal("expected missing credentials to be rejected")
	}
}
