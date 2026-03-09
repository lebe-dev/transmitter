package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lebe-dev/transmitter/internal/bot"
	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/server"
	"github.com/lebe-dev/transmitter/internal/transmission"
	"github.com/lebe-dev/transmitter/static"
)

const Version = "0.1.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	logger.Info("starting transmitter", "version", Version)

	client := transmission.NewClient(cfg.TransmissionURL, cfg.TransmissionUser, cfg.TransmissionPass)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	var tgBot *bot.Bot
	if cfg.TelegramToken != "" {
		tgBot, err = bot.New(cfg.TelegramToken, cfg.TelegramUsers, client, logger)
		if err != nil {
			logger.Error("bot init failed", "err", err)
			os.Exit(1)
		}
		go tgBot.Start()
	} else {
		logger.Info("telegram bot disabled (TELEGRAM_TOKEN not set)")
	}

	srv, err := server.New(cfg, client, static.FS, logger)
	if err != nil {
		logger.Error("server init failed", "err", err)
		os.Exit(1)
	}

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if tgBot != nil {
		tgBot.Stop()
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
	logger.Info("shutdown complete")
}
