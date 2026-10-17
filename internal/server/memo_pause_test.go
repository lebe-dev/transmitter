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

type fakeMemoStore struct{ text string }

func (f *fakeMemoStore) Memo(context.Context) (string, error) { return f.text, nil }

func (f *fakeMemoStore) SetMemo(_ context.Context, text string) error {
	f.text = text
	return nil
}

func TestMemoRoundTrip(t *testing.T) {
	store := &fakeMemoStore{}

	rec := httptest.NewRecorder()
	MemoUpdateHandler(store).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPut, "/api/memo", strings.NewReader(`{"text":"a\r\nb\n"}`)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", rec.Code)
	}
	if store.text != "a\nb\n" {
		t.Errorf("stored %q, want line breaks kept and normalised", store.text)
	}

	rec = httptest.NewRecorder()
	MemoHandler(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memo", nil))
	var got memoBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Text != "a\nb\n" {
		t.Errorf("GET = %q, %v", got.Text, err)
	}
}

func TestMemoUpdateRejectsTooLong(t *testing.T) {
	body, _ := json.Marshal(memoBody{Text: strings.Repeat("я", MemoMaxLength+1)})
	rec := httptest.NewRecorder()
	MemoUpdateHandler(&fakeMemoStore{}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPut, "/api/memo", strings.NewReader(string(body))))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestMemoUpdateClearsWhitespace(t *testing.T) {
	store := &fakeMemoStore{text: "old"}
	rec := httptest.NewRecorder()
	MemoUpdateHandler(store).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPut, "/api/memo", strings.NewReader(`{"text":"  \n "}`)))
	if rec.Code != http.StatusNoContent || store.text != "" {
		t.Errorf("status = %d, text = %q; want cleared", rec.Code, store.text)
	}
}

type fakePauser struct {
	paused bool
	err    error
}

func (f *fakePauser) Paused(context.Context) (bool, error) { return f.paused, nil }

func (f *fakePauser) Pause(context.Context) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.paused = true
	return 3, nil
}

func (f *fakePauser) Resume(context.Context) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.paused = false
	return 2, nil
}

func pauseRequestTo(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/pause", strings.NewReader(body)))
	return rec
}

func TestPauseToggleHandler(t *testing.T) {
	p := &fakePauser{}

	rec := pauseRequestTo(PauseToggleHandler(p), `{"paused":true}`)
	var got pauseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Paused || got.Count != 3 || !p.paused {
		t.Fatalf("pause: status %d, body %+v, err %v", rec.Code, got, err)
	}

	rec = httptest.NewRecorder()
	PauseStateHandler(p).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pause", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Paused {
		t.Fatalf("state: %+v, %v", got, err)
	}

	rec = pauseRequestTo(PauseToggleHandler(p), `{"paused":false}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Paused || got.Count != 2 || p.paused {
		t.Fatalf("resume: status %d, body %+v, err %v", rec.Code, got, err)
	}
}

func TestPauseToggleHandlerErrors(t *testing.T) {
	for _, body := range []string{`{}`, `nope`, ``} {
		if rec := pauseRequestTo(PauseToggleHandler(&fakePauser{}), body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
	if rec := pauseRequestTo(PauseToggleHandler(&fakePauser{err: errors.New("rpc")}), `{"paused":true}`); rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}
