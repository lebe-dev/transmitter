package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// pauseBodyMaxBytes caps the tiny {"paused": bool} body of a toggle request.
const pauseBodyMaxBytes = 256

// Pauser switches the global pause on and off (see internal/pause).
type Pauser interface {
	Paused(ctx context.Context) (bool, error)
	Pause(ctx context.Context) (int, error)
	Resume(ctx context.Context) (int, error)
}

// pauseRequest is the body of PUT /api/pause.
type pauseRequest struct {
	Paused *bool `json:"paused"`
}

// pauseResponse reports the pause state and, after a toggle, how many torrents
// were stopped or started.
type pauseResponse struct {
	Paused bool `json:"paused"`
	Count  int  `json:"count"`
}

// PauseStateHandler reports whether the global pause is on.
func PauseStateHandler(p Pauser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paused, err := p.Paused(r.Context())
		if err != nil {
			slog.Error("failed to read global pause state", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to read pause state")
			return
		}

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(pauseResponse{Paused: paused}) //nolint:errcheck
	}
}

// PauseToggleHandler switches the global pause on or off.
func PauseToggleHandler(p Pauser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, pauseBodyMaxBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request too large")
			return
		}

		var req pauseRequest
		if err := json.Unmarshal(body, &req); err != nil || req.Paused == nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}

		var count int
		if *req.Paused {
			count, err = p.Pause(r.Context())
		} else {
			count, err = p.Resume(r.Context())
		}
		if err != nil {
			slog.Error("failed to switch global pause", "paused", *req.Paused, "err", err)
			writeJSONError(w, http.StatusBadGateway, "failed to switch pause")
			return
		}

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(pauseResponse{Paused: *req.Paused, Count: count}) //nolint:errcheck
	}
}
