package bot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/shift"
)

func testShiftOptions() []shift.Options {
	return []shift.Options{
		{
			Name:     shift.NightLabel,
			Label:    shift.NightLabel,
			Start:    config.DayTime{Hour: 23, Minute: 0},
			End:      config.DayTime{Hour: 7, Minute: 0},
			Interval: time.Minute,
		},
		{
			Name:     shift.DayLabel,
			Label:    shift.DayLabel,
			Start:    config.DayTime{Hour: 8, Minute: 0},
			End:      config.DayTime{Hour: 22, Minute: 0},
			Interval: 5 * time.Minute,
		},
	}
}

func at(t *testing.T, hhmm string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("15:04", hhmm, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", hhmm, err)
	}
	return parsed
}

func TestShiftViews(t *testing.T) {
	opts := testShiftOptions()
	enabled := map[string]bool{shift.NightLabel: true, shift.DayLabel: false}

	views := shiftViews(opts, at(t, "23:30"), enabled)

	if len(views) != 2 {
		t.Fatalf("got %d views, want 2", len(views))
	}

	night := views[0]
	if night.Label != shift.NightLabel {
		t.Errorf("first view label = %q, want %q", night.Label, shift.NightLabel)
	}
	if !night.InWindow {
		t.Error("night shift should be in window at 23:30")
	}
	if !night.Enabled {
		t.Error("night shift should be enabled")
	}
	if night.Window != "23:00–07:00" {
		t.Errorf("night window = %q", night.Window)
	}

	day := views[1]
	if day.InWindow {
		t.Error("day shift should be out of window at 23:30")
	}
	if day.Enabled {
		t.Error("day shift should be disabled")
	}
}

func TestShiftViewsMissingToggleDefaultsToEnabled(t *testing.T) {
	views := shiftViews(testShiftOptions(), at(t, "12:00"), nil)

	for _, v := range views {
		if !v.Enabled {
			t.Errorf("%s: want enabled by default", v.Label)
		}
	}
}

func TestFormatShifts(t *testing.T) {
	views := shiftViews(testShiftOptions(), at(t, "09:00"), map[string]bool{
		shift.NightLabel: false,
		shift.DayLabel:   true,
	})

	text := formatShifts(views)

	for _, want := range []string{"Night", "Day", "23:00–07:00", "08:00–22:00", "every 1m", "every 5m"} {
		if !strings.Contains(text, want) {
			t.Errorf("formatShifts output missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "active now") != 1 {
		t.Errorf("expected exactly one shift in window:\n%s", text)
	}
}

func TestFormatShiftsWithoutConfiguredShifts(t *testing.T) {
	text := formatShifts(nil)
	if !strings.Contains(text, "No shifts configured") {
		t.Errorf("unexpected text: %s", text)
	}
}

func TestShiftsKeyboard(t *testing.T) {
	views := shiftViews(testShiftOptions(), at(t, "09:00"), map[string]bool{
		shift.NightLabel: true,
		shift.DayLabel:   false,
	})

	kb := shiftsKeyboard(views)
	if countButtons(kb.InlineKeyboard) != 3 {
		t.Fatalf("got %d buttons, want 3 (two toggles + refresh)", countButtons(kb.InlineKeyboard))
	}

	night := kb.InlineKeyboard[0][0]
	if night.Data != "sh:night-shift:0" {
		t.Errorf("night toggle data = %q, want sh:night-shift:0 (switch an enabled shift off)", night.Data)
	}
	day := kb.InlineKeyboard[1][0]
	if day.Data != "sh:day-shift:1" {
		t.Errorf("day toggle data = %q, want sh:day-shift:1 (switch a disabled shift on)", day.Data)
	}
	if kb.InlineKeyboard[2][0].Data != shiftsRefreshData {
		t.Errorf("last button data = %q, want %q", kb.InlineKeyboard[2][0].Data, shiftsRefreshData)
	}
}

func TestShiftsKeyboardWithoutConfiguredShifts(t *testing.T) {
	if kb := shiftsKeyboard(nil); kb != nil {
		t.Errorf("expected no keyboard, got %+v", kb)
	}
}

func TestParseShiftToggle(t *testing.T) {
	opts := testShiftOptions()

	cases := []struct {
		data        string
		wantLabel   string
		wantEnabled bool
		wantErr     bool
	}{
		{data: "sh:night-shift:1", wantLabel: shift.NightLabel, wantEnabled: true},
		{data: "sh:day-shift:0", wantLabel: shift.DayLabel, wantEnabled: false},
		{data: "sh:evening-shift:1", wantErr: true},
		{data: "sh:night-shift:yes", wantErr: true},
		{data: "sh:night-shift", wantErr: true},
		{data: "sh:", wantErr: true},
	}

	for _, tc := range cases {
		label, enabled, err := parseShiftToggle(tc.data, opts)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseShiftToggle(%q) expected error", tc.data)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseShiftToggle(%q): %v", tc.data, err)
			continue
		}
		if label != tc.wantLabel || enabled != tc.wantEnabled {
			t.Errorf("parseShiftToggle(%q) = %q,%v want %q,%v", tc.data, label, enabled, tc.wantLabel, tc.wantEnabled)
		}
	}
}

