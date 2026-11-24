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

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func TestInstructionResendWorker(t *testing.T) {
	const jobID = "22d03db0-31e0-4f55-8bb8-d57d133cfeaf"
	for _, tc := range []struct {
		name    string
		allowed bool
		mock    bool
		timeout bool
		want    string
		calls   int
	}{
		{name: "original buyer", allowed: true, mock: true, want: "sent", calls: 1},
		{name: "buyer outside allowlist", mock: true, want: "failed"},
		{name: "real activation", allowed: true, want: "failed"},
		{name: "ambiguous Telegram failure", allowed: true, mock: true, timeout: true, want: "unknown", calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := sim.Order{ID: "test-order", TelegramUserID: 42, OrderStatus: "completed", PaymentStatus: "succeeded",
				FulfillmentStatus: "provisioned", DeliveryStatus: "sent",
				Activation: &sim.ActivationData{IsMock: tc.mock, ManualCode: "LPA:1$mock.smdp.invalid$MOCK"}}
			payload, err := json.Marshal(map[string]any{"delivery": sim.InstructionDelivery{ID: jobID, Order: order}})
			if err != nil {
				t.Fatal(err)
			}
			client := NewHTTPClient("http://sim.test", "unit-test")
			var outcome string
			client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/sim/instruction-deliveries/claim":
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(payload))}, nil
				case "/api/v1/sim/instruction-deliveries/" + jobID + "/finish":
					var result map[string]string
					if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
						t.Fatal(err)
					}
					outcome = result["state"]
					return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
				default:
					t.Fatalf("unexpected SIM API path: %s", r.URL.Path)
					return nil, nil
				}
			})
			var attempts int
			telegram := &Telegram{client: client, token: "unit-test"}
			if tc.allowed {
				telegram.allowedUserIDs = map[int64]struct{}{42: {}}
			}
			telegram.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				if tc.timeout {
					return nil, errors.New("connection interrupted")
				}
				reader, err := r.MultipartReader()
				if err != nil {
					t.Fatal(err)
				}
				form, err := reader.ReadForm(1 << 20)
				if err != nil {
					t.Fatal(err)
				}
				defer form.RemoveAll()
				if ids := form.Value["chat_id"]; len(ids) != 1 || ids[0] != "42" || len(form.File["photo"]) != 1 {
					t.Fatalf("resend did not target the original buyer with a QR: %v", ids)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
			})}

			telegram.processInstructionResend(context.Background())
			if outcome != tc.want || attempts != tc.calls {
				t.Fatalf("outcome = %q, sends = %d, want %q and %d", outcome, attempts, tc.want, tc.calls)
			}
		})
	}
}
