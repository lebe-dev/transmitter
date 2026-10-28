package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestDayTimeMinutes(t *testing.T) {
	if got := (DayTime{Hour: 1, Minute: 30}).Minutes(); got != 90 {
		t.Errorf("Minutes() = %d, want 90", got)
	}
	if got := (DayTime{Hour: 0, Minute: 0}).Minutes(); got != 0 {
		t.Errorf("Minutes() = %d, want 0", got)
	}
}

func TestDayTimeString(t *testing.T) {
	if got := (DayTime{Hour: 7, Minute: 5}).String(); got != "07:05" {
		t.Errorf("String() = %q, want %q", got, "07:05")
	}
}

func TestParseTelegramUsers(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"@alice", []string{"alice"}},
		{"alice, @bob ,, @carol", []string{"alice", "bob", "carol"}},
	}
	for _, tc := range cases {
		got := parseTelegramUsers(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("parseTelegramUsers(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseTelegramUsers(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("SOME_KEY", "value")
	if got := envOrDefault("SOME_KEY", "def"); got != "value" {
		t.Errorf("envOrDefault set = %q, want %q", got, "value")
	}
	if got := envOrDefault("MISSING_KEY_XYZ", "def"); got != "def" {
		t.Errorf("envOrDefault missing = %q, want %q", got, "def")
	}
}

func TestParseDuration(t *testing.T) {
	def := 30 * time.Second
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", def},
		{"garbage", def},
		{"-5s", def},
		{"0s", def},
		{"2m", 2 * time.Minute},
	}
	for _, tc := range cases {
		if got := parseDuration(tc.in, def); got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParsePositiveInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 3},
		{"abc", 3},
		{"0", 3},
		{"-4", 3},
		{"10", 10},
	}
	for _, tc := range cases {
		if got := parsePositiveInt(tc.in, 3); got != tc.want {
			t.Errorf("parsePositiveInt(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParsePositiveInt64(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 100},
		{"abc", 100},
		{"0", 100},
		{"-4", 100},
		{"4096", 4096},
	}
	for _, tc := range cases {
		if got := parsePositiveInt64(tc.in, 100); got != tc.want {
			t.Errorf("parsePositiveInt64(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseBoolDefault(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"", true, true},
		{"", false, false},
		{"true", false, true},
		{"TRUE", false, true},
		{"1", false, true},
		{"false", true, false},
		{"nonsense", true, false},
	}
	for _, tc := range cases {
		if got := parseBoolDefault(tc.in, tc.def); got != tc.want {
			t.Errorf("parseBoolDefault(%q, %v) = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tc := range cases {
		if got := parseLogLevel(tc.in); got != tc.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
