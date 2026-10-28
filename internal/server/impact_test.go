package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeImpactStore records the auto-stop state it is asked to store.
type fakeImpactStore struct {
	enabled bool
	stored  bool
	err     error
}

func (f *fakeImpactStore) ImpactAutoStopEnabled(context.Context) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.enabled, nil
}

func (f *fakeImpactStore) SetImpactAutoStopEnabled(_ context.Context, enabled bool) error {
	if f.err != nil {
		return f.err
	}
	f.enabled = enabled
	f.stored = true
	return nil
}

func impactRequest(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/impact", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestImpactToggleHandlerStoresState(t *testing.T) {
	store := &fakeImpactStore{}
	h := ImpactToggleHandler(store)

	rec := impactRequest(t, h, `{"enabled":true}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !store.stored || !store.enabled {
		t.Errorf("store = %+v, want the enabled state persisted", store)
	}
}

func TestImpactToggleHandlerRejectsInvalidBody(t *testing.T) {
	for _, body := range []string{`{}`, `not json`, ``} {
		rec := impactRequest(t, ImpactToggleHandler(&fakeImpactStore{}), body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status for %q = %d, want 400", body, rec.Code)
		}
	}
}

func TestImpactToggleHandlerReportsStoreFailure(t *testing.T) {
	h := ImpactToggleHandler(&fakeImpactStore{err: errors.New("db down")})

	rec := impactRequest(t, h, `{"enabled":true}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestSettingsHandlerReportsImpactState(t *testing.T) {
	h := SettingsHandler(UISettings{ImpactTargetRatio: 1.5}, nil, &fakeImpactStore{enabled: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	var got UISettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.ImpactAutoStopEnabled {
		t.Error("impactAutoStopEnabled = false, want the stored true")
	}
	if got.ImpactTargetRatio != 1.5 {
		t.Errorf("impactTargetRatio = %v, want 1.5", got.ImpactTargetRatio)
	}
}

// A store that cannot be read must not promise a stop that never happens.
func TestSettingsHandlerReportsImpactOffOnStoreFailure(t *testing.T) {
	h := SettingsHandler(UISettings{}, nil, &fakeImpactStore{enabled: true, err: errors.New("db down")})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	var got UISettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ImpactAutoStopEnabled {
		t.Error("impactAutoStopEnabled = true, want false when the store fails")
	}
}
