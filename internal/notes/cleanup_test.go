package notes

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

type fakeLister struct {
	torrents []transmission.Torrent
	err      error
	calls    int
}

func (f *fakeLister) GetTorrents(context.Context) ([]transmission.Torrent, error) {
	f.calls++
	return f.torrents, f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSweepRemovesNotesOfMissingTorrents(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()
	store.Set(ctx, hashA, "gone")  //nolint:errcheck
	store.Set(ctx, hashB, "still") //nolint:errcheck

	lister := &fakeLister{torrents: []transmission.Torrent{{HashString: hashB}}}
	NewCleaner(store, lister, time.Minute, discardLogger()).sweep(ctx)

	all, _ := store.All(ctx)
	if len(all) != 1 || all[hashB] != "still" {
		t.Errorf("All() = %v, want only the note of the existing torrent", all)
	}
}

func TestSweepKeepsNotesWhenTransmissionFails(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()
	store.Set(ctx, hashA, "keep me") //nolint:errcheck

	lister := &fakeLister{err: errors.New("connection refused")}
	NewCleaner(store, lister, time.Minute, discardLogger()).sweep(ctx)

	all, _ := store.All(ctx)
	if len(all) != 1 {
		t.Errorf("All() = %v, want the note to survive an unreachable Transmission", all)
	}
}

func TestNewCleanerFallsBackToDefaultInterval(t *testing.T) {
	cleaner := NewCleaner(newStore(t, 200), &fakeLister{}, 0, discardLogger())

	if cleaner.interval != DefaultCleanupInterval {
		t.Errorf("interval = %v, want %v", cleaner.interval, DefaultCleanupInterval)
	}
}

func TestRunSweepsOnStartAndStopsOnContextCancel(t *testing.T) {
	store := newStore(t, 200)
	ctx, cancel := context.WithCancel(context.Background())
	store.Set(ctx, hashA, "orphan") //nolint:errcheck

	lister := &fakeLister{}
	done := make(chan struct{})
	go func() {
		NewCleaner(store, lister, time.Hour, discardLogger()).Run(ctx)
		close(done)
	}()

	// The initial sweep runs before the first tick.
	deadline := time.After(2 * time.Second)
	for {
		all, _ := store.All(context.Background())
		if len(all) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("initial sweep did not run within 2s")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
