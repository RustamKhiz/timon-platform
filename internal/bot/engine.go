package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

type Button struct {
	Text string
	Data string
}

type Response struct {
	Text             string
	Buttons          [][]Button
	DeliveredOrderID string
}

type userSelection struct {
	CountryCode     string
	DurationDays    int
	GigabytesPerDay int
}

type Engine struct {
	client SIMClient
	mu     sync.Mutex
	users  map[int64]userSelection
}

func NewEngine(client SIMClient) *Engine {
	return &Engine{client: client, users: make(map[int64]userSelection)}
}

func (e *Engine) Handle(ctx context.Context, userID int64, input string) (Response, error) {
	input = normalizeInput(input)
	switch {
	case input == "/start" || input == "menu":
		return mainMenu(), nil
	case input == "/buy" || input == "buy":
		return e.countryMenu(ctx)
	case input == "/my" || input == "my":
		return e.myESIMs(ctx, userID)
	case input == "/support" || input == "support":
		return support(), nil
	case strings.HasPrefix(input, "country:"):
		return e.selectCountry(userID, strings.TrimPrefix(input, "country:")), nil
	case strings.HasPrefix(input, "days:"):
		return e.selectDays(userID, strings.TrimPrefix(input, "days:"))
	case strings.HasPrefix(input, "gb:"):
		return e.selectGB(ctx, userID, strings.TrimPrefix(input, "gb:"))
	case strings.HasPrefix(input, "order:"):
		return e.createOrder(ctx, userID, strings.TrimPrefix(input, "order:"))
	case strings.HasPrefix(input, "pay:"):
		return e.mockPay(ctx, userID, strings.TrimPrefix(input, "pay:"))
	default:
		return mainMenu(), nil
	}
}

func mainMenu() Response {
	return Response{
		Text: "ТЕСТ / MOCK\n\nTIMON SIM — тестовый каталог. Реальные платежи и eSIM не используются.",
		Buttons: [][]Button{
			{{Text: "Купить eSIM", Data: "buy"}},
			{{Text: "Мои eSIM", Data: "my"}},
			{{Text: "Поддержка", Data: "support"}},
		},
	}
}

func support() Response {
	return Response{
		Text:    "ТЕСТ / MOCK\n\nЭто закрытый тест TIMON SIM. По вопросам обратитесь к владельцу тестирования.",
		Buttons: [][]Button{{{Text: "В меню", Data: "menu"}}},
	}
}

func (e *Engine) countryMenu(ctx context.Context) (Response, error) {
	countries, err := e.client.Countries(ctx)
	if err != nil {
		return Response{}, err
	}
	rows := make([][]Button, 0, len(countries))
	for _, country := range countries {
		rows = append(rows, []Button{{Text: countryLabel(country), Data: "country:" + country.Code}})
	}
	return Response{Text: "ТЕСТ / MOCK\n\nВыберите страну:", Buttons: rows}, nil
}

func countryLabel(country sim.Country) string {
	flags := map[string]string{
		"TR": "🇹🇷",
		"EG": "🇪🇬",
		"AE": "🇦🇪",
		"IN": "🇮🇳",
		"TH": "🇹🇭",
		"VN": "🇻🇳",
		"CN": "🇨🇳",
	}
	if flag := flags[country.Code]; flag != "" {
		return flag + " " + country.Name
	}
	return country.Name
}

func (e *Engine) selectCountry(userID int64, code string) Response {
	e.mu.Lock()
	e.users[userID] = userSelection{CountryCode: code}
	e.mu.Unlock()
	buttons := make([]Button, 0, 30)
	for day := 1; day <= 30; day++ {
		buttons = append(buttons, Button{Text: strconv.Itoa(day), Data: "days:" + strconv.Itoa(day)})
	}
	return Response{Text: "ТЕСТ / MOCK\n\nНа сколько дней нужна eSIM?", Buttons: rows(buttons, 5)}
}

func (e *Engine) selectDays(userID int64, value string) (Response, error) {
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 || days > 30 {
		return Response{}, errorsNew("срок должен быть от 1 до 30 дней")
	}
	e.mu.Lock()
	selection, ok := e.users[userID]
	selection.DurationDays = days
	e.users[userID] = selection
	e.mu.Unlock()
	if !ok || selection.CountryCode == "" {
		return Response{}, errorsNew("сначала выберите страну")
	}
	buttons := make([]Button, 0, 5)
	for gb := 1; gb <= 5; gb++ {
		buttons = append(buttons, Button{Text: fmt.Sprintf("%d ГБ/день", gb), Data: "gb:" + strconv.Itoa(gb)})
	}
	return Response{Text: "ТЕСТ / MOCK\n\nСколько гигабайт в день?", Buttons: rows(buttons, 1)}, nil
}

