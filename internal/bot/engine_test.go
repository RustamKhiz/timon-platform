package bot

import (
	"bytes"
	"context"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

type fakeClient struct {
	selection sim.Selection
	orders    []sim.Order
}

func TestEngineSupportsTelegramCommandsAndMockLabel(t *testing.T) {
	engine := NewEngine(&fakeClient{})
	ctx := context.Background()

	for _, input := range []string{"/start", "/start@timon_sim_test_bot", "/support"} {
		response, err := engine.Handle(ctx, 42, input)
		if err != nil {
			t.Fatalf("Handle(%q) error = %v", input, err)
		}
		if !strings.HasPrefix(response.Text, "ТЕСТ /") {
			t.Fatalf("Handle(%q) text = %q", input, response.Text)
		}
	}

	response, err := engine.Handle(ctx, 42, "/buy")
	if err != nil || len(response.Buttons) != 2 || response.Buttons[0][0].Data != "country:TR" {
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
func (f *fakeClient) Packages(context.Context, string) ([]sim.Package, error) {
	return []sim.Package{{ID: "keepgo:29:data:20480:30", CountryCode: "TR", Name: "20 GB / 30 days", DataMB: 20 * 1024, ValidityDays: 30, RetailRUB: 916}}, nil
}
func (f *fakeClient) CreateQuote(_ context.Context, selection sim.Selection) (sim.Quote, error) {
	selection.PackageName = "20 GB / 30 days"
	selection.TotalDataMB = 20 * 1024
	selection.DurationDays = 30
	f.selection = selection
	return sim.Quote{ID: "q_1", Selection: selection, RetailRUB: 520, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}
func (f *fakeClient) CreateOrder(context.Context, string, int64, string) (sim.Order, error) {
	return sim.Order{ID: "o_1"}, nil
}
func (f *fakeClient) MockPay(context.Context, string, int64) (sim.Order, error) {
	return readyOrder(), nil
}
func (f *fakeClient) MarkDelivered(context.Context, string, int64) error { return nil }
func (f *fakeClient) Orders(context.Context, int64) ([]sim.Order, error) { return f.orders, nil }
func (f *fakeClient) Order(context.Context, string, int64) (sim.Order, error) {
	return readyOrder(), nil
}

func readyOrder() sim.Order {
	return sim.Order{
		ID:                "o_1234567890",
		OrderStatus:       "completed",
		PaymentStatus:     "succeeded",
		FulfillmentStatus: "provisioned",
		DeliveryStatus:    "sent",
		Quote:             sim.Quote{Selection: sim.Selection{CountryCode: "TR", PackageName: "20 GB / 30 days", TotalDataMB: 20 * 1024, DurationDays: 30}},
		Activation:        &sim.ActivationData{SMAddress: "mock.invalid", ActivationCode: "MOCK", ManualCode: "LPA:1$mock.invalid$MOCK", IsMock: true},
	}
}

func TestEngineCountryPackageFlow(t *testing.T) {
	client := &fakeClient{}
	engine := NewEngine(client)
	ctx := context.Background()

	response, err := engine.Handle(ctx, 42, "buy")
	if err != nil || len(response.Buttons) != 2 || response.Buttons[0][0].Data != "country:TR" {
		t.Fatalf("country menu = %+v, err = %v", response, err)
	}
	response, err = engine.Handle(ctx, 42, "country:TR")
	if err != nil || len(response.Buttons) != 3 || response.Buttons[0][0].Data != "package:TR|keepgo:29:data:20480:30" {
		t.Fatalf("package menu = %+v, err = %v", response, err)
	}
	engine = NewEngine(client)
	response, err = engine.Handle(ctx, 42, "package:TR|keepgo:29:data:20480:30")
	if err != nil || client.selection.CountryCode != "TR" || client.selection.PackageID != "keepgo:29:data:20480:30" {
		t.Fatalf("selection = %+v, response = %+v, err = %v", client.selection, response, err)
	}
	if !strings.Contains(response.Text, "20 ГБ · 30 дней") || !strings.Contains(response.Text, "≈ 26 ₽ за 1 ГБ") {
		t.Fatalf("package details = %q", response.Text)
	}
	if response.Buttons[0][0].Data != "compat:q_1|TR" {
		t.Fatalf("package details buttons = %+v", response.Buttons)
	}

	response, err = engine.Handle(ctx, 42, "compat:q_1|TR")
	if err != nil || response.Buttons[2][0].Data != "confirm:q_1" {
		t.Fatalf("compatibility response = %+v, err = %v", response, err)
	}
	for _, row := range response.Buttons {
		for _, button := range row {
			if len(button.Data) > 64 {
				t.Fatalf("callback_data exceeds Telegram limit: %q", button.Data)
			}
		}
	}
	response, err = engine.Handle(ctx, 42, "confirm:q_1")
	if err != nil || !strings.Contains(response.Text, "Тестовый заказ создан") {
		t.Fatalf("confirmed order response = %+v, err = %v", response, err)
	}
}

func TestEngineProvidesProductHelpAndCompatibility(t *testing.T) {
	engine := NewEngine(&fakeClient{})
	ctx := context.Background()

	menu, err := engine.Handle(ctx, 42, "/start")
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"🏠 Главное меню", "🌍 Купить eSIM", "📱 Мои eSIM", "ℹ️ Как это работает", "✅ Совместимость", "❓ Вопросы и ответы", "🛟 Поддержка"} {
		if !hasReplyLabel(menu, label) {
			t.Fatalf("main menu does not contain %q: %+v", label, menu.ReplyKeyboard)
		}
	}
	for input, expected := range map[string]string{"🏠 Главное меню": "TIMON SIM", "🌍 Купить eSIM": "Куда вы едете?", "📱 Мои eSIM": "У вас пока нет", "ℹ️ Как это работает": "Как работает", "✅ Совместимость": "Проверка совместимости", "❓ Вопросы и ответы": "Частые вопросы", "🛟 Поддержка": "закрытый тест"} {
		response, err := engine.Handle(ctx, 42, input)
		if err != nil || !strings.Contains(response.Text, expected) {
			t.Fatalf("Handle(%q) response = %+v, err = %v", input, response, err)
		}
	}

	for _, input := range []string{"how", "/help", "compatibility", "compatibility:ios", "compatibility:android", "faq:activation", "faq:installation", "faq:refund"} {
		response, err := engine.Handle(ctx, 42, input)
		if err != nil || !strings.HasPrefix(response.Text, "ТЕСТ / SANDBOX") || !hasCallback(response, "menu") {
			t.Fatalf("Handle(%q) response = %+v, err = %v", input, response, err)
		}
	}
}

func hasReplyLabel(response Response, label string) bool {
	for _, row := range response.ReplyKeyboard {
		for _, value := range row {
			if value == label {
				return true
			}
		}
	}
	return false
}

func hasCallback(response Response, callback string) bool {
	for _, row := range response.Buttons {
		for _, button := range row {
			if button.Data == callback {
				return true
			}
		}
	}
	return false
}

func TestEngineReturnsQRCodeAndInstallationInstructions(t *testing.T) {
	engine := NewEngine(&fakeClient{})
	response, err := engine.Handle(context.Background(), 42, "pay:o_1")
	if err != nil {
		t.Fatalf("pay response error = %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(response.Photo)); err != nil {
		t.Fatalf("response photo is not PNG: %v", err)
	}
	for _, fragment := range []string{"eSIM для", "Заказ #123456", "тестовый QR"} {
		if !strings.Contains(response.Text, fragment) {
			t.Fatalf("response text does not contain %q: %q", fragment, response.Text)
		}
	}
	if response.DeliveredOrderID != "o_1234567890" {
		t.Fatalf("delivered order ID = %q", response.DeliveredOrderID)
	}
	for input, fragment := range map[string]string{"guide-ios:o_1234567890": "Установка на iPhone", "guide-android:o_1234567890": "Установка на Android", "guide-manual:o_1234567890": "LPA:1$mock.invalid$MOCK"} {
		guide, err := engine.Handle(context.Background(), 42, input)
		if err != nil || !strings.Contains(guide.Text, fragment) {
			t.Fatalf("Handle(%q) response = %+v, err = %v", input, guide, err)
		}
	}
}

func TestMyESIMsAllowsRepeatedActivationDisplay(t *testing.T) {
	client := &fakeClient{orders: []sim.Order{readyOrder()}}
	engine := NewEngine(client)
	response, err := engine.Handle(context.Background(), 42, "my")
	if err != nil {
		t.Fatalf("my response error = %v", err)
	}
	if !strings.Contains(response.Text, "Заказ: #123456") || !strings.Contains(response.Text, "Статус: Инструкция отправлена") {
		t.Fatalf("my response text = %q", response.Text)
	}
	if len(response.Buttons) < 1 || response.Buttons[0][0].Data != "details:o_1234567890" {
		t.Fatalf("my response buttons = %+v", response.Buttons)
	}
	details, err := engine.Handle(context.Background(), 42, "details:o_1234567890")
	if err != nil || !strings.Contains(details.Text, "Инструкция отправлена") || !hasCallback(details, "esim:o_1234567890") {
		t.Fatalf("details response = %+v, error = %v", details, err)
	}
	repeated, err := engine.Handle(context.Background(), 42, "esim:o_1234567890")
	if err != nil || len(repeated.Photo) == 0 {
		t.Fatalf("repeat response = %+v, error = %v", repeated, err)
	}
}
