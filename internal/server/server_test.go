package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

// backend spins up a fake Transmission RPC server and returns a client pointing at it.
// The handler receives the decoded RPC method and the raw request body.
func backend(t *testing.T, handler func(method string, body []byte) transmission.RPCResponse) *transmission.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Transmission-Session-Id", "test-session")
		body, _ := io.ReadAll(r.Body)
		var req transmission.RPCRequest
		json.Unmarshal(body, &req)                           //nolint:errcheck
		json.NewEncoder(w).Encode(handler(req.Method, body)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return transmission.NewClient(srv.URL, "user", "pass")
}

func TestProxyHandlerAllowedMethod(t *testing.T) {
	client := backend(t, func(method string, _ []byte) transmission.RPCResponse {
		if method != "torrent-get" {
			t.Errorf("unexpected upstream method: %q", method)
		}
		return transmission.RPCResponse{Result: "success"}
	})

	h := ProxyHandler(client, AutoPriorityConfig{}, 1024)
	req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"torrent-get"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != mimeJSON {
		t.Errorf("Content-Type = %q, want %q", ct, mimeJSON)
	}
	if !strings.Contains(rec.Body.String(), "success") {
		t.Errorf("body = %q, want upstream response", rec.Body.String())
	}
}

func TestProxyHandlerBlockedMethod(t *testing.T) {
	client := backend(t, func(string, []byte) transmission.RPCResponse {
		t.Fatal("upstream must not be called for blocked method")
		return transmission.RPCResponse{}
	})

	h := ProxyHandler(client, AutoPriorityConfig{}, 1024)
	req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"session-set"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestProxyHandlerInvalidJSON(t *testing.T) {
	client := backend(t, func(string, []byte) transmission.RPCResponse {
		t.Fatal("upstream must not be called for invalid json")
		return transmission.RPCResponse{}
	})

	h := ProxyHandler(client, AutoPriorityConfig{}, 1024)
	req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{not json`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestProxyHandlerBodyTooLarge(t *testing.T) {
	client := backend(t, func(string, []byte) transmission.RPCResponse {
		return transmission.RPCResponse{Result: "success"}
	})

	h := ProxyHandler(client, AutoPriorityConfig{}, 8)
	req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"torrent-get","extra":"padding"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestApplyAutoPriority(t *testing.T) {
	var setCalled bool
	client := backend(t, func(method string, _ []byte) transmission.RPCResponse {
		switch method {
		case "torrent-get":
			args, _ := json.Marshal(transmission.TorrentWithFilesResult{
				Torrents: []transmission.TorrentWithFiles{{
					ID:    7,
					Files: []transmission.TorrentFile{{Name: "a"}, {Name: "b"}},
				}},
			})
			return transmission.RPCResponse{Result: "success", Arguments: args}
		case "torrent-set":
			setCalled = true
			return transmission.RPCResponse{Result: "success"}
		}
		return transmission.RPCResponse{Result: "success"}
	})

	respBody := []byte(`{"result":"success","arguments":{"torrent-added":{"id":7,"name":"x"}}}`)
	applyAutoPriority(client, respBody, 1)

	if !setCalled {
		t.Error("expected torrent-set to be called for auto-priority")
	}
}

func TestApplyAutoPriorityNoTorrentAdded(t *testing.T) {
	client := backend(t, func(string, []byte) transmission.RPCResponse {
		t.Fatal("upstream must not be called when no torrent was added")
		return transmission.RPCResponse{}
	})

	// Duplicate torrents have no "torrent-added" key, so priority must not run.
	respBody := []byte(`{"result":"success","arguments":{"torrent-duplicate":{"id":7}}}`)
	applyAutoPriority(client, respBody, 1)
}

func TestSettingsHandler(t *testing.T) {
	h := SettingsHandler(UISettings{
		DeleteWithData:    true,
		NightShiftEnabled: true,
		NightShiftStart:   "01:00",
		NightShiftEnd:     "07:00",
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got UISettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.DeleteWithData || got.NightShiftStart != "01:00" {
		t.Errorf("unexpected settings: %+v", got)
	}
}

func TestConfigHandler(t *testing.T) {
	h := ConfigHandler(ServerConfig{ListenAddr: ":8080", LogLevel: "info"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got ServerConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ListenAddr != ":8080" || got.LogLevel != "info" {
		t.Errorf("unexpected config: %+v", got)
	}
}

func TestHealthHandlerOK(t *testing.T) {
	client := backend(t, func(string, []byte) transmission.RPCResponse {
		return transmission.RPCResponse{Result: "success", Arguments: json.RawMessage(`{}`)}
	})

	h := HealthHandler(client)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("body = %q, want ok", rec.Body.String())
	}
}

func TestHealthHandlerUnavailable(t *testing.T) {
	client := backend(t, func(string, []byte) transmission.RPCResponse {
		return transmission.RPCResponse{Result: "failure"}
	})

	h := HealthHandler(client)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestStaticHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/index.html":   {Data: []byte("<html>index</html>")},
		"dist/app/main.css": {Data: []byte("body{}")},
	}
	h, err := StaticHandler(fsys)
	if err != nil {
		t.Fatalf("StaticHandler: %v", err)
	}

	t.Run("serves existing asset", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/main.css", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "body{}") {
			t.Errorf("body = %q, want asset content", rec.Body.String())
		}
	})

	t.Run("falls back to index.html for unknown route", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "index") {
			t.Errorf("body = %q, want index.html fallback", rec.Body.String())
		}
	})
}

func TestCORSMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := CORSMiddleware("http://localhost:4200", next)

	t.Run("sets headers and forwards", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:4200" {
			t.Errorf("origin = %q", origin)
		}
		if rec.Code != http.StatusTeapot {
			t.Errorf("status = %d, want next handler to run (418)", rec.Code)
		}
	})

	t.Run("short-circuits preflight", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/", nil))
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204 for OPTIONS", rec.Code)
		}
	})
}
