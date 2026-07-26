package shift

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
		{"day window inside", at(12, 0), dt(8, 0), dt(22, 0), true},
		{"day window before", at(7, 59), dt(8, 0), dt(22, 0), false},
		{"day window at end (excluded)", at(22, 0), dt(8, 0), dt(22, 0), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InWindow(tc.now, tc.start, tc.end); got != tc.want {
				t.Errorf("InWindow(%v, %s, %s) = %v, want %v", tc.now, tc.start, tc.end, got, tc.want)
			}
		})
	}
}

func TestNightOptions(t *testing.T) {
	cfg := &config.Config{
		NightShiftStart:    config.DayTime{Hour: 23, Minute: 0},
		NightShiftEnd:      config.DayTime{Hour: 7, Minute: 0},
		NightShiftInterval: 2 * time.Minute,
	}
	opts := NightOptions(cfg)
	if opts.Label != NightLabel || opts.Name != NightLabel {
		t.Errorf("label/name = %q/%q, want %q", opts.Label, opts.Name, NightLabel)
	}
	if opts.Start != cfg.NightShiftStart || opts.End != cfg.NightShiftEnd || opts.Interval != cfg.NightShiftInterval {
		t.Errorf("window = %s-%s/%s, want 23:00-07:00/2m0s", opts.Start, opts.End, opts.Interval)
	}
}

func TestDayOptions(t *testing.T) {
	cfg := &config.Config{
		DayShiftStart:    config.DayTime{Hour: 8, Minute: 0},
		DayShiftEnd:      config.DayTime{Hour: 22, Minute: 0},
		DayShiftInterval: 30 * time.Second,
	}
	opts := DayOptions(cfg)
	if opts.Label != DayLabel || opts.Name != DayLabel {
		t.Errorf("label/name = %q/%q, want %q", opts.Label, opts.Name, DayLabel)
	}
	if opts.Start != cfg.DayShiftStart || opts.End != cfg.DayShiftEnd || opts.Interval != cfg.DayShiftInterval {
		t.Errorf("window = %s-%s/%s, want 08:00-22:00/30s", opts.Start, opts.End, opts.Interval)
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
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success", Arguments: args}) //nolint:errcheck
		case "torrent-start":
			var a transmission.TorrentActionArgs
			_ = json.Unmarshal(req.Arguments, &a)
			ids := append([]int64(nil), a.IDs...)
			slices.Sort(ids)
			rs.starts = append(rs.starts, ids)
			json.NewEncoder(w).Encode(transmission.RPCResponse{Result: "success"}) //nolint:errcheck
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

func newScheduler(t *testing.T, rs *recordingServer, label string, now time.Time, start, end config.DayTime) *Scheduler {
	t.Helper()
	return newSchedulerWithToggle(t, rs, label, now, start, end, nil)
}

func newSchedulerWithToggle(t *testing.T, rs *recordingServer, label string, now time.Time, start, end config.DayTime, toggle Toggle) *Scheduler {
	t.Helper()
	srv := httptest.NewServer(rs.handle(t))
	t.Cleanup(srv.Close)
	client := transmission.NewClient(srv.URL, "u", "p")

	opts := Options{
		Name:     label,
		Label:    label,
		Start:    start,
		End:      end,
		Interval: time.Minute,
	}
	s := New(client, opts, toggle, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	s.now = func() time.Time { return now }
	return s
}

// stubToggle answers the enabled lookup with a fixed value or error.
type stubToggle struct {
	enabled bool
	err     error
	shift   string
}

func (s *stubToggle) ShiftEnabled(_ context.Context, shift string) (bool, error) {
	s.shift = shift
	return s.enabled, s.err
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }

func TestReconcileStartsTaggedDuringWindow(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 0, PercentDone: 0.2, Labels: []string{NightLabel}}, // start downloading
		{ID: 2, Status: 0, PercentDone: 0.5, Labels: []string{"other"}},    // wrong label
		{ID: 3, Status: 4, PercentDone: 0.3, Labels: []string{NightLabel}}, // already running
		{ID: 4, Status: 0, PercentDone: 1.0, Labels: []string{NightLabel}}, // completed, start seeding
	}}
	s := newScheduler(t, rs, NightLabel, time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 1 {
		t.Fatalf("starts = %v, want one batch", rs.starts)
	}
	got := rs.starts[0]
	if len(got) != 2 || got[0] != 1 || got[1] != 4 {
		t.Errorf("start ids = %v, want [1 4]", got)
	}
	if len(rs.stops) != 0 {
		t.Errorf("stops = %v, want none", rs.stops)
	}
}

func TestReconcileStopsTaggedOutsideWindow(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 4, PercentDone: 0.2, Labels: []string{NightLabel}}, // stop downloading
		{ID: 2, Status: 3, PercentDone: 0.5, Labels: []string{NightLabel}}, // stop queued
		{ID: 3, Status: 0, PercentDone: 0.3, Labels: []string{NightLabel}}, // already stopped
		{ID: 4, Status: 4, PercentDone: 0.5, Labels: []string{}},           // untagged, untouched
		{ID: 5, Status: 6, PercentDone: 1.0, Labels: []string{NightLabel}}, // stop seeding (completed)
	}}
	s := newScheduler(t, rs, NightLabel, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 0 {
		t.Errorf("starts = %v, want none", rs.starts)
	}
	if len(rs.stops) != 1 {
		t.Fatalf("stops = %v, want one batch", rs.stops)
	}
	got := rs.stops[0]
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 5 {
		t.Errorf("stop ids = %v, want [1 2 5]", got)
	}
}

