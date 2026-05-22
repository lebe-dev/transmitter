package nightshift

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

func TestInWindow(t *testing.T) {
	dt := func(h, m int) config.DayTime { return config.DayTime{Hour: h, Minute: m} }
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }

	cases := []struct {
		name       string
		now        time.Time
		start, end config.DayTime
		want       bool
	}{
		{"linear inside", at(2, 0), dt(1, 0), dt(3, 0), true},
		{"linear before", at(0, 30), dt(1, 0), dt(3, 0), false},
		{"linear after", at(3, 0), dt(1, 0), dt(3, 0), false},
		{"linear exactly start", at(1, 0), dt(1, 0), dt(3, 0), true},
		{"wrap inside late evening", at(23, 30), dt(23, 0), dt(7, 0), true},
		{"wrap inside early morning", at(3, 0), dt(23, 0), dt(7, 0), true},
		{"wrap outside daytime", at(12, 0), dt(23, 0), dt(7, 0), false},
		{"wrap at end (excluded)", at(7, 0), dt(23, 0), dt(7, 0), false},
		{"equal start/end never matches", at(5, 0), dt(5, 0), dt(5, 0), false},
		{"minute precision", at(23, 29), dt(23, 30), dt(7, 0), false},
		{"minute precision inside", at(23, 30), dt(23, 30), dt(7, 0), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InWindow(tc.now, tc.start, tc.end); got != tc.want {
				t.Errorf("InWindow(%v, %s, %s) = %v, want %v", tc.now, tc.start, tc.end, got, tc.want)
			}
		})
	}
}

type recordingServer struct {
	mu       sync.Mutex
	torrents []transmission.Torrent
	starts   [][]int64
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
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success", Arguments: args})
		case "torrent-start":
			var a transmission.TorrentActionArgs
			_ = json.Unmarshal(req.Arguments, &a)
			ids := append([]int64(nil), a.IDs...)
			slices.Sort(ids)
			rs.starts = append(rs.starts, ids)
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success"})
		case "torrent-stop":
			var a transmission.TorrentActionArgs
			_ = json.Unmarshal(req.Arguments, &a)
			ids := append([]int64(nil), a.IDs...)
			slices.Sort(ids)
			rs.stops = append(rs.stops, ids)
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success"})
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	}
}

func newScheduler(t *testing.T, rs *recordingServer, now time.Time, start, end config.DayTime) *Scheduler {
	t.Helper()
	srv := httptest.NewServer(rs.handle(t))
	t.Cleanup(srv.Close)
	client := transmission.NewClient(srv.URL, "u", "p")

	cfg := &config.Config{
		NightShiftEnabled:  true,
		NightShiftStart:    start,
		NightShiftEnd:      end,
		NightShiftInterval: time.Minute,
	}
	s := New(client, cfg, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	s.now = func() time.Time { return now }
	return s
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }

func TestReconcileStartsTaggedDuringWindow(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 0, PercentDone: 0.2, Labels: []string{"night-shift"}},
		{ID: 2, Status: 0, PercentDone: 0.5, Labels: []string{"other"}},
		{ID: 3, Status: 4, PercentDone: 0.3, Labels: []string{"night-shift"}}, // already running
		{ID: 4, Status: 0, PercentDone: 1.0, Labels: []string{"night-shift"}}, // completed, skip
	}}
	s := newScheduler(t, rs, time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 1 || len(rs.starts[0]) != 1 || rs.starts[0][0] != 1 {
		t.Errorf("starts = %v, want [[1]]", rs.starts)
	}
	if len(rs.stops) != 0 {
		t.Errorf("stops = %v, want none", rs.stops)
	}
}

func TestReconcileStopsTaggedOutsideWindow(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 4, PercentDone: 0.2, Labels: []string{"night-shift"}},
		{ID: 2, Status: 3, PercentDone: 0.5, Labels: []string{"night-shift"}},
		{ID: 3, Status: 0, PercentDone: 0.3, Labels: []string{"night-shift"}}, // already stopped
		{ID: 4, Status: 4, PercentDone: 0.5, Labels: []string{}},              // untagged, untouched
		{ID: 5, Status: 6, PercentDone: 1.0, Labels: []string{"night-shift"}}, // completed, seeding -> skip
	}}
	s := newScheduler(t, rs, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 0 {
		t.Errorf("starts = %v, want none", rs.starts)
	}
	if len(rs.stops) != 1 {
		t.Fatalf("stops = %v, want one batch", rs.stops)
	}
	got := rs.stops[0]
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("stop ids = %v, want [1 2]", got)
	}
}

func TestReconcileNoOpWhenNothingTagged(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 4, PercentDone: 0.2, Labels: []string{}},
	}}
	s := newScheduler(t, rs, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 0 || len(rs.stops) != 0 {
		t.Errorf("expected no calls, starts=%v stops=%v", rs.starts, rs.stops)
	}
}
