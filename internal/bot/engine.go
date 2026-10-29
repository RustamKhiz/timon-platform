package bot

import (
	"context"
	"fmt"
	"math"
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
	case input == "/help" || input == "faq":
		return faq(), nil
	case input == "how":
		return howItWorks(), nil
	case input == "compatibility":
		return compatibilityResponse("", "", ""), nil
	case input == "compatibility:ios":
		return compatibilityResponse("ios", "", ""), nil
	case input == "compatibility:android":
		return compatibilityResponse("android", "", ""), nil
	case input == "faq:activation":
		return faqActivation(), nil
	case input == "faq:installation":
		return faqInstallation(), nil
	case input == "faq:refund":
		return faqRefund(), nil
	case strings.HasPrefix(input, "country:"):
		return e.selectCountry(ctx, userID, strings.TrimPrefix(input, "country:"))
	case strings.HasPrefix(input, "package:"):
		return e.selectPackage(ctx, userID, strings.TrimPrefix(input, "package:"))
	case strings.HasPrefix(input, "compat-ios:"):
		quoteID, countryCode, err := parseQuoteContext(input, "compat-ios:")
		if err != nil {
			return Response{}, err
		}
		return compatibilityResponse("ios", quoteID, countryCode), nil
	case strings.HasPrefix(input, "compat-android:"):
		quoteID, countryCode, err := parseQuoteContext(input, "compat-android:")
		if err != nil {
			return Response{}, err
		}
		return compatibilityResponse("android", quoteID, countryCode), nil
	case strings.HasPrefix(input, "compat:"):
		quoteID, countryCode, err := parseQuoteContext(input, "compat:")
		if err != nil {
			return Response{}, err
		}
		return compatibilityResponse("", quoteID, countryCode), nil
	case strings.HasPrefix(input, "confirm:"):
		return e.createOrder(ctx, userID, strings.TrimPrefix(input, "confirm:"))
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
		Text: "ТЕСТ / SANDBOX\n\nTIMON SIM — мобильный интернет для поездок без физической SIM-карты.\n\nВыберите страну и пакет, проверьте совместимость телефона и пройдите тестовое оформление. Реальные платежи и активация сети пока выключены.",
		Buttons: [][]Button{
			{{Text: "🌍 Купить eSIM", Data: "buy"}},
			{{Text: "📱 Мои eSIM", Data: "my"}},
			{{Text: "ℹ️ Как это работает", Data: "how"}},
			{{Text: "✅ Проверить совместимость", Data: "compatibility"}},
			{{Text: "❓ Вопросы и ответы", Data: "faq"}},
			{{Text: "🛟 Поддержка", Data: "support"}},
		},
	}
}

func support() Response {
	return Response{
		Text:    "ТЕСТ / MOCK\n\nЭто закрытый тест TIMON SIM. По вопросам обратитесь к владельцу тестирования.",
		Buttons: [][]Button{{{Text: "В меню", Data: "menu"}}},
	}
}

func howItWorks() Response {
	return Response{
		Text:    "ТЕСТ / SANDBOX\n\nКак работает TIMON SIM:\n\n1. Вы выбираете страну и пакет интернета.\n2. Проверяете, что телефон поддерживает eSIM.\n3. Оплачиваете заказ — в тестовом режиме списания нет.\n4. Получаете QR-код и инструкцию.\n5. Устанавливаете eSIM и включаете её в поездке.\n\nНе удаляйте установленную eSIM до окончания поездки: повторная установка зависит от правил поставщика.",
		Buttons: [][]Button{{{Text: "🌍 Выбрать страну", Data: "buy"}}, {{Text: "✅ Проверить совместимость", Data: "compatibility"}}, {{Text: "В меню", Data: "menu"}}},
	}
}

func faq() Response {
	return Response{
		Text:    "ТЕСТ / SANDBOX\n\nЧастые вопросы\n\nВыберите тему. Точные правила активации и возврата будут подтверждены после выбора production-поставщика.",
		Buttons: [][]Button{{{Text: "Когда начинается срок действия?", Data: "faq:activation"}}, {{Text: "Можно установить заранее?", Data: "faq:installation"}}, {{Text: "Как работает возврат?", Data: "faq:refund"}}, {{Text: "✅ Совместимость телефона", Data: "compatibility"}}, {{Text: "В меню", Data: "menu"}}},
	}
}