func (e *Engine) selectGB(ctx context.Context, userID int64, value string) (Response, error) {
	gb, err := strconv.Atoi(value)
	if err != nil || gb < 1 || gb > 5 {
		return Response{}, errorsNew("объём должен быть от 1 до 5 ГБ в день")
	}
	e.mu.Lock()
	selection, ok := e.users[userID]
	selection.GigabytesPerDay = gb
	e.users[userID] = selection
	e.mu.Unlock()
	if !ok || selection.CountryCode == "" || selection.DurationDays == 0 {
		return Response{}, errorsNew("начните выбор заново")
	}
	quote, err := e.client.CreateQuote(ctx, sim.Selection{
		CountryCode:     selection.CountryCode,
		DurationDays:    selection.DurationDays,
		GigabytesPerDay: selection.GigabytesPerDay,
	})
	if err != nil {
		return Response{}, err
	}
	text := fmt.Sprintf("ТЕСТ / MOCK\n\nТестовый расчёт: %s, %d дней, %d ГБ/день. Цена: %d ₽. Quote действует 15 минут.", selection.CountryCode, selection.DurationDays, selection.GigabytesPerDay, quote.RetailRUB)
	return Response{Text: text, Buttons: [][]Button{{{Text: "Создать тестовый заказ", Data: "order:" + quote.ID}}, {{Text: "В меню", Data: "menu"}}}}, nil
}

func (e *Engine) createOrder(ctx context.Context, userID int64, quoteID string) (Response, error) {
	key := fmt.Sprintf("bot-%d-%s", userID, quoteID)
	order, err := e.client.CreateOrder(ctx, quoteID, userID, key)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Text:    "ТЕСТ / MOCK\n\nТестовый заказ создан. Настоящее списание денег не выполняется.",
		Buttons: [][]Button{{{Text: "Подтвердить mock-оплату", Data: "pay:" + order.ID}}, {{Text: "В меню", Data: "menu"}}},
	}, nil
}

func (e *Engine) mockPay(ctx context.Context, userID int64, orderID string) (Response, error) {
	order, err := e.client.MockPay(ctx, orderID, userID)
	if err != nil {
		return Response{}, err
	}
	activation := order.Activation
	if activation == nil || !activation.IsMock {
		return Response{}, errorsNew("mock provider did not return activation data")
	}
	text := fmt.Sprintf("ТЕСТ / MOCK\n\nТестовая eSIM готова. Это нерабочие mock-данные.\n\nSM-DP+: %s\nКод: %s\nManual: %s", activation.SMAddress, activation.ActivationCode, activation.ManualCode)
	return Response{Text: text, Buttons: [][]Button{{{Text: "Мои eSIM", Data: "my"}}, {{Text: "В меню", Data: "menu"}}}, DeliveredOrderID: order.ID}, nil
}

func (e *Engine) myESIMs(ctx context.Context, userID int64) (Response, error) {
	orders, err := e.client.Orders(ctx, userID)
	if err != nil {
		return Response{}, err
	}
	if len(orders) == 0 {
		return Response{Text: "ТЕСТ / MOCK\n\nУ вас пока нет тестовых eSIM.", Buttons: [][]Button{{{Text: "Купить eSIM", Data: "buy"}}}}, nil
	}
	var text strings.Builder
	text.WriteString("ТЕСТ / MOCK\n\nВаши тестовые eSIM:\n")
	for _, order := range orders {
		fmt.Fprintf(&text, "\n%s — %s, %d дней, %d ГБ/день — %s", order.ID, order.Quote.Selection.CountryCode, order.Quote.Selection.DurationDays, order.Quote.Selection.GigabytesPerDay, order.OrderStatus)
	}
	return Response{Text: text.String(), Buttons: [][]Button{{{Text: "В меню", Data: "menu"}}}}, nil
}

func normalizeInput(input string) string {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return input
	}
	command := strings.Fields(input)[0]
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return strings.ToLower(command)
}

func rows(buttons []Button, width int) [][]Button {
	result := make([][]Button, 0, (len(buttons)+width-1)/width)
	for start := 0; start < len(buttons); start += width {
		end := start + width
		if end > len(buttons) {
			end = len(buttons)
		}
		result = append(result, buttons[start:end])
	}
	return result
}

type userError string

func (e userError) Error() string { return string(e) }

func errorsNew(message string) error { return userError(message) }
