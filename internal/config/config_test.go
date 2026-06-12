package config

import "testing"

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
