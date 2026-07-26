package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lebe-dev/transmitter/internal/shift"
)

// fakeShiftStore records the toggles it is asked to store.
type fakeShiftStore struct {
	state map[string]bool
	err   error
}

func newFakeShiftStore() *fakeShiftStore {
	return &fakeShiftStore{state: map[string]bool{}}
}

func (f *fakeShiftStore) ShiftEnabled(_ context.Context, name string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	enabled, ok := f.state[name]
	if !ok {
		return true, nil
	}
	return enabled, nil
}

func (f *fakeShiftStore) SetShiftEnabled(_ context.Context, name string, enabled bool) error {
	if f.err != nil {
		return f.err
	}
	f.state[name] = enabled
	return nil
}

func toggleRequest(t *testing.T, h http.HandlerFunc, name, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/shifts/"+name, strings.NewReader(body))
	req.SetPathValue("shift", name)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestShiftToggleHandlerDisablesShift(t *testing.T) {
	store := newFakeShiftStore()
	h := ShiftToggleHandler(store, ConfiguredShifts{Night: true, Day: true})

	rec := toggleRequest(t, h, "night", `{"enabled":false}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["enabled"] {
		t.Errorf("response enabled = true, want false")
	}
	if store.state[shift.NightLabel] {
		t.Errorf("store state = %v, want night shift disabled", store.state)
	}
	if _, ok := store.state[shift.DayLabel]; ok {
		t.Errorf("day shift must be untouched, got %v", store.state)
	}
}

func TestShiftToggleHandlerEnablesShift(t *testing.T) {
	store := newFakeShiftStore()
	store.state[shift.DayLabel] = false
	h := ShiftToggleHandler(store, ConfiguredShifts{Day: true})

	rec := toggleRequest(t, h, "day", `{"enabled":true}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !store.state[shift.DayLabel] {
		t.Errorf("store state = %v, want day shift enabled", store.state)
	}
}

func TestShiftToggleHandlerUnknownShift(t *testing.T) {
	h := ShiftToggleHandler(newFakeShiftStore(), ConfiguredShifts{Night: true, Day: true})

	rec := toggleRequest(t, h, "evening", `{"enabled":true}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Without a time window there is no scheduler to switch on.
func TestShiftToggleHandlerNotConfigured(t *testing.T) {
	store := newFakeShiftStore()
	h := ShiftToggleHandler(store, ConfiguredShifts{Night: true})

	rec := toggleRequest(t, h, "day", `{"enabled":true}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if len(store.state) != 0 {
		t.Errorf("store must stay untouched, got %v", store.state)
	}
}

func TestShiftToggleHandlerInvalidBody(t *testing.T) {
	h := ShiftToggleHandler(newFakeShiftStore(), ConfiguredShifts{Night: true})

	for _, body := range []string{`{not json`, `{}`, `{"enabled":"yes"}`} {
		rec := toggleRequest(t, h, "night", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestShiftToggleHandlerStoreError(t *testing.T) {
	store := newFakeShiftStore()
	store.err = errors.New("db down")
	h := ShiftToggleHandler(store, ConfiguredShifts{Night: true})

	rec := toggleRequest(t, h, "night", `{"enabled":false}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestSettingsHandlerReportsStoredShiftState(t *testing.T) {
	store := newFakeShiftStore()
	store.state[shift.NightLabel] = false

	h := SettingsHandler(UISettings{
		NightShiftConfigured: true,
		NightShiftStart:      "23:00",
		NightShiftEnd:        "08:00",
		DayShiftConfigured:   true,
		DayShiftStart:        "08:00",
		DayShiftEnd:          "23:00",
	}, store)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	var got UISettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.NightShiftConfigured || got.NightShiftEnabled {
		t.Errorf("night shift = configured %v / enabled %v, want configured and disabled", got.NightShiftConfigured, got.NightShiftEnabled)
	}
	// No stored preference means the shift is on.
	if !got.DayShiftEnabled {
		t.Errorf("day shift enabled = false, want true by default")
	}
}

// A shift without a configured window is never reported as enabled, whatever
// the store holds.
func TestSettingsHandlerIgnoresStateOfUnconfiguredShift(t *testing.T) {
	store := newFakeShiftStore()
	store.state[shift.DayLabel] = true

	h := SettingsHandler(UISettings{}, store)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	var got UISettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.DayShiftEnabled || got.DayShiftConfigured {
		t.Errorf("unconfigured day shift reported as %+v", got)
	}
}
