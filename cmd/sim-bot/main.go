package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/RustamKhiz/timon-platform/internal/bot"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		logger.Error("TELEGRAM_BOT_TOKEN is required; keep it only in the environment or an ignored .env file")
		os.Exit(2)
	}
	apiURL := os.Getenv("SIM_API_URL")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:18080"
	}
	allowedUserIDs, err := bot.ParseAllowedUserIDs(os.Getenv("TELEGRAM_ALLOWED_USER_IDS"))
	if err != nil {
		logger.Error("TELEGRAM_ALLOWED_USER_IDS is required for the closed test bot", "error", err)
		os.Exit(2)
	}
	client := bot.NewHTTPClient(apiURL, os.Getenv("SIM_API_TOKEN"))
	engine := bot.NewEngine(client)
	telegram := bot.NewTelegram(token, engine, client, allowedUserIDs, logger)
	if offsetFile := os.Getenv("TELEGRAM_OFFSET_FILE"); offsetFile != "" {
		telegram.SetOffsetStore(bot.NewFileOffsetStore(offsetFile))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("TIMON SIM bot starting", "api_url", apiURL, "mode", "mock")
	if err := telegram.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}
