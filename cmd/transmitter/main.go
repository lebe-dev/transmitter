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
	"github.com/lebe-dev/transmitter/internal/notes"
	"github.com/lebe-dev/transmitter/internal/prefs"
	"github.com/lebe-dev/transmitter/internal/sentrylog"
	"github.com/lebe-dev/transmitter/internal/server"
	"github.com/lebe-dev/transmitter/internal/shift"
	"github.com/lebe-dev/transmitter/internal/transmission"
	"github.com/lebe-dev/transmitter/static"
)

var Version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg)
	logger.Info("starting transmitter", "version", Version)
	if cfg.SentryDSN != "" {
		defer sentrylog.Flush(2 * time.Second)
		logger.Info("sentry enabled", "environment", cfg.SentryEnvironment)
	}

	if !cfg.WebUIEnabled && !cfg.TelegramBotEnabled {
		logger.Error("both WEBUI_ENABLED and TELEGRAM_BOT_ENABLED are false — nothing to run")
		os.Exit(1)
	}

	client := transmission.NewClient(cfg.TransmissionURL, cfg.TransmissionUser, cfg.TransmissionPass)

	noteStore, err := notes.Open(cfg.DBPath, cfg.NoteMaxLength)
	if err != nil {
		logger.Error("notes database init failed", "path", cfg.DBPath, "err", err)
		os.Exit(1)
	}
	defer noteStore.Close() //nolint:errcheck
	logger.Info("notes database ready", "path", cfg.DBPath, "max_length", cfg.NoteMaxLength)

	// Shares the database file with the notes store; holds the shift toggles set
	// from the web UI.
	prefStore, err := prefs.Open(cfg.DBPath)
	if err != nil {
		logger.Error("preferences database init failed", "path", cfg.DBPath, "err", err)
		os.Exit(1)
	}
	defer prefStore.Close() //nolint:errcheck

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	tgBot := startBot(ctx, cfg, client, noteStore, logger)
	srv := startServer(cfg, client, noteStore, prefStore, logger, stop)

	go notes.NewCleaner(noteStore, client, cfg.NoteCleanupInterval, logger).Run(ctx)

	if cfg.NightShiftEnabled {
		go shift.New(client, shift.NightOptions(cfg), prefStore, logger).Run(ctx)
	} else {
		logger.Info("night-shift not configured (NIGHT_SHIFT_START/NIGHT_SHIFT_END not set)")
	}

	if cfg.DayShiftEnabled {
		go shift.New(client, shift.DayOptions(cfg), prefStore, logger).Run(ctx)
	} else {
		logger.Info("day-shift not configured (DAY_SHIFT_START/DAY_SHIFT_END not set)")
	}

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if tgBot != nil {
		tgBot.Stop()
	}
	if srv != nil {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "err", err)
		}
	}
	logger.Info("shutdown complete")
}

// setupLogger builds the application logger, wiring in Sentry when a DSN is configured.
func setupLogger(cfg *config.Config) *slog.Logger {
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	if cfg.SentryDSN != "" {
		if err := sentrylog.Init(cfg.SentryDSN, cfg.SentryEnvironment, Version); err != nil {
			slog.Error("sentry init failed", "err", err)
			os.Exit(1)
		}
		handler = sentrylog.NewHandler(handler, nil)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// startBot initializes and starts the Telegram bot when enabled, returning nil when disabled.
func startBot(ctx context.Context, cfg *config.Config, client *transmission.Client, noteStore bot.NoteSource, logger *slog.Logger) *bot.Bot {
	if !cfg.TelegramBotEnabled {
		logger.Info("telegram bot disabled (TELEGRAM_BOT_ENABLED=false)")
		return nil
	}
	if cfg.TelegramToken == "" {
		logger.Error("TELEGRAM_BOT_ENABLED=true but TELEGRAM_TOKEN is not set")
		os.Exit(1)
	}

	tgBot, err := bot.New(cfg.TelegramToken, cfg.TelegramUsers, client, noteStore, logger, cfg.FilePriorityEnabled, cfg.FilePriorityHighCount, cfg.FileSelectTimeout)
	if err != nil {
		logger.Error("bot init failed", "err", err)
		os.Exit(1)
	}
	go tgBot.Start()
	go tgBot.StartMonitor(ctx, cfg.MonitorInterval)
	return tgBot
}

// startServer initializes and starts the HTTP server when enabled, returning nil when disabled.
func startServer(cfg *config.Config, client *transmission.Client, noteStore server.NoteStore, shiftStore server.ShiftStore, logger *slog.Logger, stop context.CancelFunc) *server.Server {
	if !cfg.WebUIEnabled {
		logger.Info("web UI disabled (WEBUI_ENABLED=false)")
		return nil
	}

	srv, err := server.New(cfg, client, noteStore, shiftStore, static.FS, logger)
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
	return srv
}
