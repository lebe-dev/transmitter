package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/lebe-dev/transmitter/internal/shift"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

// noteDeleteTimeout bounds the background cleanup of notes after a torrent removal.
const noteDeleteTimeout = 5 * time.Second

const (
	headerContentType = "Content-Type"
	mimeJSON          = "application/json"
)

// writeJSONError replies with a JSON {"error": ...} body and the given status.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set(headerContentType, mimeJSON)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

var allowedMethods = map[string]bool{
	"torrent-get":    true,
	"torrent-add":    true,
	"torrent-start":  true,
	"torrent-stop":   true,
	"torrent-remove": true,
	"torrent-set":    true,
	"session-get":    true,
	"free-space":     true,
}

// AutoPriorityConfig holds settings for automatic file priority.
type AutoPriorityConfig struct {
	Enabled   bool
	HighCount int
}

// ProxyHandler proxies JSON-RPC requests to Transmission, enforcing method whitelist.
// A non-nil noteStore also has the notes of removed torrents cleaned up.
func ProxyHandler(client *transmission.Client, priorityCfg AutoPriorityConfig, maxBodyBytes int64, noteStore NoteStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"result":"request too large"}`, http.StatusRequestEntityTooLarge)
			return
		}

		var parsed struct {
			Method    string `json:"method"`
			Arguments struct {
				IDs json.RawMessage `json:"ids"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			http.Error(w, `{"result":"invalid json"}`, http.StatusBadRequest)
			return
		}

		if !allowedMethods[parsed.Method] {
			slog.Warn("blocked rpc method", "method", parsed.Method, "remote", r.RemoteAddr)
			w.Header().Set(headerContentType, mimeJSON)
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"result":"method not allowed"}`)) //nolint:errcheck
			return
		}

		// Resolve the hashes before forwarding: once the torrents are gone,
		// Transmission can no longer map the IDs in the request to hashes.
		var removedHashes []string
		if noteStore != nil && parsed.Method == "torrent-remove" {
			removedHashes = resolveHashes(r.Context(), client, parsed.Arguments.IDs)
		}

		respBody, err := client.DoRaw(r.Context(), body)
		if err != nil {
			slog.Error("transmission proxy error", "method", parsed.Method, "err", err)
			http.Error(w, `{"result":"upstream error"}`, http.StatusBadGateway)
			return
		}

		w.Header().Set(headerContentType, mimeJSON)
		w.Write(respBody) //nolint:errcheck

		if priorityCfg.Enabled && parsed.Method == "torrent-add" {
			go applyAutoPriority(client, respBody, priorityCfg.HighCount)
		}
		if len(removedHashes) > 0 && rpcSucceeded(respBody) {
			go deleteNotes(noteStore, removedHashes)
		}
	}
}

// resolveHashes maps a raw Transmission "ids" selector to torrent hashes.
// Failures are logged and yield no hashes: the periodic notes cleanup is the
// backstop, so a hiccup here only delays removal instead of losing data.
func resolveHashes(ctx context.Context, client *transmission.Client, ids json.RawMessage) []string {
	hashes, err := client.GetTorrentHashes(ctx, ids)
	if err != nil {
		slog.Warn("notes: failed to resolve hashes of torrents being removed", "err", err)
		return nil
	}
	return hashes
}

// deleteNotes removes the notes of torrents that were just deleted.
func deleteNotes(store NoteStore, hashes []string) {
	ctx, cancel := context.WithTimeout(context.Background(), noteDeleteTimeout)
	defer cancel()

	if err := store.Delete(ctx, hashes...); err != nil {
		slog.Warn("notes: failed to delete notes of removed torrents", "count", len(hashes), "err", err)
		return
	}
	slog.Debug("notes: deleted notes of removed torrents", "count", len(hashes))
}

// rpcSucceeded reports whether a Transmission response carries result "success".
func rpcSucceeded(respBody []byte) bool {
	var resp struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return false
	}
	return resp.Result == "success"
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

// UISettings holds UI-relevant configuration exposed via /api/settings.
//
// A shift is "configured" when its time window is set via the environment, and
// "enabled" when the user has not switched it off in the UI. The UI shows the
// shift buttons for a configured shift and greys them out while it is disabled.
type UISettings struct {
	Version              string `json:"version"`
	DeleteWithData       bool   `json:"deleteWithData"`
	NightShiftConfigured bool   `json:"nightShiftConfigured"`
	NightShiftEnabled    bool   `json:"nightShiftEnabled"`
	NightShiftStart      string `json:"nightShiftStart,omitempty"`
	NightShiftEnd        string `json:"nightShiftEnd,omitempty"`
	DayShiftConfigured   bool   `json:"dayShiftConfigured"`
	DayShiftEnabled      bool   `json:"dayShiftEnabled"`
	DayShiftStart        string `json:"dayShiftStart,omitempty"`
	DayShiftEnd          string `json:"dayShiftEnd,omitempty"`
	NoteMaxLength        int    `json:"noteMaxLength"`
}

// SettingsHandler returns UI-relevant server configuration as JSON. The shift
// toggles are read from shifts on every request, since they change at runtime;
// a nil store leaves the values of settings untouched.
func SettingsHandler(settings UISettings, shifts ShiftStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := settings
		if shifts != nil {
			resp.NightShiftEnabled = settings.NightShiftConfigured && shiftEnabled(r.Context(), shifts, shift.NightLabel)
			resp.DayShiftEnabled = settings.DayShiftConfigured && shiftEnabled(r.Context(), shifts, shift.DayLabel)
		}

		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

// ServerConfig holds non-sensitive env configuration exposed via /api/config.
type ServerConfig struct {
	TransmissionURL       string   `json:"transmissionUrl"`
	ListenAddr            string   `json:"listenAddr"`
	CORSOrigin            string   `json:"corsOrigin"`
	MaxRequestBodyBytes   int64    `json:"maxRequestBodyBytes"`
	WebUIEnabled          bool     `json:"webUiEnabled"`
	TelegramBotEnabled    bool     `json:"telegramBotEnabled"`
	TelegramUsers         []string `json:"telegramUsers"`
	LogLevel              string   `json:"logLevel"`
	FilePriorityEnabled   bool     `json:"filePriorityEnabled"`
	FilePriorityHighCount int      `json:"filePriorityHighCount"`
	DeleteWithData        bool     `json:"deleteWithData"`
	MonitorInterval       string   `json:"monitorInterval"`
	FileSelectTimeout     string   `json:"fileSelectTimeout"`
	NightShiftConfigured  bool     `json:"nightShiftConfigured"`
	NightShiftStart       string   `json:"nightShiftStart"`
	NightShiftEnd         string   `json:"nightShiftEnd"`
	DayShiftConfigured    bool     `json:"dayShiftConfigured"`
	DayShiftStart         string   `json:"dayShiftStart"`
	DayShiftEnd           string   `json:"dayShiftEnd"`
	DBPath                string   `json:"dbPath"`
	NoteMaxLength         int      `json:"noteMaxLength"`
	NoteCleanupInterval   string   `json:"noteCleanupInterval"`
}

// ConfigHandler returns non-sensitive server configuration as JSON.
func ConfigHandler(cfg ServerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, mimeJSON)
		json.NewEncoder(w).Encode(cfg) //nolint:errcheck
	}
}

// HealthHandler checks Transmission availability via session-get.
func HealthHandler(client *transmission.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := client.SessionGet(r.Context())
		w.Header().Set(headerContentType, mimeJSON)
		if err != nil {
			slog.Warn("health check failed", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"error","message":"transmission unavailable"}`)) //nolint:errcheck
			return
		}
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	}
}
