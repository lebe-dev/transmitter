package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

var allowedMethods = map[string]bool{
	"torrent-get":    true,
	"torrent-add":    true,
	"torrent-start":  true,
	"torrent-stop":   true,
	"torrent-remove": true,
	"torrent-set":    true,
	"session-get":    true,
}

// AutoPriorityConfig holds settings for automatic file priority.
type AutoPriorityConfig struct {
	Enabled   bool
	HighCount int
}

// ProxyHandler proxies JSON-RPC requests to Transmission, enforcing method whitelist.
func ProxyHandler(client *transmission.Client, priorityCfg AutoPriorityConfig) http.HandlerFunc {
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

		if priorityCfg.Enabled && parsed.Method == "torrent-add" {
			go applyAutoPriority(client, respBody, priorityCfg.HighCount)
		}
	}
}

func applyAutoPriority(client *transmission.Client, respBody []byte, highCount int) {
	var rpcResp struct {
		Result    string `json:"result"`
		Arguments struct {
			TorrentAdded *struct {
				ID int64 `json:"id"`
			} `json:"torrent-added"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		slog.Warn("auto-priority: failed to parse response", "err", err)
		return
	}
	if rpcResp.Result != "success" || rpcResp.Arguments.TorrentAdded == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.SetHighPriorityFiles(ctx, rpcResp.Arguments.TorrentAdded.ID, highCount); err != nil {
		slog.Warn("auto-priority: failed to set file priorities", "torrent_id", rpcResp.Arguments.TorrentAdded.ID, "err", err)
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
