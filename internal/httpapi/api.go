package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

type API struct {
	service *sim.Service
	logger  *slog.Logger
	config  Config
}

type Config struct {
	BotToken    string
	StorageMode string
}

func New(service *sim.Service, logger *slog.Logger, configs ...Config) http.Handler {
	config := Config{StorageMode: "memory"}
	if len(configs) > 0 {
		config = configs[0]
	}
	api := &API{service: service, logger: logger, config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /api/v1/sim/countries", api.countries)
	mux.HandleFunc("POST /api/v1/sim/quotes", api.createQuote)
	mux.HandleFunc("POST /api/v1/sim/orders", api.createOrder)
	mux.HandleFunc("GET /api/v1/sim/orders", api.listOrders)
	mux.HandleFunc("GET /api/v1/sim/orders/{id}", api.getOrder)
	mux.HandleFunc("POST /api/v1/sim/orders/{id}/mock-payment", api.mockPayment)
	mux.HandleFunc("POST /api/v1/sim/orders/{id}/delivered", api.markDelivered)
	return logging(logger, authenticate(config.BotToken, mux))
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "mock", "storage": a.config.StorageMode})
}

func (a *API) countries(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"countries":               a.service.Countries(),
		"duration_days":           map[string]int{"min": 1, "max": 30},
		"gigabytes_per_day":       map[string]int{"min": 1, "max": 5},
		"catalog_is_mock":         true,
		"provider_specific_later": true,
	})
}

func (a *API) createQuote(w http.ResponseWriter, r *http.Request) {
	var selection sim.Selection
	if err := decodeJSON(r, &selection); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	quote, err := a.service.CreateQuote(r.Context(), selection)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, quote)
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var request struct {
		QuoteID        string `json:"quote_id"`
		TelegramUserID int64  `json:"telegram_user_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	order, err := a.service.CreateOrder(r.Context(), request.QuoteID, request.TelegramUserID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (a *API) listOrders(w http.ResponseWriter, r *http.Request) {
	userID, err := telegramUserID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	orders, err := a.service.OrdersForUser(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	userID, err := telegramUserID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	order, err := a.service.GetOrder(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) mockPayment(w http.ResponseWriter, r *http.Request) {
	userID, err := telegramUserIDFromBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	order, err := a.service.MockPayAndProvision(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) markDelivered(w http.ResponseWriter, r *http.Request) {
	userID, err := telegramUserIDFromBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	order, err := a.service.MarkDelivered(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func telegramUserID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.URL.Query().Get("telegram_user_id"), 10, 64)
}

func telegramUserIDFromBody(r *http.Request) (int64, error) {
	var request struct {
		TelegramUserID int64 `json:"telegram_user_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		return 0, err
	}
	if request.TelegramUserID == 0 {
		return 0, errors.New("telegram_user_id is required")
	}
	return request.TelegramUserID, nil
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	if errors.Is(err, sim.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, sim.ErrForbidden) {
		status = http.StatusForbidden
	} else if errors.Is(err, sim.ErrIdempotencyConflict) {
		status = http.StatusConflict
	} else if errors.Is(err, sim.ErrIdempotencyRequired) {
		status = http.StatusBadRequest
	}
	writeError(w, status, err)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func logging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if !strings.HasSuffix(r.URL.Path, "/healthz") {
			logger.Info("request", "method", r.Method, "path", r.URL.Path)
		}
	})
}

func authenticate(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || !strings.HasPrefix(r.URL.Path, "/api/v1/sim/") {
			next.ServeHTTP(w, r)
			return
		}
		const prefix = "Bearer "
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authorization, prefix)), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
