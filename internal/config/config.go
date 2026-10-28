package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	// defaultNoteMaxLength caps a per-torrent note at this many runes.
	defaultNoteMaxLength = 200
	// defaultNoteCleanupInterval is how often notes of deleted torrents are collected.
	defaultNoteCleanupInterval = time.Hour
	// defaultImpactTargetRatio is the upload ratio counted as "gave back everything
	// we took": uploaded as much as was downloaded.
	defaultImpactTargetRatio = 1.0
	// defaultImpactInterval is how often the impact worker looks for torrents that
	// reached the target ratio.
	defaultImpactInterval = 5 * time.Minute
	// defaultMovieLanguage is the TMDB locale for titles and descriptions.
	defaultMovieLanguage = "en-US"
	// defaultMovieRenameTemplate gives Plex/Jellyfin-friendly names.
	defaultMovieRenameTemplate = "{title} ({year})"
	// defaultMovieScanInterval is how often the movie plugin looks for new torrents.
	defaultMovieScanInterval = time.Minute
	// defaultMovieScanBatch caps the lookups per round of the movie plugin.
	defaultMovieScanBatch = 10
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
	DayShiftEnabled       bool
	DayShiftStart         DayTime
	DayShiftEnd           DayTime
	DayShiftInterval      time.Duration
	ImpactTargetRatio     float64
	ImpactInterval        time.Duration
	DBPath                string
	NoteMaxLength         int
	NoteCleanupInterval   time.Duration
	SentryDSN             string
	SentryEnvironment     string
	Movie                 MovieConfig
}

// MovieConfig configures the Movie Plugin: title detection on TMDB, posters,
// renaming and the watched / missing subtitles marks.
type MovieConfig struct {
	Enabled      bool
	TMDBAPIKey   string
	TMDBAPIURL   string
	TMDBImageURL string
	Language     string
	AutoDetect   bool
	Backfill     bool
	AutoRename   bool
	Template     string
	ScanInterval time.Duration
	ScanBatch    int
	PosterDir    string
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
	cfg.DayShiftInterval = parseDuration(os.Getenv("DAY_SHIFT_INTERVAL"), time.Minute)
	cfg.ImpactTargetRatio = parsePositiveFloat(os.Getenv("IMPACT_TARGET_RATIO"), defaultImpactTargetRatio)
	cfg.ImpactInterval = parseDuration(os.Getenv("IMPACT_INTERVAL"), defaultImpactInterval)
	cfg.DBPath = envOrDefault("DB_PATH", "data/transmitter.db")
	cfg.NoteMaxLength = parsePositiveInt(os.Getenv("TORRENT_NOTE_MAX_LENGTH"), defaultNoteMaxLength)
	cfg.NoteCleanupInterval = parseDuration(os.Getenv("TORRENT_NOTE_CLEANUP_INTERVAL"), defaultNoteCleanupInterval)

	cfg.SentryDSN = strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	cfg.SentryEnvironment = strings.TrimSpace(os.Getenv("SENTRY_ENVIRONMENT"))
	if cfg.SentryDSN != "" && cfg.SentryEnvironment == "" {
		return nil, fmt.Errorf("SENTRY_ENVIRONMENT is required when SENTRY_DSN is set")
	}

	movie, err := parseMovieConfig(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	cfg.Movie = movie

	nightStart, nightEnd, nightEnabled, err := parseShiftWindow("NIGHT_SHIFT")
	if err != nil {
		return nil, err
	}
	cfg.NightShiftEnabled, cfg.NightShiftStart, cfg.NightShiftEnd = nightEnabled, nightStart, nightEnd

	dayStart, dayEnd, dayEnabled, err := parseShiftWindow("DAY_SHIFT")
	if err != nil {
		return nil, err
	}
	cfg.DayShiftEnabled, cfg.DayShiftStart, cfg.DayShiftEnd = dayEnabled, dayStart, dayEnd

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

// parseMovieConfig reads the MOVIE_* and TMDB_* variables. Posters are cached
// next to the database unless MOVIE_POSTER_DIR says otherwise.
func parseMovieConfig(dbPath string) (MovieConfig, error) {
	mc := MovieConfig{
		Enabled:      parseBoolDefault(os.Getenv("MOVIE_PLUGIN_ENABLED"), false),
		TMDBAPIKey:   strings.TrimSpace(os.Getenv("TMDB_API_KEY")),
		TMDBAPIURL:   envOrDefault("TMDB_API_URL", "https://api.themoviedb.org/3"),
		TMDBImageURL: envOrDefault("TMDB_IMAGE_URL", "https://image.tmdb.org/t/p"),
		Language:     envOrDefault("MOVIE_LANGUAGE", defaultMovieLanguage),
		AutoDetect:   parseBoolDefault(os.Getenv("MOVIE_AUTO_DETECT"), true),
		Backfill:     parseBoolDefault(os.Getenv("MOVIE_BACKFILL"), true),
		AutoRename:   parseBoolDefault(os.Getenv("MOVIE_AUTO_RENAME"), true),
		Template:     envOrDefault("MOVIE_RENAME_TEMPLATE", defaultMovieRenameTemplate),
		ScanInterval: parseDuration(os.Getenv("MOVIE_SCAN_INTERVAL"), defaultMovieScanInterval),
		ScanBatch:    parsePositiveInt(os.Getenv("MOVIE_SCAN_BATCH"), defaultMovieScanBatch),
		PosterDir:    envOrDefault("MOVIE_POSTER_DIR", filepath.Join(filepath.Dir(dbPath), "posters")),
	}
	if !mc.Enabled {
		return mc, nil
	}
	if mc.TMDBAPIKey == "" {
		return MovieConfig{}, fmt.Errorf("TMDB_API_KEY is required when MOVIE_PLUGIN_ENABLED=true")
	}
	// A template without the title would give every torrent the same name.
	if !strings.Contains(mc.Template, "{title}") && !strings.Contains(mc.Template, "{original_title}") {
		return MovieConfig{}, fmt.Errorf("MOVIE_RENAME_TEMPLATE must contain {title} or {original_title}")
	}
	return mc, nil
}

// parseShiftWindow reads the <prefix>_START/<prefix>_END window, e.g. NIGHT_SHIFT or DAY_SHIFT.
// The shift is enabled only when both variables are set.
func parseShiftWindow(prefix string) (start, end DayTime, enabled bool, err error) {
	startVar, endVar := prefix+"_START", prefix+"_END"
	startRaw := strings.TrimSpace(os.Getenv(startVar))
	endRaw := strings.TrimSpace(os.Getenv(endVar))
	if startRaw == "" || endRaw == "" {
		return DayTime{}, DayTime{}, false, nil
	}

	start, err = parseDayTime(startRaw)
	if err != nil {
		return DayTime{}, DayTime{}, false, fmt.Errorf("%s: %w", startVar, err)
	}
	end, err = parseDayTime(endRaw)
	if err != nil {
		return DayTime{}, DayTime{}, false, fmt.Errorf("%s: %w", endVar, err)
	}
	if start == end {
		return DayTime{}, DayTime{}, false, fmt.Errorf("%s and %s must differ", startVar, endVar)
	}

	return start, end, true, nil
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

func parsePositiveFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return def
	}
	return f
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
