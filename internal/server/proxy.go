package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

var allowedMethods = map[string]bool{
	"torrent-get":    true,
	"torrent-add":    true,
	"torrent-start":  true,
	"torrent-stop":   true,
	"torrent-remove": true,
	"session-get":    true,
}

// ProxyHandler proxies JSON-RPC requests to Transmission, enforcing method whitelist.
func ProxyHandler(client *transmission.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"result":"request too large"}`, http.StatusRequestEntityTooLarge)
			return
		}

		var parsed struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			http.Error(w, `{"result":"invalid json"}`, http.StatusBadRequest)
			return
		}

		if !allowedMethods[parsed.Method] {
			slog.Warn("blocked rpc method", "method", parsed.Method, "remote", r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"result":"method not allowed"}`)) //nolint:errcheck
			return
		}

		respBody, err := client.DoRaw(r.Context(), body)
		if err != nil {
			slog.Error("transmission proxy error", "method", parsed.Method, "err", err)
			http.Error(w, `{"result":"upstream error"}`, http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody) //nolint:errcheck
	}
}

// HealthHandler checks Transmission availability via session-get.
func HealthHandler(client *transmission.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := client.SessionGet(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			slog.Warn("health check failed", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"error","message":"transmission unavailable"}`)) //nolint:errcheck
			return
		}
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	}
}
