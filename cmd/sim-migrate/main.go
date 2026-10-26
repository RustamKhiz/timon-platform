package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	databaseURL := os.Getenv("SIM_DATABASE_URL")
	if databaseURL == "" {
		logger.Error("SIM_DATABASE_URL is required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("could not configure PostgreSQL connection")
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("could not connect to PostgreSQL")
		os.Exit(1)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("TIMON SIM migrations applied")
}
