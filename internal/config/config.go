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
	MaxRequestBodyBytes   int64
	FileSelectTimeout     time.Duration
	DeleteWithData        bool
	NightShiftEnabled     bool
	NightShiftStart       DayTime
	NightShiftEnd         DayTime
	NightShiftInterval    time.Duration
	SentryDSN             string
	SentryEnvironment     string
}

// DayTime represents a wall-clock time of day (hours and minutes, 0..24h-1m).
type DayTime struct {
	Hour   int
	Minute int
}

// Minutes returns the time-of-day as minutes since midnight.
func (d DayTime) Minutes() int {
	return d.Hour*60 + d.Minute
}

// String formats as HH:MM.
func (d DayTime) String() string {
	return fmt.Sprintf("%02d:%02d", d.Hour, d.Minute)
}

// Load reads configuration from environment variables, optionally loading a .env file first.
func Load() (*Config, error) {
	if err := loadDotenv(); err != nil {
		return nil, err
	}

	cfg := &Config{
		TransmissionURL:  envOrDefault("TRANSMISSION_URL", "http://localhost:9091/transmission/rpc"),
		TransmissionUser: os.Getenv("TRANSMISSION_USER"),
		TransmissionPass: os.Getenv("TRANSMISSION_PASS"),
		ListenAddr:       envOrDefault("LISTEN_ADDR", "127.0.0.1:8080"),
		CORSOrigin:       envOrDefault("CORS_ORIGIN", "http://localhost:8080"),
		TelegramToken:    os.Getenv("TELEGRAM_TOKEN"),
	}

	if cfg.TransmissionUser == "" {
		return nil, fmt.Errorf("TRANSMISSION_USER is required")
	}
	if cfg.TransmissionPass == "" {
		return nil, fmt.Errorf("TRANSMISSION_PASS is required")
	}

	cfg.TelegramUsers = parseTelegramUsers(os.Getenv("TELEGRAM_USERS"))
	cfg.LogLevel = parseLogLevel(os.Getenv("LOG_LEVEL"))
	cfg.MonitorInterval = parseDuration(os.Getenv("MONITOR_INTERVAL"), 30*time.Second)
	cfg.FilePriorityEnabled = strings.EqualFold(os.Getenv("FILE_PRIORITY_ENABLED"), "true")
	cfg.FilePriorityHighCount = parsePositiveInt(os.Getenv("FILE_PRIORITY_HIGH_COUNT"), 3)
	cfg.WebUIEnabled = parseBoolDefault(os.Getenv("WEBUI_ENABLED"), true)
	cfg.TelegramBotEnabled = parseBoolDefault(os.Getenv("TELEGRAM_BOT_ENABLED"), false)
	cfg.MaxRequestBodyBytes = parsePositiveInt64(os.Getenv("MAX_REQUEST_BODY_BYTES"), 10<<20)
	cfg.FileSelectTimeout = parseDuration(os.Getenv("FILE_SELECT_TIMEOUT"), 5*time.Minute)
	cfg.DeleteWithData = strings.EqualFold(os.Getenv("DELETE_WITH_DATA"), "true")
	cfg.NightShiftInterval = parseDuration(os.Getenv("NIGHT_SHIFT_INTERVAL"), time.Minute)

	cfg.SentryDSN = strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	cfg.SentryEnvironment = strings.TrimSpace(os.Getenv("SENTRY_ENVIRONMENT"))
	if cfg.SentryDSN != "" && cfg.SentryEnvironment == "" {
		return nil, fmt.Errorf("SENTRY_ENVIRONMENT is required when SENTRY_DSN is set")
	}

	if err := parseNightShift(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadDotenv loads a .env file if present, ignoring a missing file but failing on parse errors.
func loadDotenv() error {
	err := godotenv.Load()
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// godotenv returns a plain error for a missing file, not os.ErrNotExist.
	if strings.Contains(err.Error(), "no such file") {
		return nil
	}
	return fmt.Errorf("load .env: %w", err)
}

// parseTelegramUsers splits a comma-separated user list, trimming whitespace and a leading '@'.
func parseTelegramUsers(usersStr string) []string {
	if usersStr == "" {
		return nil
	}
	var users []string
	for _, part := range strings.Split(usersStr, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "@")
		if part == "" {
			continue
		}
		users = append(users, part)
	}
	return users
}

// parseNightShift reads the NIGHT_SHIFT_START/END window into cfg, enabling it when both are set.
func parseNightShift(cfg *Config) error {
	startRaw := strings.TrimSpace(os.Getenv("NIGHT_SHIFT_START"))
	endRaw := strings.TrimSpace(os.Getenv("NIGHT_SHIFT_END"))
	if startRaw == "" || endRaw == "" {
		return nil
	}

	start, err := parseDayTime(startRaw)
	if err != nil {
		return fmt.Errorf("NIGHT_SHIFT_START: %w", err)
	}
	end, err := parseDayTime(endRaw)
	if err != nil {
		return fmt.Errorf("NIGHT_SHIFT_END: %w", err)
	}
	if start == end {
		return fmt.Errorf("NIGHT_SHIFT_START and NIGHT_SHIFT_END must differ")
	}

	cfg.NightShiftEnabled = true
	cfg.NightShiftStart = start
	cfg.NightShiftEnd = end
	return nil
}

func parseDayTime(s string) (DayTime, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return DayTime{}, fmt.Errorf("expected HH:MM, got %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return DayTime{}, fmt.Errorf("invalid hour in %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return DayTime{}, fmt.Errorf("invalid minute in %q", s)
	}
	return DayTime{Hour: h, Minute: m}, nil
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

func parsePositiveInt64(s string, def int64) int64 {
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
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
