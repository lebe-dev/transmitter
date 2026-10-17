package pause

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

type fakeClient struct {
	torrents []transmission.Torrent
	stopErr  error
	started  []int64
	stopped  []int64
}

func (f *fakeClient) GetTorrents(context.Context) ([]transmission.Torrent, error) {
	return f.torrents, nil
}

func (f *fakeClient) StartTorrents(_ context.Context, ids []int64) error {
	f.started = append(f.started, ids...)
	for i := range f.torrents {
		if slices.Contains(ids, f.torrents[i].ID) {
			f.torrents[i].Status = 4
		}
	}
	return nil
}

func (f *fakeClient) StopTorrents(_ context.Context, ids []int64) error {
	if f.stopErr != nil {
		return f.stopErr
	}
	f.stopped = append(f.stopped, ids...)
	for i := range f.torrents {
		if slices.Contains(ids, f.torrents[i].ID) {
			f.torrents[i].Status = 0
		}
	}
	return nil
}

type fakeStore struct {
	paused bool
	hashes []string
}

func (f *fakeStore) PauseState(context.Context) (bool, []string, error) {
	return f.paused, f.hashes, nil
}

func (f *fakeStore) SetPauseState(_ context.Context, paused bool, hashes []string) error {
	f.paused, f.hashes = paused, hashes
	return nil
}

func newController(client *fakeClient, store *fakeStore) *Controller {
	return New(client, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func torrents() []transmission.Torrent {
	return []transmission.Torrent{
		{ID: 1, HashString: "a", Status: 4}, // downloading
		{ID: 2, HashString: "b", Status: 0}, // stopped by the user
		{ID: 3, HashString: "c", Status: 6}, // seeding
	}
}

func TestPauseThenResumeRestoresPreviousState(t *testing.T) {
	client := &fakeClient{torrents: torrents()}
	store := &fakeStore{}
	c := newController(client, store)
	ctx := context.Background()

	n, err := c.Pause(ctx)
	if err != nil || n != 2 {
		t.Fatalf("Pause() = %d, %v; want 2 stopped", n, err)
	}
	if !store.paused || !slices.Equal(store.hashes, []string{"a", "c"}) {
		t.Fatalf("store = %+v, want paused with [a c]", store)
	}

	n, err = c.Resume(ctx)
	if err != nil || n != 2 {
		t.Fatalf("Resume() = %d, %v; want 2 started", n, err)
	}
	if !slices.Equal(client.started, []int64{1, 3}) {
		t.Errorf("started = %v, want [1 3]: the torrent stopped by the user stays stopped", client.started)
	}
	if store.paused {
		t.Error("pause still on after Resume()")
	}
}

func TestPauseTwiceKeepsRememberedSet(t *testing.T) {
	client := &fakeClient{torrents: torrents()}
	store := &fakeStore{}
	c := newController(client, store)
	ctx := context.Background()

	if _, err := c.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Pause(ctx); err != nil || n != 0 {
		t.Fatalf("second Pause() = %d, %v; want a no-op", n, err)
	}
	if !slices.Equal(store.hashes, []string{"a", "c"}) {
		t.Errorf("hashes = %v, want [a c] kept", store.hashes)
	}
}

func TestResumeSkipsRemovedTorrents(t *testing.T) {
	client := &fakeClient{torrents: []transmission.Torrent{{ID: 3, HashString: "c", Status: 0}}}
	store := &fakeStore{paused: true, hashes: []string{"a", "c"}}

	n, err := newController(client, store).Resume(context.Background())
	if err != nil || n != 1 || !slices.Equal(client.started, []int64{3}) {
		t.Fatalf("Resume() = %d, %v, started %v; want only torrent 3", n, err, client.started)
	}
}

func TestResumeWhenNotPausedIsNoop(t *testing.T) {
	client := &fakeClient{torrents: torrents()}
	n, err := newController(client, &fakeStore{}).Resume(context.Background())
	if err != nil || n != 0 || len(client.started) != 0 {
		t.Fatalf("Resume() = %d, %v, started %v; want no-op", n, err, client.started)
	}
}

func TestPauseKeepsSetWhenStopFails(t *testing.T) {
	client := &fakeClient{torrents: torrents(), stopErr: errors.New("rpc down")}
	store := &fakeStore{}

	if _, err := newController(client, store).Pause(context.Background()); err == nil {
		t.Fatal("Pause() error = nil, want the stop failure")
	}
	if !store.paused || !slices.Equal(store.hashes, []string{"a", "c"}) {
		t.Errorf("store = %+v, want the set remembered so Resume can restore it", store)
	}
}
