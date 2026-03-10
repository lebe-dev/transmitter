package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

// fakeGetter implements torrentGetter for tests.
type fakeGetter struct {
	torrents []transmission.Torrent
	err      error
}

func (f *fakeGetter) GetTorrents(_ context.Context) ([]transmission.Torrent, error) {
	return f.torrents, f.err
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newMonitorBot creates a Bot wired for monitor tests.
func newMonitorBot(g *fakeGetter, notifyFn func(string)) *Bot {
	b := &Bot{
		logger:   nopLogger(),
		getter:   g,
		notifyFn: notifyFn,
	}
	return b
}

func TestPollOnce_FirstPollNoNotification(t *testing.T) {
	called := 0
	g := &fakeGetter{torrents: []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 0.5},
	}}
	b := newMonitorBot(g, func(string) { called++ })

	b.pollOnce(context.Background())

	if called != 0 {
		t.Fatalf("expected 0 notifications on first poll, got %d", called)
	}
	if b.progress == nil {
		t.Fatal("expected progress to be initialized")
	}
	if b.progress[1] != 0.5 {
		t.Fatalf("expected progress[1]=0.5, got %v", b.progress[1])
	}
}

func TestPollOnce_CompletionDetected(t *testing.T) {
	ch := make(chan string, 1)

	g := &fakeGetter{torrents: []transmission.Torrent{
		{ID: 1, Name: "MyTorrent", PercentDone: 0.5, TotalSize: 1 << 20},
	}}
	b := newMonitorBot(g, func(msg string) { ch <- msg })

	// First poll — snapshot
	b.pollOnce(context.Background())

	// Second poll — torrent completes
	g.torrents = []transmission.Torrent{
		{ID: 1, Name: "MyTorrent", PercentDone: 1.0, TotalSize: 1 << 20},
	}
	b.pollOnce(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case msg := <-ch:
		if !strings.Contains(msg, "MyTorrent") {
			t.Errorf("expected message to contain torrent name, got: %s", msg)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for completion notification")
	}
}

func TestPollOnce_AlreadyComplete_NoSpam(t *testing.T) {
	called := 0
	g := &fakeGetter{torrents: []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 1.0},
	}}
	b := newMonitorBot(g, func(string) { called++ })

	b.pollOnce(context.Background()) // snapshot at 1.0
	b.pollOnce(context.Background()) // stays at 1.0

	if called != 0 {
		t.Fatalf("expected 0 notifications, got %d", called)
	}
}

func TestPollOnce_NewTorrent_NoNotification(t *testing.T) {
	called := 0
	g := &fakeGetter{torrents: []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 0.5},
	}}
	b := newMonitorBot(g, func(string) { called++ })

	b.pollOnce(context.Background()) // snapshot

	// New torrent appears already complete
	g.torrents = []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 0.5},
		{ID: 2, Name: "B", PercentDone: 1.0},
	}
	b.pollOnce(context.Background())

	if called != 0 {
		t.Fatalf("expected 0 notifications for new torrent appearing complete, got %d", called)
	}
}

func TestPollOnce_RemovedTorrentPruned(t *testing.T) {
	g := &fakeGetter{torrents: []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 0.5},
		{ID: 2, Name: "B", PercentDone: 0.8},
	}}
	b := newMonitorBot(g, func(string) {})

	b.pollOnce(context.Background()) // snapshot with 2 torrents

	// Torrent 2 removed
	g.torrents = []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 0.5},
	}
	b.pollOnce(context.Background())

	b.mu.Lock()
	size := len(b.progress)
	_, has2 := b.progress[2]
	b.mu.Unlock()

	if size != 1 {
		t.Fatalf("expected progress map size 1, got %d", size)
	}
	if has2 {
		t.Fatal("expected torrent 2 to be pruned from progress map")
	}
}

func TestPollOnce_TransmissionError_NoStateChange(t *testing.T) {
	g := &fakeGetter{torrents: []transmission.Torrent{
		{ID: 1, Name: "A", PercentDone: 0.5},
	}}
	b := newMonitorBot(g, func(string) {})

	b.pollOnce(context.Background()) // snapshot

	// Simulate error
	g.torrents = nil
	g.err = errors.New("connection refused")
	b.pollOnce(context.Background())

	b.mu.Lock()
	prog := b.progress[1]
	b.mu.Unlock()

	if prog != 0.5 {
		t.Fatalf("expected progress unchanged at 0.5, got %v", prog)
	}
}

func TestFormatCompletionMessage(t *testing.T) {
	msg := formatCompletionMessage("<Evil &Name>", 1<<20)
	if !strings.Contains(msg, "&lt;Evil &amp;Name&gt;") {
		t.Errorf("expected HTML-escaped name in message, got: %s", msg)
	}
	if !strings.Contains(msg, "1.0 MB") {
		t.Errorf("expected size in message, got: %s", msg)
	}
}
