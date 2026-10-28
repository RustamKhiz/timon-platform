package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/RustamKhiz/timon-platform/internal/sim"
	qrcode "github.com/skip2/go-qrcode"
)

type Button struct {
	Text string
	Data string
}

type Response struct {
	Text             string
	Buttons          [][]Button
	Photo            []byte
	PhotoFilename    string
	DeliveredOrderID string
}

type userSelection struct {
	CountryCode string
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
		return e.selectCountry(ctx, userID, strings.TrimPrefix(input, "country:"))
	case strings.HasPrefix(input, "package:"):
		return e.selectPackage(ctx, userID, strings.TrimPrefix(input, "package:"))
	case strings.HasPrefix(input, "order:"):
		return e.createOrder(ctx, userID, strings.TrimPrefix(input, "order:"))
	case strings.HasPrefix(input, "pay:"):
		return e.mockPay(ctx, userID, strings.TrimPrefix(input, "pay:"))
	case strings.HasPrefix(input, "esim:"):
		return e.esimDetails(ctx, userID, strings.TrimPrefix(input, "esim:"))
	default:
		return mainMenu(), nil
	}
}

func mainMenu() Response {
	return Response{
		Text: "ТЕСТ / SANDBOX\n\nTIMON SIM — тестовый каталог. Реальные платежи не используются, выдача eSIM пока остаётся mock.",
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
	return Response{Text: "ТЕСТ / SANDBOX\n\nВыберите страну:", Buttons: rows}, nil
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

func (e *Engine) selectCountry(ctx context.Context, userID int64, code string) (Response, error) {
	e.mu.Lock()
	e.users[userID] = userSelection{CountryCode: code}
	e.mu.Unlock()
	packages, err := e.client.Packages(ctx, code)
	if err != nil {
		return Response{}, err
	}
	if len(packages) == 0 {
		return Response{Text: "ТЕСТ / SANDBOX\n\nДля этой страны сейчас нет доступных пакетов.", Buttons: [][]Button{{{Text: "Выбрать другую страну", Data: "buy"}}}}, nil
	}
	buttons := make([][]Button, 0, len(packages)+1)
	for _, pkg := range packages {
		buttons = append(buttons, []Button{{Text: fmt.Sprintf("%s — %d ₽", pkg.Name, pkg.RetailRUB), Data: "package:" + pkg.ID}})
	}
	buttons = append(buttons, []Button{{Text: "Выбрать другую страну", Data: "buy"}})
	return Response{Text: "ТЕСТ / SANDBOX\n\nВыберите готовый пакет. Объём указан на весь срок действия:", Buttons: buttons}, nil
}

func (e *Engine) selectPackage(ctx context.Context, userID int64, packageID string) (Response, error) {
	e.mu.Lock()
	selection, ok := e.users[userID]
	e.mu.Unlock()
	if !ok || selection.CountryCode == "" {
		return Response{}, errorsNew("сначала выберите страну")
	}
	quote, err := e.client.CreateQuote(ctx, sim.Selection{
		CountryCode: selection.CountryCode,
		PackageID:   packageID,
	})
	if err != nil {
		return Response{}, err
	}
	text := fmt.Sprintf("ТЕСТ / SANDBOX\n\n%s, %s. Цена: %d ₽. Закупка рассчитана по sandbox-тарифу Keepgo; выдача после mock-оплаты остаётся тестовой. Quote действует 15 минут.", selection.CountryCode, quote.Selection.PackageName, quote.RetailRUB)
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
	return activationResponse(order)
}

func (e *Engine) esimDetails(ctx context.Context, userID int64, orderID string) (Response, error) {
	order, err := e.client.Order(ctx, orderID, userID)
	if err != nil {
		return Response{}, err
	}
	return activationResponse(order)
}

func activationResponse(order sim.Order) (Response, error) {
	activation := order.Activation
	if activation == nil || !activation.IsMock {
		return Response{}, errorsNew("тестовая eSIM ещё не готова")
	}
	png, err := qrcode.Encode(activation.ManualCode, qrcode.Medium, 512)
	if err != nil {
		return Response{}, fmt.Errorf("generate mock activation QR: %w", err)
	}
	text := fmt.Sprintf("ТЕСТ / MOCK\n\nТестовая eSIM готова. QR и данные ниже не работают в реальной сети.\n\nУстановка:\n• iPhone: Настройки → Сотовая связь → Добавить eSIM → Использовать QR-код.\n• Android: Настройки → Сеть и интернет / SIM → Добавить eSIM → Сканировать QR-код.\n\nРучная установка:\nSM-DP+: %s\nКод: %s\nLPA: %s", activation.SMAddress, activation.ActivationCode, activation.ManualCode)
	return Response{
		Text:             text,
		Buttons:          [][]Button{{{Text: "Мои eSIM", Data: "my"}}, {{Text: "В меню", Data: "menu"}}},
		Photo:            png,
		PhotoFilename:    "timon-sim-mock-qr.png",
		DeliveredOrderID: order.ID,
	}, nil
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
	buttons := make([][]Button, 0, len(orders)+1)
	for index, order := range orders {
		if order.Quote.Selection.PackageName != "" {
			fmt.Fprintf(&text, "\n%s — %s, %s — %s", order.ID, order.Quote.Selection.CountryCode, order.Quote.Selection.PackageName, order.OrderStatus)
		} else {
			fmt.Fprintf(&text, "\n%s — %s, %d дней, %d ГБ/день — %s", order.ID, order.Quote.Selection.CountryCode, order.Quote.Selection.DurationDays, order.Quote.Selection.GigabytesPerDay, order.OrderStatus)
		}
		if order.Activation != nil && order.Activation.IsMock {
			buttons = append(buttons, []Button{{Text: fmt.Sprintf("Показать eSIM #%d", index+1), Data: "esim:" + order.ID}})
		}
	}
	buttons = append(buttons, []Button{{Text: "В меню", Data: "menu"}})
	return Response{Text: text.String(), Buttons: buttons}, nil
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
