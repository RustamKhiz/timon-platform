package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestTelegramSendUsesPersistentReplyKeyboard(t *testing.T) {
	var payload map[string]any
	telegram := &Telegram{
		token: "test-token",
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})},
	}

	err := telegram.send(context.Background(), 42, mainMenu())
	if err != nil {
		t.Fatal(err)
	}
	markup, ok := payload["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("reply_markup = %#v", payload["reply_markup"])
	}
	if markup["is_persistent"] != true || markup["resize_keyboard"] != true {
		t.Fatalf("reply keyboard options = %#v", markup)
	}
	if _, ok := markup["keyboard"]; !ok {
		t.Fatalf("reply keyboard is absent: %#v", markup)
	}
	if _, ok := markup["inline_keyboard"]; ok {
		t.Fatalf("unexpected inline keyboard: %#v", markup)
	}
}

func TestSafeErrorResponseDoesNotExposeInternalError(t *testing.T) {
	response := safeErrorResponse("country:TR")
	if strings.Contains(response.Text, "127.0.0.1") || strings.Contains(response.Text, "deadline") {
		t.Fatalf("unsafe error response = %q", response.Text)
	}
	if len(response.Buttons) == 0 || response.Buttons[0][0].Data != "country:TR" {
		t.Fatalf("retry button is absent: %+v", response.Buttons)
	}
	longInput := "package:" + strings.Repeat("x", 80)
	response = safeErrorResponse(longInput)
	if response.Buttons[0][0].Data == longInput {
		t.Fatal("oversized callback was returned")
	}
}

func TestTelegramSendPhotoUsesMultipartWithoutExposingToken(t *testing.T) {
	const token = "secret-photo-token"
	var gotContentType string
	var gotBody []byte
	telegram := &Telegram{
		token: token,
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(request.URL.Path, "/sendPhoto") {
				t.Fatalf("request path = %q", request.URL.Path)
			}
			gotContentType = request.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(request.Body)
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})},
	}

	err := telegram.send(context.Background(), 42, Response{
		Text:          "ТЕСТ / MOCK QR",
		Photo:         []byte("fake-png"),
		PhotoFilename: "qr.png",
		Buttons:       [][]Button{{{Text: "В меню", Data: "menu"}}},
	})
	if err != nil {
		t.Fatalf("send photo error = %v", err)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	for _, fragment := range []string{"ТЕСТ / MOCK QR", "fake-png", "qr.png", "reply_markup"} {
		if !bytes.Contains(gotBody, []byte(fragment)) {
			t.Fatalf("multipart body does not contain %q", fragment)
		}
	}
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