func faqActivation() Response {
	return Response{Text: "ТЕСТ / SANDBOX\n\nКогда начинается срок действия?\n\nЭто зависит от поставщика и конкретного пакета: при установке eSIM, при первом подключении к поддерживаемой сети или сразу после покупки. Перед реальным заказом TIMON SIM покажет точное правило на экране тарифа.", Buttons: [][]Button{{{Text: "Назад к вопросам", Data: "faq"}}, {{Text: "В меню", Data: "menu"}}}}
}

func faqInstallation() Response {
	return Response{Text: "ТЕСТ / SANDBOX\n\nМожно установить eSIM заранее?\n\nОбычно да, но только если срок пакета начинается при первом подключении в стране поездки. До подтверждения правила конкретного поставщика устанавливайте eSIM незадолго до поездки и не включайте передачу данных для неё заранее.", Buttons: [][]Button{{{Text: "Назад к вопросам", Data: "faq"}}, {{Text: "В меню", Data: "menu"}}}}
}

func faqRefund() Response {
	return Response{Text: "ТЕСТ / SANDBOX\n\nКак работает возврат?\n\nВ тестовом режиме деньги не списываются. Для реального запуска будет действовать отдельный процесс проверки заказа и ручного подтверждения возврата. Точные условия появятся в оферте до подключения оплаты.", Buttons: [][]Button{{{Text: "Назад к вопросам", Data: "faq"}}, {{Text: "🛟 Поддержка", Data: "support"}}, {{Text: "В меню", Data: "menu"}}}}
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
	rows = append(rows, []Button{{Text: "В меню", Data: "menu"}})
	return Response{Text: "ТЕСТ / SANDBOX\n\nКуда вы едете?\n\nВыберите страну, чтобы посмотреть доступные пакеты интернета:", Buttons: rows}, nil
}

func countryLabel(country sim.Country) string {
	if flag := countryFlag(country.Code); flag != "" {
		return flag + " " + country.Name
	}
	return country.Name
}

func countryFlag(code string) string {
	flags := map[string]string{
		"TR": "🇹🇷",
		"EG": "🇪🇬",
		"AE": "🇦🇪",
		"IN": "🇮🇳",
		"TH": "🇹🇭",
		"VN": "🇻🇳",
		"CN": "🇨🇳",
	}
	return flags[code]
}

func countryName(code string) string {
	names := map[string]string{"TR": "Турция", "EG": "Египет", "AE": "ОАЭ", "IN": "Индия", "TH": "Таиланд", "VN": "Вьетнам", "CN": "Китай"}
	if name := names[code]; name != "" {
		return name
	}
	return code
}

func countryTitle(code string) string {
	if flag := countryFlag(code); flag != "" {
		return flag + " " + countryName(code)
	}
	return countryName(code)
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
		buttons = append(buttons, []Button{{Text: fmt.Sprintf("%s · %d дней — %d ₽", formatData(pkg.DataMB), pkg.ValidityDays, pkg.RetailRUB), Data: "package:" + pkg.ID}})
	}
	buttons = append(buttons, []Button{{Text: "Выбрать другую страну", Data: "buy"}})
	buttons = append(buttons, []Button{{Text: "В меню", Data: "menu"}})
	return Response{Text: fmt.Sprintf("ТЕСТ / SANDBOX\n\n%s\n\nВыберите пакет. Объём указан на весь срок действия:\n\n• 5 ГБ — мессенджеры и навигация\n• 10 ГБ — оптимальный выбор\n• 20 ГБ — активное использование", countryTitle(code)), Buttons: buttons}, nil
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
	dataLabel := formatData(quote.Selection.TotalDataMB)
	if quote.Selection.TotalDataMB == 0 {
		dataLabel = quote.Selection.PackageName
	}
	daysLabel := ""
	if quote.Selection.DurationDays > 0 {
		daysLabel = fmt.Sprintf(" · %d дней", quote.Selection.DurationDays)
	}
	pricePerGB := ""
	if quote.Selection.TotalDataMB > 0 {
		perGB := int(math.Ceil(float64(quote.RetailRUB) / (float64(quote.Selection.TotalDataMB) / 1024)))
		pricePerGB = fmt.Sprintf("\n≈ %d ₽ за 1 ГБ", perGB)
	}
	text := fmt.Sprintf("ТЕСТ / SANDBOX\n\n%s\n\n%s%s\n%s\n\nЦена: %d ₽%s\nЦена действительна 15 минут.\n\nПеред оформлением убедитесь, что телефон поддерживает eSIM.", countryTitle(selection.CountryCode), dataLabel, daysLabel, packageRecommendation(quote.Selection.TotalDataMB), quote.RetailRUB, pricePerGB)
	context := quote.ID + "|" + selection.CountryCode
	return Response{Text: text, Buttons: [][]Button{{{Text: "✅ Проверить совместимость", Data: "compat:" + context}}, {{Text: "Выбрать другой тариф", Data: "country:" + selection.CountryCode}}, {{Text: "Выбрать другую страну", Data: "buy"}}, {{Text: "В меню", Data: "menu"}}}}, nil
}