func TestReconcileNoOpWhenNothingTagged(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 4, PercentDone: 0.2, Labels: []string{}},
	}}
	s := newScheduler(t, rs, NightLabel, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 0 || len(rs.stops) != 0 {
		t.Errorf("expected no calls, starts=%v stops=%v", rs.starts, rs.stops)
	}
}

func TestReconcileDayShiftIgnoresNightLabel(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 0, PercentDone: 0.2, Labels: []string{DayLabel}},   // start, day-tagged
		{ID: 2, Status: 0, PercentDone: 0.2, Labels: []string{NightLabel}}, // other shift, untouched
	}}
	s := newScheduler(t, rs, DayLabel, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		config.DayTime{Hour: 8, Minute: 0}, config.DayTime{Hour: 22, Minute: 0})

	s.reconcile(context.Background())

	if len(rs.starts) != 1 || len(rs.starts[0]) != 1 || rs.starts[0][0] != 1 {
		t.Errorf("starts = %v, want [[1]]", rs.starts)
	}
	if len(rs.stops) != 0 {
		t.Errorf("stops = %v, want none", rs.stops)
	}
}

// A shift switched off in the UI must not touch torrents at all: the ones it
// paused earlier stay paused until the user resumes them.
func TestReconcileSkippedWhenShiftDisabled(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 0, PercentDone: 0.2, Labels: []string{NightLabel}},
		{ID: 2, Status: 4, PercentDone: 0.2, Labels: []string{NightLabel}},
	}}
	toggle := &stubToggle{enabled: false}
	s := newSchedulerWithToggle(t, rs, NightLabel, time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0}, toggle)

	s.reconcile(context.Background())

	if len(rs.starts) != 0 || len(rs.stops) != 0 {
		t.Errorf("expected no calls while disabled, starts=%v stops=%v", rs.starts, rs.stops)
	}
	if toggle.shift != NightLabel {
		t.Errorf("toggle queried with %q, want %q", toggle.shift, NightLabel)
	}
}

func TestReconcileRunsWhenShiftEnabled(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 0, PercentDone: 0.2, Labels: []string{NightLabel}},
	}}
	s := newSchedulerWithToggle(t, rs, NightLabel, time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0}, &stubToggle{enabled: true})

	s.reconcile(context.Background())

	if len(rs.starts) != 1 || len(rs.starts[0]) != 1 || rs.starts[0][0] != 1 {
		t.Errorf("starts = %v, want [[1]]", rs.starts)
	}
}

// A failing preference lookup must not leave tagged torrents unmanaged.
func TestReconcileRunsWhenToggleLookupFails(t *testing.T) {
	rs := &recordingServer{torrents: []transmission.Torrent{
		{ID: 1, Status: 0, PercentDone: 0.2, Labels: []string{NightLabel}},
	}}
	s := newSchedulerWithToggle(t, rs, NightLabel, time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC),
		config.DayTime{Hour: 23, Minute: 0}, config.DayTime{Hour: 7, Minute: 0},
		&stubToggle{err: errors.New("db down")})

	s.reconcile(context.Background())

	if len(rs.starts) != 1 {
		t.Errorf("starts = %v, want the shift to keep running on a lookup error", rs.starts)
	}
}

// A torrent carrying both labels belongs to both shifts; the schedulers are not
// coordinated, so each one acts on it according to its own window.
func TestClassifyTorrentWithBothLabels(t *testing.T) {
	torrents := []transmission.Torrent{
		{ID: 7, Status: 0, PercentDone: 0.5, Labels: []string{NightLabel, DayLabel}},
	}

	for _, label := range []string{NightLabel, DayLabel} {
		tagged, toStart, toStop := classify(torrents, label, true)
		if len(tagged) != 1 || tagged[0] != 7 {
			t.Errorf("%s: tagged = %v, want [7]", label, tagged)
		}
		if len(toStart) != 1 || toStart[0] != 7 {
			t.Errorf("%s: toStart = %v, want [7]", label, toStart)
		}
		if len(toStop) != 0 {
			t.Errorf("%s: toStop = %v, want none", label, toStop)
		}
	}
}
