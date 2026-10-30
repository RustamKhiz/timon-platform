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
	"time"

	"github.com/RustamKhiz/timon-platform/internal/adminauth"
	"github.com/RustamKhiz/timon-platform/internal/sim"
)

type API struct {
	service *sim.Service
	logger  *slog.Logger
	config  Config
}

type Config struct {
	BotToken          string
	StorageMode       string
	ProviderMode      string
	AdminAuth         *adminauth.Manager
	AdminCookieSecure bool
}

const adminSessionCookie = "timon_sim_admin_session"

func New(service *sim.Service, logger *slog.Logger, configs ...Config) http.Handler {
	config := Config{StorageMode: "memory"}
	if len(configs) > 0 {
		config = configs[0]
	}
	api := &API{service: service, logger: logger, config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /api/v1/sim/countries", api.countries)
	mux.HandleFunc("GET /api/v1/sim/packages", api.packages)
	mux.HandleFunc("POST /api/v1/sim/quotes", api.createQuote)
	mux.HandleFunc("POST /api/v1/sim/orders", api.createOrder)
	mux.HandleFunc("GET /api/v1/sim/orders", api.listOrders)
	mux.HandleFunc("GET /api/v1/sim/orders/{id}", api.getOrder)
	mux.HandleFunc("POST /api/v1/sim/orders/{id}/mock-payment", api.mockPayment)
	mux.HandleFunc("POST /api/v1/sim/orders/{id}/delivered", api.markDelivered)
	mux.HandleFunc("POST /api/v1/admin/auth/login", api.adminLogin)
	mux.HandleFunc("POST /api/v1/admin/auth/logout", api.adminLogout)
	mux.Handle("GET /api/v1/admin/session", api.requireAdmin(http.HandlerFunc(api.adminSession)))
	mux.Handle("GET /api/v1/admin/orders", api.requireAdmin(http.HandlerFunc(api.adminOrders)))
	mux.Handle("GET /api/v1/admin/orders/{id}", api.requireAdmin(http.HandlerFunc(api.adminOrder)))
	return logging(logger, authenticate(config.BotToken, mux))
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	providerMode := a.config.ProviderMode
	if providerMode == "" {
		providerMode = "mock"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "mock-fulfillment", "provider": providerMode, "storage": a.config.StorageMode})
}

func (a *API) countries(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"countries":       a.service.Countries(),
		"catalog_is_mock": !a.service.UsesProviderCatalog(),
	})
}

func (a *API) packages(w http.ResponseWriter, r *http.Request) {
	packages, err := a.service.Packages(r.Context(), r.URL.Query().Get("country_code"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": packages})
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
	for index := range orders {
		orders[index].Activation = nil
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

func (a *API) adminLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.config.AdminAuth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin access is not configured"})
		return
	}
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !a.config.AdminAuth.Authenticate(request.Username, request.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	now := time.Now().UTC()
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: a.config.AdminAuth.Issue(now), Path: "/api/v1/admin",
		MaxAge: int(a.config.AdminAuth.TTL().Seconds()), HttpOnly: true, Secure: a.config.AdminCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": request.Username})
}

func (a *API) adminLogout(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: "", Path: "/api/v1/admin", MaxAge: -1,
		HttpOnly: true, Secure: a.config.AdminCookieSecure, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) adminSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (a *API) adminOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := a.service.AdminOrders(r.Context(), sim.AdminOrderFilter{
		Query: r.URL.Query().Get("q"), Status: r.URL.Query().Get("status"),
		CountryCode: r.URL.Query().Get("country_code"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (a *API) adminOrder(w http.ResponseWriter, r *http.Request) {
	order, err := a.service.AdminOrder(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if a.config.AdminAuth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin access is not configured"})
			return
		}
		cookie, err := r.Cookie(adminSessionCookie)
		if err != nil || a.config.AdminAuth.Verify(cookie.Value, time.Now().UTC()) != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
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
	} else if errors.Is(err, sim.ErrInvalidAdminFilter) {
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
