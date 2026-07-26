package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/notes"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

const (
	testHash      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testOtherHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func noteStore(t *testing.T, maxLen int) *notes.Store {
	t.Helper()
	store, err := notes.Open(filepath.Join(t.TempDir(), "notes.db"), maxLen)
	if err != nil {
		t.Fatalf("notes.Open() error = %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	return store
}

// notifyingStore reports deletions on a channel so tests can await the
// background cleanup triggered by torrent-remove.
type notifyingStore struct {
	*notes.Store
	deleted chan []string
}

func (s *notifyingStore) Delete(ctx context.Context, hashes ...string) error {
	err := s.Store.Delete(ctx, hashes...)
	s.deleted <- hashes
	return err
}

func putNote(t *testing.T, h http.HandlerFunc, hash, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/notes/"+hash, strings.NewReader(body))
	req.SetPathValue("hash", hash)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNotesHandlerReturnsAllNotes(t *testing.T) {
	store := noteStore(t, 200)
	store.Set(context.Background(), testHash, "watch later") //nolint:errcheck

	rec := httptest.NewRecorder()
	NotesHandler(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/notes", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got[testHash] != "watch later" {
		t.Errorf("notes = %v, want the stored note", got)
	}
}

func TestNoteUpdateHandlerStoresNote(t *testing.T) {
	store := noteStore(t, 200)

	rec := putNote(t, NoteUpdateHandler(store), testHash, `{"text":"смотреть с семьёй"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
	}
	got, _ := store.Get(context.Background(), testHash)
	if got != "смотреть с семьёй" {
		t.Errorf("stored note = %q, want %q", got, "смотреть с семьёй")
	}
}

func TestNoteUpdateHandlerEmptyTextRemovesNote(t *testing.T) {
	store := noteStore(t, 200)
	store.Set(context.Background(), testHash, "old") //nolint:errcheck

	rec := putNote(t, NoteUpdateHandler(store), testHash, `{"text":""}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	all, _ := store.All(context.Background())
	if len(all) != 0 {
		t.Errorf("notes = %v, want empty", all)
	}
}

func TestNoteUpdateHandlerRejectsInvalidHash(t *testing.T) {
	rec := putNote(t, NoteUpdateHandler(noteStore(t, 200)), "not-a-hash", `{"text":"x"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNoteUpdateHandlerRejectsInvalidJSON(t *testing.T) {
	rec := putNote(t, NoteUpdateHandler(noteStore(t, 200)), testHash, `{not json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNoteUpdateHandlerRejectsTooLongNote(t *testing.T) {
	store := noteStore(t, 10)

	body, _ := json.Marshal(noteRequest{Text: strings.Repeat("a", 11)})
	rec := putNote(t, NoteUpdateHandler(store), testHash, string(body))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "note too long") {
		t.Errorf("body = %q, want an explanatory error", rec.Body.String())
	}
}

func TestNoteUpdateHandlerRejectsOversizedBody(t *testing.T) {
	store := noteStore(t, 10)

	body, _ := json.Marshal(noteRequest{Text: strings.Repeat("a", 4096)})
	rec := putNote(t, NoteUpdateHandler(store), testHash, string(body))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestNoteDeleteHandlerRemovesNote(t *testing.T) {
	store := noteStore(t, 200)
	store.Set(context.Background(), testHash, "bye") //nolint:errcheck

	req := httptest.NewRequest(http.MethodDelete, "/api/notes/"+testHash, nil)
	req.SetPathValue("hash", testHash)
	rec := httptest.NewRecorder()
	NoteDeleteHandler(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	all, _ := store.All(context.Background())
	if len(all) != 0 {
		t.Errorf("notes = %v, want empty", all)
	}
}

func TestNoteDeleteHandlerRejectsInvalidHash(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/notes/zzz", nil)
	req.SetPathValue("hash", "zzz")
	rec := httptest.NewRecorder()
	NoteDeleteHandler(noteStore(t, 200)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// removeBackend answers torrent-get with the given hashes and torrent-remove
// with the given result.
func removeBackend(t *testing.T, hashes []string, removeResult string) *transmission.Client {
	t.Helper()
	return backend(t, func(method string, _ []byte) transmission.RPCResponse {
		if method == "torrent-get" {
			torrents := make([]transmission.Torrent, 0, len(hashes))
			for _, h := range hashes {
				torrents = append(torrents, transmission.Torrent{HashString: h})
			}
			args, _ := json.Marshal(transmission.TorrentGetResult{Torrents: torrents})
			return transmission.RPCResponse{Result: "success", Arguments: args}
		}
		return transmission.RPCResponse{Result: removeResult}
	})
}

func awaitDeletion(t *testing.T, deleted chan []string) []string {
	t.Helper()
	select {
	case hashes := <-deleted:
		return hashes
	case <-time.After(2 * time.Second):
		t.Fatal("notes of the removed torrent were not deleted within 2s")
		return nil
	}
}

func TestProxyHandlerDeletesNotesOfRemovedTorrents(t *testing.T) {
	ctx := context.Background()
	store := &notifyingStore{Store: noteStore(t, 200), deleted: make(chan []string, 1)}
	store.Set(ctx, testHash, "gone soon") //nolint:errcheck
	store.Set(ctx, testOtherHash, "keep") //nolint:errcheck

	client := removeBackend(t, []string{testHash}, "success")
	h := ProxyHandler(client, AutoPriorityConfig{}, 1024, store)

	req := httptest.NewRequest(http.MethodPost, "/api/rpc",
		strings.NewReader(`{"method":"torrent-remove","arguments":{"ids":[7]}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	awaitDeletion(t, store.deleted)

	all, _ := store.All(ctx)
	if _, ok := all[testHash]; ok {
		t.Error("note of the removed torrent survived")
	}
	if all[testOtherHash] != "keep" {
		t.Error("note of an unrelated torrent was deleted")
	}
}

func TestProxyHandlerKeepsNotesWhenRemoveFails(t *testing.T) {
	ctx := context.Background()
	store := &notifyingStore{Store: noteStore(t, 200), deleted: make(chan []string, 1)}
	store.Set(ctx, testHash, "still here") //nolint:errcheck

	client := removeBackend(t, []string{testHash}, "failure")
	h := ProxyHandler(client, AutoPriorityConfig{}, 1024, store)

	req := httptest.NewRequest(http.MethodPost, "/api/rpc",
		strings.NewReader(`{"method":"torrent-remove","arguments":{"ids":[7]}}`))
	h.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case hashes := <-store.deleted:
		t.Fatalf("notes deleted after a failed removal: %v", hashes)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestProxyHandlerSkipsHashLookupWithoutNoteStore(t *testing.T) {
	client := backend(t, func(method string, _ []byte) transmission.RPCResponse {
		if method == "torrent-get" {
			t.Error("hashes must not be resolved when notes are disabled")
		}
		return transmission.RPCResponse{Result: "success"}
	})

	h := ProxyHandler(client, AutoPriorityConfig{}, 1024, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/rpc",
		strings.NewReader(`{"method":"torrent-remove","arguments":{"ids":[7]}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