func compatibilityResponse(platform, quoteID, countryCode string) Response {
	text := "ТЕСТ / SANDBOX\n\nПроверка совместимости\n\neSIM должна поддерживаться самим телефоном и быть разрешена для его региона продажи. Если в настройках есть пункт добавления eSIM или мобильного тарифа, устройство, как правило, совместимо."
	if platform == "ios" {
		text = "ТЕСТ / SANDBOX\n\nПроверка на iPhone\n\nОткройте Настройки → Сотовая связь. Найдите пункт «Добавить eSIM» или «Добавить сотовый тариф». Также можно набрать *#06#: наличие EID обычно подтверждает поддержку eSIM. Учтите, что некоторые модели для отдельных рынков выпускаются без eSIM."
	}
	if platform == "android" {
		text = "ТЕСТ / SANDBOX\n\nПроверка на Android\n\nОткройте Настройки → Сеть и интернет / Подключения → SIM-карты. Найдите пункт «Добавить eSIM», «Добавить мобильный тариф» или «Скачать SIM». Названия разделов зависят от производителя и версии Android."
	}

	buttons := [][]Button{}
	if quoteID != "" {
		context := quoteID + "|" + countryCode
		buttons = append(buttons, []Button{{Text: "Проверить iPhone", Data: "compat-ios:" + context}})
		buttons = append(buttons, []Button{{Text: "Проверить Android", Data: "compat-android:" + context}})
		buttons = append(buttons, []Button{{Text: "✅ Телефон поддерживает eSIM", Data: "confirm:" + quoteID}})
		buttons = append(buttons, []Button{{Text: "Назад к тарифам", Data: "country:" + countryCode}})
	} else {
		buttons = append(buttons, []Button{{Text: "Проверить iPhone", Data: "compatibility:ios"}})
		buttons = append(buttons, []Button{{Text: "Проверить Android", Data: "compatibility:android"}})
		buttons = append(buttons, []Button{{Text: "🌍 Выбрать страну", Data: "buy"}})
	}
	buttons = append(buttons, []Button{{Text: "В меню", Data: "menu"}})
	return Response{Text: text, Buttons: buttons}
}

func parseQuoteContext(input, prefix string) (string, string, error) {
	value := strings.TrimPrefix(input, prefix)
	separator := strings.LastIndexByte(value, '|')
	if separator <= 0 || separator == len(value)-1 {
		return "", "", errorsNew("не удалось открыть проверку совместимости")
	}
	return value[:separator], value[separator+1:], nil
}

func formatData(dataMB int) string {
	if dataMB <= 0 {
		return "Пакет интернета"
	}
	if dataMB >= 1024 {
		if dataMB%1024 == 0 {
			return fmt.Sprintf("%d ГБ", dataMB/1024)
		}
		return fmt.Sprintf("%.1f ГБ", float64(dataMB)/1024)
	}
	return fmt.Sprintf("%d МБ", dataMB)
}

func packageRecommendation(dataMB int) string {
	switch {
	case dataMB <= 0:
		return "Пакет для поездки"
	case dataMB <= 5*1024:
		return "Для мессенджеров и навигации"
	case dataMB <= 10*1024:
		return "Оптимальный выбор"
	default:
		return "Для активного использования"
	}
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
