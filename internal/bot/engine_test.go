package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

type fakeClient struct {
	selection sim.Selection
}

func TestEngineSupportsTelegramCommandsAndMockLabel(t *testing.T) {
	engine := NewEngine(&fakeClient{})
	ctx := context.Background()

	for _, input := range []string{"/start", "/start@timon_sim_test_bot", "/support"} {
		response, err := engine.Handle(ctx, 42, input)
		if err != nil {
			t.Fatalf("Handle(%q) error = %v", input, err)
		}
		if !strings.HasPrefix(response.Text, "ТЕСТ / MOCK") {
			t.Fatalf("Handle(%q) text = %q", input, response.Text)
		}
	}

	response, err := engine.Handle(ctx, 42, "/buy")
	if err != nil || len(response.Buttons) != 1 || response.Buttons[0][0].Data != "country:TR" {
		t.Fatalf("/buy response = %+v, err = %v", response, err)
	}
	if response.Buttons[0][0].Text != "🇹🇷 Турция" {
		t.Fatalf("country button text = %q", response.Buttons[0][0].Text)
	}
}

func TestParseAllowedUserIDs(t *testing.T) {
	allowed, err := ParseAllowedUserIDs("42, 99,42")
	if err != nil {
		t.Fatalf("ParseAllowedUserIDs() error = %v", err)
	}
	if len(allowed) != 2 {
		t.Fatalf("allowed count = %d", len(allowed))
	}
	if _, ok := allowed[42]; !ok {
		t.Fatal("user 42 is not allowed")
	}

	for _, value := range []string{"", "0", "abc"} {
		if _, err := ParseAllowedUserIDs(value); err == nil {
			t.Fatalf("ParseAllowedUserIDs(%q) returned nil error", value)
		}
	}
}

func (f *fakeClient) Countries(context.Context) ([]sim.Country, error) {
	return []sim.Country{{Code: "TR", Name: "Турция"}}, nil
}
func (f *fakeClient) CreateQuote(_ context.Context, selection sim.Selection) (sim.Quote, error) {
	f.selection = selection
	return sim.Quote{ID: "q_1", Selection: selection, RetailRUB: 520, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}
func (f *fakeClient) CreateOrder(context.Context, string, int64, string) (sim.Order, error) {
	return sim.Order{ID: "o_1"}, nil
}
func (f *fakeClient) MockPay(context.Context, string, int64) (sim.Order, error) {
	return sim.Order{ID: "o_1", Activation: &sim.ActivationData{SMAddress: "mock.invalid", ActivationCode: "MOCK", ManualCode: "MOCK", IsMock: true}}, nil
}
func (f *fakeClient) MarkDelivered(context.Context, string, int64) error { return nil }
func (f *fakeClient) Orders(context.Context, int64) ([]sim.Order, error) { return nil, nil }

func TestEngineCountryDaysAndGBFlow(t *testing.T) {
	client := &fakeClient{}
	engine := NewEngine(client)
	ctx := context.Background()

	response, err := engine.Handle(ctx, 42, "buy")
	if err != nil || len(response.Buttons) != 1 || response.Buttons[0][0].Data != "country:TR" {
		t.Fatalf("country menu = %+v, err = %v", response, err)
	}
	response, err = engine.Handle(ctx, 42, "country:TR")
	if err != nil || len(response.Buttons) != 6 {
		t.Fatalf("days menu rows = %d, err = %v", len(response.Buttons), err)
	}
	response, err = engine.Handle(ctx, 42, "days:30")
	if err != nil || len(response.Buttons) != 5 {
		t.Fatalf("GB menu rows = %d, err = %v", len(response.Buttons), err)
	}
	response, err = engine.Handle(ctx, 42, "gb:5")
	if err != nil || client.selection != (sim.Selection{CountryCode: "TR", DurationDays: 30, GigabytesPerDay: 5}) {
		t.Fatalf("selection = %+v, response = %+v, err = %v", client.selection, response, err)
	}
}
