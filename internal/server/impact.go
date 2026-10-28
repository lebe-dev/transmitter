package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// impactBodyMaxBytes caps the tiny {"enabled": bool} body of a toggle request.
const impactBodyMaxBytes = 256

// ImpactStore persists whether torrents that reached the target upload ratio are
// stopped automatically.
type ImpactStore interface {
	ImpactAutoStopEnabled(ctx context.Context) (bool, error)
	SetImpactAutoStopEnabled(ctx context.Context, enabled bool) error
}

// impactToggleRequest is the body of PUT /api/impact.
type impactToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

// ImpactToggleHandler switches automatic stopping of fully repaid torrents on or
// off. The new state is persisted, so it also applies after a restart.
func ImpactToggleHandler(store ImpactStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, impactBodyMaxBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request too large")
			return
		}

		var req impactToggleRequest
		if err := json.Unmarshal(body, &req); err != nil || req.Enabled == nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}

		if err := store.SetImpactAutoStopEnabled(r.Context(), *req.Enabled); err != nil {
			slog.Error("failed to store impact auto-stop state", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to store impact state")
			return
		}
		slog.Info("impact auto-stop switched by user", "enabled", *req.Enabled)

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(map[string]bool{"enabled": *req.Enabled}) //nolint:errcheck
	}
}

// impactAutoStopEnabled reads the state of the rule, falling back to off when the
// lookup fails — the same choice the worker makes, so the UI does not promise a
// stop that never happens.
func impactAutoStopEnabled(ctx context.Context, store ImpactStore) bool {
	enabled, err := store.ImpactAutoStopEnabled(ctx)
	if err != nil {
		slog.Warn("failed to read impact auto-stop state", "err", err)
		return false
	}
	return enabled
}
