package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type Telegram struct {
	token          string
	engine         *Engine
	client         *HTTPClient
	allowedUserIDs map[int64]struct{}
	logger         *slog.Logger
	http           *http.Client
	offsets        OffsetStore
}

func ParseAllowedUserIDs(value string) (map[int64]struct{}, error) {
	allowed := make(map[int64]struct{})
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		userID, err := strconv.ParseInt(part, 10, 64)
		if err != nil || userID <= 0 {
			return nil, fmt.Errorf("invalid Telegram user ID")
		}
		allowed[userID] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("at least one Telegram user ID is required")
	}
	return allowed, nil
}

func NewTelegram(token string, engine *Engine, client *HTTPClient, allowedUserIDs map[int64]struct{}, logger *slog.Logger) *Telegram {
	return &Telegram{token: token, engine: engine, client: client, allowedUserIDs: allowedUserIDs, logger: logger, http: &http.Client{Timeout: 65 * time.Second}, offsets: memoryOffsetStore{}}
}

func (t *Telegram) SetOffsetStore(store OffsetStore) {
	if store != nil {
		t.offsets = store
	}
}

func (t *Telegram) Run(ctx context.Context) error {
	offset, err := t.offsets.Load()
	if err != nil {
		return fmt.Errorf("load Telegram offset: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := t.getUpdates(ctx, offset)
		if err != nil {
			t.logger.Error("getUpdates failed", "error", err)
			time.Sleep(time.Second)
			continue
		}
		for _, update := range updates {
			if err := t.handleUpdate(ctx, update); err != nil {
				t.logger.Error("update failed", "update_id", update.ID, "error", err)
			}
			offset = update.ID + 1
			if err := t.offsets.Save(offset); err != nil {
				return fmt.Errorf("save Telegram offset: %w", err)
			}
		}
	}
}

type telegramUpdate struct {
	ID      int64 `json:"update_id"`
	Message *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
	} `json:"message"`
	Callback *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

func (t *Telegram) handleUpdate(ctx context.Context, update telegramUpdate) error {
	var chatID, userID int64
	var input string
	if update.Callback != nil {
		chatID = update.Callback.Message.Chat.ID
		userID = update.Callback.From.ID
		input = update.Callback.Data
		_ = t.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": update.Callback.ID}, nil)
	} else if update.Message != nil {
		chatID = update.Message.Chat.ID
		userID = update.Message.From.ID
		input = update.Message.Text
	} else {
		return nil
	}
	if _, allowed := t.allowedUserIDs[userID]; !allowed {
		if update.Callback != nil {
			_ = t.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": update.Callback.ID}, nil)
		}
		return t.send(ctx, chatID, Response{Text: "ТЕСТ / MOCK\n\nДоступ к тестовому боту закрыт."})
	}

	response, err := t.engine.Handle(ctx, userID, input)
	if err != nil {
		t.logger.Error("bot action failed", "input_kind", inputKind(input), "error", err)
		return t.send(ctx, chatID, safeErrorResponse(input))
	}
	if err := t.send(ctx, chatID, response); err != nil {
		return err
	}
	if response.DeliveredOrderID != "" {
		if err := t.client.MarkDelivered(ctx, response.DeliveredOrderID, userID); err != nil {
			t.logger.Error("could not mark order delivered", "order_id", response.DeliveredOrderID, "error", err)
		}
	}
	return nil
}

func (t *Telegram) send(ctx context.Context, chatID int64, response Response) error {
	keyboard := make([][]map[string]string, 0, len(response.Buttons))
	for _, row := range response.Buttons {
		telegramRow := make([]map[string]string, 0, len(row))
		for _, button := range row {
			telegramRow = append(telegramRow, map[string]string{"text": button.Text, "callback_data": button.Data})
		}
		keyboard = append(keyboard, telegramRow)
	}
	replyMarkup := map[string]any{}
	if len(response.ReplyKeyboard) > 0 {
		replyRows := make([][]map[string]string, 0, len(response.ReplyKeyboard))
		for _, row := range response.ReplyKeyboard {
			replyRow := make([]map[string]string, 0, len(row))
			for _, label := range row {
				replyRow = append(replyRow, map[string]string{"text": label})
			}
			replyRows = append(replyRows, replyRow)
		}
		replyMarkup = map[string]any{"keyboard": replyRows, "resize_keyboard": true, "is_persistent": true}
	} else if len(keyboard) > 0 {
		replyMarkup = map[string]any{"inline_keyboard": keyboard}
	}
	if len(response.Photo) > 0 {
		return t.sendPhoto(ctx, chatID, response, replyMarkup)
	}
	payload := map[string]any{"chat_id": chatID, "text": response.Text}
	if len(replyMarkup) > 0 {
		payload["reply_markup"] = replyMarkup
	}
	return t.call(ctx, "sendMessage", payload, nil)
}

func safeErrorResponse(input string) Response {
	buttons := [][]Button{}
	if retryableInput(input) && len(input) <= 64 {
		buttons = append(buttons, []Button{{Text: "Повторить", Data: input}})
	}
	buttons = append(buttons, []Button{{Text: "В меню", Data: "menu"}})
	buttons = append(buttons, []Button{{Text: "Поддержка", Data: "support"}})
	return Response{Text: "Не удалось выполнить действие. Попробуйте ещё раз через несколько секунд. Если ошибка повторяется, обратитесь в поддержку.", Buttons: buttons}
}

func retryableInput(input string) bool {
	if input == "buy" || input == "my" {
		return true
	}
	prefixes := []string{"country:", "package:", "compat:", "confirm:", "pay:", "esim:", "details:", "guide-manual:"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(input, prefix) {
			return true
		}
	}
	return false
}

func inputKind(input string) string {
	if separator := strings.IndexByte(input, ':'); separator > 0 {
		return input[:separator]
	}
	if strings.HasPrefix(input, "/") {
		return "command"
	}
	switch input {
	case "buy", "my", "menu", "support", "faq", "how", "compatibility":
		return input
	default:
		return "text"
	}
}

func (t *Telegram) sendPhoto(ctx context.Context, chatID int64, response Response, replyMarkup map[string]any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return err
	}
	if err := writer.WriteField("caption", response.Text); err != nil {
		return err
	}
	if len(replyMarkup) > 0 {
		data, err := json.Marshal(replyMarkup)
		if err != nil {
			return err
		}
		if err := writer.WriteField("reply_markup", string(data)); err != nil {
			return err
		}
	}
	filename := response.PhotoFilename
	if filename == "" {
		filename = "qr.png"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="photo"; filename=%q`, filename))
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := part.Write(response.Photo); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+t.token+"/sendPhoto", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	telegramResponse, err := t.http.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("Telegram API sendPhoto request failed")
	}
	defer telegramResponse.Body.Close()
	if telegramResponse.StatusCode < 200 || telegramResponse.StatusCode >= 300 {
		return fmt.Errorf("Telegram API sendPhoto returned %s", telegramResponse.Status)
	}
	return nil
}

func (t *Telegram) getUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	var response struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 50, "allowed_updates": []string{"message", "callback_query"}}, &response)
	return response.Result, err
}

func (t *Telegram) call(ctx context.Context, method string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+t.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := t.http.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("Telegram API %s request failed", method)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram API %s returned %s", method, response.Status)
	}
	if target == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(target)
}
