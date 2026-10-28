package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/fx"
	"github.com/RustamKhiz/timon-platform/internal/httpapi"
	"github.com/RustamKhiz/timon-platform/internal/keepgo"
	"github.com/RustamKhiz/timon-platform/internal/postgres"
	"github.com/RustamKhiz/timon-platform/internal/sim"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	address := envOrDefault("SIM_API_ADDR", "127.0.0.1:18080")
	markup := intEnvOrDefault("SIM_FIXED_MARKUP_RUB", 100)
	pricing := sim.PricingConfig{
		FixedMarkupRUB: markup,
		ExpectedFeeBPS: intEnvOrDefault("SIM_EXPECTED_PAYMENT_FEE_BPS", 350),
		MinimumMargin:  50,
		QuoteTTL:       15 * time.Minute,
	}
	storageMode := "memory"
	providerMode := envOrDefault("SIM_PROVIDER_MODE", "mock")
	apiToken := os.Getenv("SIM_API_TOKEN")
	var pool *pgxpool.Pool
	var repository sim.Repository
	if databaseURL := os.Getenv("SIM_DATABASE_URL"); databaseURL != "" {
		if apiToken == "" {
			logger.Error("SIM_API_TOKEN is required with PostgreSQL storage")
			os.Exit(2)
		}
		connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		pool, err = pgxpool.New(connectCtx, databaseURL)
		if err != nil {
			logger.Error("could not configure PostgreSQL connection")
			os.Exit(1)
		}
		if err := pool.Ping(connectCtx); err != nil {
			logger.Error("could not connect to PostgreSQL")
			os.Exit(1)
		}
		defer pool.Close()
		repository = postgres.NewStore(pool)
		storageMode = "postgres"
	}
	var catalog sim.Catalog
	switch providerMode {
	case "mock":
	case "keepgo-sandbox":
		baseURL := envOrDefault("KEEPGO_BASE_URL", keepgo.SandboxBaseURL)
		if strings.TrimRight(baseURL, "/") != keepgo.SandboxBaseURL {
			logger.Error("keepgo-sandbox mode only accepts the sandbox API URL")
			os.Exit(2)
		}
		client, err := keepgo.NewClient(keepgo.Config{BaseURL: baseURL, APIKey: os.Getenv("KEEPGO_API_KEY"), AccessToken: os.Getenv("KEEPGO_ACCESS_TOKEN")})
		if err != nil {
			logger.Error("could not configure Keepgo sandbox client", "error", err)
			os.Exit(2)
		}
		fxProvider, err := fx.NewCBRUSDProvider(fx.Config{
			BufferBPS:    intEnvOrDefault("SIM_FX_BUFFER_BPS", 500),
			FallbackRate: floatEnv("SIM_USD_RUB_RATE"),
		})
		if err != nil {
			logger.Error("could not configure CBR FX client", "error", err)
			os.Exit(2)
		}
		keepgoCatalog, err := keepgo.NewCatalog(client, fxProvider, 5*time.Minute)
		if err != nil {
			logger.Error("could not configure Keepgo sandbox catalogue", "error", err)
			os.Exit(2)
		}
		catalog = keepgoCatalog
	default:
		logger.Error("unsupported SIM_PROVIDER_MODE", "mode", providerMode)
		os.Exit(2)
	}
	service := sim.NewServiceWithCatalog(pricing, repository, catalog)
	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.New(service, logger, httpapi.Config{BotToken: apiToken, StorageMode: storageMode, ProviderMode: providerMode}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("API graceful shutdown failed", "error", err)
		}
	}()
	logger.Info("TIMON SIM API starting", "address", address, "markup_rub", markup, "storage", storageMode, "provider_mode", providerMode, "fulfillment", "mock")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func floatEnv(name string) float64 {
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func intEnvOrDefault(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
