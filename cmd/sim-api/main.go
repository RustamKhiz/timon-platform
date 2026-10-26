package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/httpapi"
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
		MinimumMargin:  50,
		QuoteTTL:       15 * time.Minute,
	}
	service := sim.NewService(pricing)
	storageMode := "memory"
	apiToken := os.Getenv("SIM_API_TOKEN")
	var pool *pgxpool.Pool
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
		service = sim.NewServiceWithRepository(pricing, postgres.NewStore(pool))
		storageMode = "postgres"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.New(service, logger, httpapi.Config{BotToken: apiToken, StorageMode: storageMode}),
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
	logger.Info("TIMON SIM mock API starting", "address", address, "markup_rub", markup, "storage", storageMode)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
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
