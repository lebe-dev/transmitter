package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	TransmissionURL  string
	TransmissionUser string
	TransmissionPass string
	ListenAddr       string
	CORSOrigin       string
	TelegramToken    string
	TelegramUsers    []string
	LogLevel         slog.Level
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

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
