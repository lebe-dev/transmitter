package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	TransmissionURL       string
	TransmissionUser      string
	TransmissionPass      string
	ListenAddr            string
	CORSOrigin            string
	TelegramToken         string
	TelegramUsers         []string
	LogLevel              slog.Level
	MonitorInterval       time.Duration
	FilePriorityEnabled   bool
	FilePriorityHighCount int
	WebUIEnabled          bool
	TelegramBotEnabled    bool
}

// Load reads configuration from environment variables, optionally loading a .env file first.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// godotenv returns a plain error for missing file, not os.ErrNotExist
		// Only fail if the file exists but can't be parsed
		if !strings.Contains(err.Error(), "no such file") {
			return nil, fmt.Errorf("load .env: %w", err)
		}
	}

	cfg := &Config{
		TransmissionURL:  envOrDefault("TRANSMISSION_URL", "http://localhost:9091/transmission/rpc"),
		TransmissionUser: os.Getenv("TRANSMISSION_USER"),
		TransmissionPass: os.Getenv("TRANSMISSION_PASS"),
		ListenAddr:       envOrDefault("LISTEN_ADDR", ":8080"),
		CORSOrigin:       envOrDefault("CORS_ORIGIN", "http://localhost:8080"),
		TelegramToken:    os.Getenv("TELEGRAM_TOKEN"),
	}

	if cfg.TransmissionUser == "" {
		return nil, fmt.Errorf("TRANSMISSION_USER is required")
	}
	if cfg.TransmissionPass == "" {
		return nil, fmt.Errorf("TRANSMISSION_PASS is required")
	}

	if usersStr := os.Getenv("TELEGRAM_USERS"); usersStr != "" {
		for _, part := range strings.Split(usersStr, ",") {
			part = strings.TrimSpace(part)
			part = strings.TrimPrefix(part, "@")
			if part == "" {
				continue
			}
			cfg.TelegramUsers = append(cfg.TelegramUsers, part)
		}
	}

	cfg.LogLevel = parseLogLevel(os.Getenv("LOG_LEVEL"))
	cfg.MonitorInterval = parseDuration(os.Getenv("MONITOR_INTERVAL"), 30*time.Second)
	cfg.FilePriorityEnabled = strings.EqualFold(os.Getenv("FILE_PRIORITY_ENABLED"), "true")
	cfg.FilePriorityHighCount = parsePositiveInt(os.Getenv("FILE_PRIORITY_HIGH_COUNT"), 3)
	cfg.WebUIEnabled = parseBoolDefault(os.Getenv("WEBUI_ENABLED"), true)
	cfg.TelegramBotEnabled = parseBoolDefault(os.Getenv("TELEGRAM_BOT_ENABLED"), false)

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func parseBoolDefault(s string, def bool) bool {
	if s == "" {
		return def
	}
	return strings.EqualFold(s, "true") || s == "1"
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
