package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/lebe-dev/transmitter/internal/shift"
)

// shiftBodyMaxBytes caps the tiny {"enabled": bool} body of a toggle request.
const shiftBodyMaxBytes = 256

// ShiftStore persists the on/off state of the shift schedulers.
type ShiftStore interface {
	ShiftEnabled(ctx context.Context, shift string) (bool, error)
	SetShiftEnabled(ctx context.Context, shift string, enabled bool) error
}

// ConfiguredShifts tells which shifts have a time window set in the environment.
// A shift without a window has no scheduler, so there is nothing to toggle.
type ConfiguredShifts struct {
	Night bool
	Day   bool
}

// label maps the shift name used in the API path to the Transmission label the
// scheduler works with, reporting false when that shift is not configured.
func (c ConfiguredShifts) label(name string) (string, bool) {
	switch name {
	case "night":
		return shift.NightLabel, c.Night
	case "day":
		return shift.DayLabel, c.Day
	}
	return "", false
}

// shiftToggleRequest is the body of PUT /api/shifts/{shift}.
type shiftToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

// ShiftToggleHandler switches a shift scheduler on or off. The new state is
// persisted, so it also applies after a restart.
func ShiftToggleHandler(store ShiftStore, configured ConfiguredShifts) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("shift")
		label, ok := configured.label(name)
		switch {
		case label == "":
			writeJSONError(w, http.StatusBadRequest, "unknown shift")
			return
		case !ok:
			writeJSONError(w, http.StatusNotFound, "shift is not configured")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, shiftBodyMaxBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request too large")
			return
		}

		var req shiftToggleRequest
		if err := json.Unmarshal(body, &req); err != nil || req.Enabled == nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}

		if err := store.SetShiftEnabled(r.Context(), label, *req.Enabled); err != nil {
			slog.Error("failed to store shift state", "shift", label, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to store shift state")
			return
		}
		slog.Info("shift switched by user", "shift", label, "enabled", *req.Enabled)

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(map[string]bool{"enabled": *req.Enabled}) //nolint:errcheck
	}
}

// shiftEnabled reads the state of a shift, falling back to enabled when the
// lookup fails — the same choice the scheduler makes, so the UI does not show a
// shift as off while it keeps running.
func shiftEnabled(ctx context.Context, store ShiftStore, label string) bool {
	enabled, err := store.ShiftEnabled(ctx, label)
	if err != nil {
		slog.Warn("failed to read shift state", "shift", label, "err", err)
		return true
	}
	return enabled
}
