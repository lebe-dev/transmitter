package config

import (
	"testing"
	"time"
)

func TestParseDayTime(t *testing.T) {
	cases := []struct {
		in      string
		want    DayTime
		wantErr bool
	}{
		{"00:00", DayTime{0, 0}, false},
		{"23:59", DayTime{23, 59}, false},
		{"7:30", DayTime{7, 30}, false},
		{"24:00", DayTime{}, true},
		{"12:60", DayTime{}, true},
		{"abc", DayTime{}, true},
		{"12", DayTime{}, true},
		{"", DayTime{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseDayTime(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseDayTime(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoadNightShift(t *testing.T) {
	t.Setenv("TRANSMISSION_USER", "u")
	t.Setenv("TRANSMISSION_PASS", "p")

	t.Run("disabled when empty", func(t *testing.T) {
		t.Setenv("NIGHT_SHIFT_START", "")
		t.Setenv("NIGHT_SHIFT_END", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.NightShiftEnabled {
			t.Fatal("expected disabled")
		}
	})

	t.Run("enabled when both set", func(t *testing.T) {
		t.Setenv("NIGHT_SHIFT_START", "23:00")
		t.Setenv("NIGHT_SHIFT_END", "07:30")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.NightShiftEnabled {
			t.Fatal("expected enabled")
		}
		if cfg.NightShiftStart != (DayTime{23, 0}) || cfg.NightShiftEnd != (DayTime{7, 30}) {
			t.Errorf("start=%v end=%v", cfg.NightShiftStart, cfg.NightShiftEnd)
		}
	})

	t.Run("error when same start and end", func(t *testing.T) {
		t.Setenv("NIGHT_SHIFT_START", "05:00")
		t.Setenv("NIGHT_SHIFT_END", "05:00")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("error when malformed", func(t *testing.T) {
		t.Setenv("NIGHT_SHIFT_START", "bad")
		t.Setenv("NIGHT_SHIFT_END", "07:00")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLoadDayShift(t *testing.T) {
	t.Setenv("TRANSMISSION_USER", "u")
	t.Setenv("TRANSMISSION_PASS", "p")

	t.Run("disabled when empty", func(t *testing.T) {
		t.Setenv("DAY_SHIFT_START", "")
		t.Setenv("DAY_SHIFT_END", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.DayShiftEnabled {
			t.Fatal("expected disabled")
		}
	})

	t.Run("disabled when only start set", func(t *testing.T) {
		t.Setenv("DAY_SHIFT_START", "08:00")
		t.Setenv("DAY_SHIFT_END", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.DayShiftEnabled {
			t.Fatal("expected disabled")
		}
	})

	t.Run("enabled when both set", func(t *testing.T) {
		t.Setenv("DAY_SHIFT_START", "08:00")
		t.Setenv("DAY_SHIFT_END", "22:15")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.DayShiftEnabled {
			t.Fatal("expected enabled")
		}
		if cfg.DayShiftStart != (DayTime{8, 0}) || cfg.DayShiftEnd != (DayTime{22, 15}) {
			t.Errorf("start=%v end=%v", cfg.DayShiftStart, cfg.DayShiftEnd)
		}
		if cfg.DayShiftInterval != time.Minute {
			t.Errorf("interval=%v, want 1m", cfg.DayShiftInterval)
		}
	})

	t.Run("custom interval", func(t *testing.T) {
		t.Setenv("DAY_SHIFT_START", "08:00")
		t.Setenv("DAY_SHIFT_END", "22:00")
		t.Setenv("DAY_SHIFT_INTERVAL", "30s")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.DayShiftInterval != 30*time.Second {
			t.Errorf("interval=%v, want 30s", cfg.DayShiftInterval)
		}
	})

	t.Run("error when same start and end", func(t *testing.T) {
		t.Setenv("DAY_SHIFT_START", "09:00")
		t.Setenv("DAY_SHIFT_END", "09:00")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("error when malformed", func(t *testing.T) {
		t.Setenv("DAY_SHIFT_START", "08:00")
		t.Setenv("DAY_SHIFT_END", "nope")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("independent from night shift", func(t *testing.T) {
		t.Setenv("NIGHT_SHIFT_START", "23:00")
		t.Setenv("NIGHT_SHIFT_END", "07:00")
		t.Setenv("DAY_SHIFT_START", "08:00")
		t.Setenv("DAY_SHIFT_END", "22:00")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.NightShiftEnabled || !cfg.DayShiftEnabled {
			t.Fatalf("night=%v day=%v, want both enabled", cfg.NightShiftEnabled, cfg.DayShiftEnabled)
		}
		if cfg.NightShiftStart != (DayTime{23, 0}) || cfg.DayShiftStart != (DayTime{8, 0}) {
			t.Errorf("windows crossed: night=%v day=%v", cfg.NightShiftStart, cfg.DayShiftStart)
		}
	})
}

func TestLoadSentry(t *testing.T) {
	t.Setenv("TRANSMISSION_USER", "u")
	t.Setenv("TRANSMISSION_PASS", "p")

	t.Run("disabled when DSN empty", func(t *testing.T) {
		t.Setenv("SENTRY_DSN", "")
		t.Setenv("SENTRY_ENVIRONMENT", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SentryDSN != "" {
			t.Errorf("expected empty DSN, got %q", cfg.SentryDSN)
		}
	})

	t.Run("enabled when DSN and environment set", func(t *testing.T) {
		t.Setenv("SENTRY_DSN", "https://key@example.com/1")
		t.Setenv("SENTRY_ENVIRONMENT", "production")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SentryDSN != "https://key@example.com/1" {
			t.Errorf("DSN = %q", cfg.SentryDSN)
		}
		if cfg.SentryEnvironment != "production" {
			t.Errorf("environment = %q", cfg.SentryEnvironment)
		}
	})

	t.Run("error when DSN set without environment", func(t *testing.T) {
		t.Setenv("SENTRY_DSN", "https://key@example.com/1")
		t.Setenv("SENTRY_ENVIRONMENT", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLoadNoteSettings(t *testing.T) {
	t.Setenv("TRANSMISSION_USER", "u")
	t.Setenv("TRANSMISSION_PASS", "p")

	t.Run("defaults", func(t *testing.T) {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.DBPath != "data/transmitter.db" {
			t.Errorf("DBPath = %q, want %q", cfg.DBPath, "data/transmitter.db")
		}
		if cfg.NoteMaxLength != defaultNoteMaxLength {
			t.Errorf("NoteMaxLength = %d, want %d", cfg.NoteMaxLength, defaultNoteMaxLength)
		}
		if cfg.NoteCleanupInterval != defaultNoteCleanupInterval {
			t.Errorf("NoteCleanupInterval = %v, want %v", cfg.NoteCleanupInterval, defaultNoteCleanupInterval)
		}
	})

	t.Run("overrides", func(t *testing.T) {
		t.Setenv("DB_PATH", "/var/lib/transmitter/app.db")
		t.Setenv("TORRENT_NOTE_MAX_LENGTH", "50")
		t.Setenv("TORRENT_NOTE_CLEANUP_INTERVAL", "15m")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.DBPath != "/var/lib/transmitter/app.db" {
			t.Errorf("DBPath = %q", cfg.DBPath)
		}
		if cfg.NoteMaxLength != 50 {
			t.Errorf("NoteMaxLength = %d, want 50", cfg.NoteMaxLength)
		}
		if cfg.NoteCleanupInterval != 15*time.Minute {
			t.Errorf("NoteCleanupInterval = %v, want 15m", cfg.NoteCleanupInterval)
		}
	})

	t.Run("invalid values fall back to defaults", func(t *testing.T) {
		t.Setenv("TORRENT_NOTE_MAX_LENGTH", "-5")
		t.Setenv("TORRENT_NOTE_CLEANUP_INTERVAL", "soon")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.NoteMaxLength != defaultNoteMaxLength {
			t.Errorf("NoteMaxLength = %d, want default", cfg.NoteMaxLength)
		}
		if cfg.NoteCleanupInterval != defaultNoteCleanupInterval {
			t.Errorf("NoteCleanupInterval = %v, want default", cfg.NoteCleanupInterval)
		}
	})
}