// stubShiftStore records writes and can fail reads on demand.
type stubShiftStore struct {
	enabled map[string]bool
	readErr error
}

func (s *stubShiftStore) ShiftEnabled(_ context.Context, label string) (bool, error) {
	if s.readErr != nil {
		return false, s.readErr
	}
	return s.enabled[label], nil
}

func (s *stubShiftStore) SetShiftEnabled(_ context.Context, label string, enabled bool) error {
	s.enabled[label] = enabled
	return nil
}

func TestBotShiftCard(t *testing.T) {
	store := &stubShiftStore{enabled: map[string]bool{shift.NightLabel: false, shift.DayLabel: true}}
	b := &Bot{
		logger: nopLogger(),
		shifts: Shifts{Configured: testShiftOptions(), Store: store},
	}

	if !b.shiftsConfigured() {
		t.Fatal("shiftsConfigured() = false with two configured shifts")
	}

	text, kb := b.shiftCard(context.Background())
	if !strings.Contains(text, "⛔ off") || !strings.Contains(text, "✅ on") {
		t.Errorf("card does not reflect the stored toggles:\n%s", text)
	}
	if !hasButtonUnique(kb.InlineKeyboard, "sh:night-shift:1") {
		t.Error("expected a button switching the disabled night shift on")
	}
}

func TestBotShiftCardTreatsFailedLookupAsEnabled(t *testing.T) {
	store := &stubShiftStore{enabled: map[string]bool{}, readErr: errors.New("db is gone")}
	b := &Bot{
		logger: nopLogger(),
		shifts: Shifts{Configured: testShiftOptions(), Store: store},
	}

	text, _ := b.shiftCard(context.Background())
	if strings.Contains(text, "⛔ off") {
		t.Errorf("a failed lookup must not show a running shift as off:\n%s", text)
	}
}

func TestBotWithoutConfiguredShifts(t *testing.T) {
	b := &Bot{logger: nopLogger()}

	if b.shiftsConfigured() {
		t.Error("shiftsConfigured() = true without any shift")
	}
	text, kb := b.shiftCard(context.Background())
	if kb != nil {
		t.Error("expected no keyboard")
	}
	if !strings.Contains(text, "No shifts configured") {
		t.Errorf("unexpected text: %s", text)
	}
}

func TestFormatInterval(t *testing.T) {
	cases := map[time.Duration]string{
		time.Minute:      "1m",
		5 * time.Minute:  "5m",
		time.Hour:        "1h",
		90 * time.Second: "1m30s",
		30 * time.Second: "30s",
	}
	for d, want := range cases {
		if got := formatInterval(d); got != want {
			t.Errorf("formatInterval(%v) = %q, want %q", d, got, want)
		}
	}
}
