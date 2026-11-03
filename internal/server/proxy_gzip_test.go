package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

// gzipBackend is a fake Transmission that compresses its answer whenever the
// request accepts gzip, like the real RPC server does.
func gzipBackend(t *testing.T, payload string) *transmission.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Transmission-Session-Id", "test-session")
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			io.WriteString(w, payload) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		io.WriteString(zw, payload) //nolint:errcheck
		zw.Close()                  //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return transmission.NewClient(srv.URL, "user", "pass")
}

func TestProxyHandlerForwardsGzip(t *testing.T) {
	const payload = `{"result":"success","arguments":{"torrents":[]}}`
	h := ProxyHandler(gzipBackend(t, payload), AutoPriorityConfig{}, 1024, nil, nil)

	t.Run("passes compressed body through to a gzip client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"torrent-get"}`))
		req.Header.Set("Accept-Encoding", "gzip, br")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Errorf("Vary = %q, want Accept-Encoding", got)
		}
		zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		body, _ := io.ReadAll(zr)
		if string(body) != payload {
			t.Errorf("body = %s, want %s", body, payload)
		}
	})

	t.Run("sends plain body to a client without gzip", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"torrent-get"}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want none", got)
		}
		if rec.Body.String() != payload {
			t.Errorf("body = %s, want %s", rec.Body.String(), payload)
		}
	})

	t.Run("decodes the answer when the proxy must read it", func(t *testing.T) {
		added := 0
		h := ProxyHandler(gzipBackend(t, `{"result":"success","arguments":{}}`), AutoPriorityConfig{}, 1024, nil, func() { added++ })
		req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"torrent-add"}`))
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want none", got)
		}
		if added != 1 {
			t.Errorf("onAdded calls = %d, want 1", added)
		}
	})
}
