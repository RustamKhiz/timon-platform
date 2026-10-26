package bot

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestTelegramCallDoesNotExposeTokenInNetworkError(t *testing.T) {
	const token = "secret-test-token"
	telegram := &Telegram{
		token: token,
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network failed")
		})},
	}

	err := telegram.call(context.Background(), "getUpdates", map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected network error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error exposed Telegram token: %q", err)
	}
}
