package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"
)

// MemoMaxLength caps the global note in runes.
const MemoMaxLength = 20_000

// MemoStore persists the free-form note shown at the top of the torrent list.
type MemoStore interface {
	Memo(ctx context.Context) (string, error)
	SetMemo(ctx context.Context, text string) error
}

// memoBody is the body of GET and PUT /api/memo.
type memoBody struct {
	Text string `json:"text"`
}

// MemoHandler returns the global note.
func MemoHandler(store MemoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		text, err := store.Memo(r.Context())
		if err != nil {
			slog.Error("failed to load memo", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to load memo")
			return
		}

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(memoBody{Text: text}) //nolint:errcheck
	}
}

// MemoUpdateHandler stores the global note. The text is kept as typed (line
// breaks included) since the UI saves it while the user is still writing; a
// whitespace-only text removes it.
func MemoUpdateHandler(store MemoStore) http.HandlerFunc {
	maxBody := int64(MemoMaxLength*6 + noteBodyOverhead)

	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "memo too long")
			return
		}

		var req memoBody
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}

		text := strings.ReplaceAll(req.Text, "\r\n", "\n")
		if strings.TrimSpace(text) == "" {
			text = ""
		}
		if utf8.RuneCountInString(text) > MemoMaxLength {
			writeJSONError(w, http.StatusUnprocessableEntity, "memo too long")
			return
		}

		if err := store.SetMemo(r.Context(), text); err != nil {
			slog.Error("failed to store memo", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to store memo")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
