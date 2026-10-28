package fx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestCBRUSDProviderAppliesBufferAndCaches(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("User-Agent") == "" {
			t.Fatal("User-Agent is required")
		}
		body := `<?xml version="1.0" encoding="windows-1251"?><ValCurs Date="28.10.2026"><Valute><CharCode>USD</CharCode><Nominal>1</Nominal><Value>80,5307</Value></Valute></ValCurs>`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	provider, err := NewCBRUSDProvider(Config{Endpoint: "https://cbr.test/daily", BufferBPS: 500, CacheTTL: time.Hour, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	rate, version, err := provider.USDToRUB(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rate < 84.5572 || rate > 84.5573 {
		t.Fatalf("rate = %f", rate)
	}
	if version != "cbr:28.10.2026:+500bps" {
		t.Fatalf("version = %q", version)
	}
	if _, _, err := provider.USDToRUB(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestCBRUSDProviderUsesConfiguredFallback(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	provider, err := NewCBRUSDProvider(Config{Endpoint: "https://cbr.test/daily", BufferBPS: 500, FallbackRate: 84.5572, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	rate, version, err := provider.USDToRUB(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rate != 84.5572 || version != "configured-fallback" {
		t.Fatalf("rate/version = %f/%q", rate, version)
	}
}
