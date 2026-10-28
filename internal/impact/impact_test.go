package impact

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

func TestRatio(t *testing.T) {
	cases := []struct {
		name string
		in   transmission.Torrent
		want float64
	}{
		{"reported ratio wins", transmission.Torrent{UploadRatio: 1.5, UploadedEver: 1, DownloadedEver: 1}, 1.5},
		{"unknown ratio falls back to counters", transmission.Torrent{UploadRatio: -1, UploadedEver: 300, DownloadedEver: 200}, 1.5},
		{"unknown ratio without downloads", transmission.Torrent{UploadRatio: -1, UploadedEver: 100}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Ratio(tc.in); got != tc.want {
				t.Errorf("Ratio() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReached(t *testing.T) {
	cases := []struct {
		name   string
		in     transmission.Torrent
		target float64
		want   bool
	}{
		{"below target", transmission.Torrent{UploadRatio: 0.9, DownloadedEver: 100}, 1.0, false},
		{"exactly at target", transmission.Torrent{UploadRatio: 1.0, DownloadedEver: 100}, 1.0, true},
		{"above target", transmission.Torrent{UploadRatio: 2.0, DownloadedEver: 100}, 1.0, true},
		{"custom target not reached", transmission.Torrent{UploadRatio: 1.4, DownloadedEver: 100}, 1.5, false},
		{"nothing downloaded", transmission.Torrent{UploadRatio: -1, UploadedEver: 500}, 1.0, false},
		{"target off", transmission.Torrent{UploadRatio: 5, DownloadedEver: 100}, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Reached(tc.in, tc.target); got != tc.want {
				t.Errorf("Reached(%v) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestReachedIDsSkipsStoppedTorrents(t *testing.T) {
	torrents := []transmission.Torrent{
		{ID: 1, Status: 6, UploadRatio: 1.2, DownloadedEver: 100}, // seeding, done paying
		{ID: 2, Status: 0, UploadRatio: 3.0, DownloadedEver: 100}, // already stopped
		{ID: 3, Status: 6, UploadRatio: 0.4, DownloadedEver: 100}, // still owes
		{ID: 4, Status: 4, UploadRatio: 1.0, DownloadedEver: 100}, // downloading, but paid
	}

	got := ReachedIDs(torrents, 1.0)
	if !slices.Equal(got, []int64{1, 4}) {
		t.Errorf("ReachedIDs() = %v, want [1 4]", got)
	}
}

// recordingServer serves torrent-get and records torrent-stop calls.
type recordingServer struct {
	mu       sync.Mutex
	torrents []transmission.Torrent
	stops    [][]int64
}

func (rs *recordingServer) handle(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Transmission-Session-Id", "test")

		var req transmission.RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}

		rs.mu.Lock()
		defer rs.mu.Unlock()

		switch req.Method {
		case "torrent-get":
			args, _ := json.Marshal(transmission.TorrentGetResult{Torrents: rs.torrents})
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success", Arguments: args}) //nolint:errcheck
		case "torrent-stop":
			var a transmission.TorrentActionArgs
			_ = json.Unmarshal(req.Arguments, &a)
			ids := append([]int64(nil), a.IDs...)
			slices.Sort(ids)
			rs.stops = append(rs.stops, ids)
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success"}) //nolint:errcheck
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	}
}

// stubToggle answers the auto-stop lookup with a fixed value or error.
type stubToggle struct {
	enabled bool
	err     error
}

func (s *stubToggle) ImpactAutoStopEnabled(context.Context) (bool, error) {
	return s.enabled, s.err
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }

func newWorker(t *testing.T, rs *recordingServer, target float64, toggle Toggle) *Worker {
	t.Helper()
	srv := httptest.NewServer(rs.handle(t))
	t.Cleanup(srv.Close)
	client := transmission.NewClient(srv.URL, "u", "p")
	return New(client, target, time.Minute, toggle, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
}

func TestReconcileStopsTorrentsThatPaidBack(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 6, UploadRatio: 1.0, DownloadedEver: 100},
		{ID: 2, Status: 6, UploadRatio: 0.5, DownloadedEver: 100},
		{ID: 3, Status: 4, UploadRatio: -1, UploadedEver: 250, DownloadedEver: 100},
	}}
	w := newWorker(t, rs, 1.0, &stubToggle{enabled: true})

	w.reconcile(context.Background())

	if len(rs.stops) != 1 || !slices.Equal(rs.stops[0], []int64{1, 3}) {
		t.Fatalf("stops = %v, want one batch [1 3]", rs.stops)
	}
}

func TestReconcileDoesNothingWhenDisabled(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 6, UploadRatio: 2.0, DownloadedEver: 100},
	}}
	w := newWorker(t, rs, 1.0, &stubToggle{enabled: false})

	w.reconcile(context.Background())

	if len(rs.stops) != 0 {
		t.Fatalf("stops = %v, want none while auto-stop is off", rs.stops)
	}
}

func TestReconcileTreatsToggleErrorAsOff(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 6, UploadRatio: 2.0, DownloadedEver: 100},
	}}
	w := newWorker(t, rs, 1.0, &stubToggle{enabled: true, err: errors.New("db is gone")})

	w.reconcile(context.Background())

	if len(rs.stops) != 0 {
		t.Fatalf("stops = %v, want none when the toggle cannot be read", rs.stops)
	}
}

func TestReconcileSkipsRPCWhenNothingReached(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 6, UploadRatio: 0.1, DownloadedEver: 100},
	}}
	w := newWorker(t, rs, 1.0, &stubToggle{enabled: true})

	w.reconcile(context.Background())

	if len(rs.stops) != 0 {
		t.Fatalf("stops = %v, want none", rs.stops)
	}
}

func TestStopTargetWithoutToggle(t *testing.T) {
	rs := &recordingServer{}
	w := newWorker(t, rs, 1.5, nil)

	target, on := w.StopTarget(context.Background())
	if target != 1.5 {
		t.Errorf("target = %v, want 1.5", target)
	}
	if on {
		t.Error("auto-stop reported as on without a toggle, want off")
	}
}
