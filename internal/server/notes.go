package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/lebe-dev/transmitter/internal/notes"
)

// NoteStore is the subset of the notes store used by the HTTP layer.
type NoteStore interface {
	All(ctx context.Context) (map[string]string, error)
	Set(ctx context.Context, hash, text string) error
	Delete(ctx context.Context, hashes ...string) error
	MaxLength() int
}

// noteBodyOverhead covers the JSON envelope around the note text, on top of the
// worst case of 4 bytes per rune plus escaping.
const noteBodyOverhead = 128

// noteRequest is the body of PUT /api/notes/{hash}.
type noteRequest struct {
	Text string `json:"text"`
}

// NotesHandler returns every stored note keyed by torrent hash.
func NotesHandler(store NoteStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := store.All(r.Context())
		if err != nil {
			slog.Error("failed to list torrent notes", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to load notes")
			return
		}

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(all) //nolint:errcheck
	}
}

// NoteUpdateHandler stores the note of a single torrent. An empty text removes it.
func NoteUpdateHandler(store NoteStore) http.HandlerFunc {
	maxBody := int64(store.MaxLength()*4 + noteBodyOverhead)

	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "note too long")
			return
		}

		var req noteRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}

		err = store.Set(r.Context(), r.PathValue("hash"), req.Text)
		switch {
		case errors.Is(err, notes.ErrInvalidHash):
			writeJSONError(w, http.StatusBadRequest, "invalid torrent hash")
			return
		case errors.Is(err, notes.ErrTooLong):
			writeJSONError(w, http.StatusUnprocessableEntity, "note too long")
			return
		case err != nil:
			slog.Error("failed to store torrent note", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to store note")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// NoteDeleteHandler removes the note of a single torrent.
func NoteDeleteHandler(store NoteStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := store.Delete(r.Context(), r.PathValue("hash"))
		switch {
		case errors.Is(err, notes.ErrInvalidHash):
			writeJSONError(w, http.StatusBadRequest, "invalid torrent hash")
			return
		case err != nil:
			slog.Error("failed to delete torrent note", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to delete note")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
