// Package pause implements the global pause switched from the web UI.
//
// Pausing stops every torrent that is not stopped yet and remembers which ones
// they were; resuming starts exactly that set again, so torrents the user had
// stopped on purpose stay stopped. The remembered set is persisted (see
// internal/prefs), so a pause survives a restart. While the pause is on the
// shift schedulers stand still (see internal/shift), otherwise they would
// restart their torrents on the next tick.
package pause

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

// Client is the subset of the Transmission client the pause needs.
type Client interface {
	GetTorrents(ctx context.Context) ([]transmission.Torrent, error)
	StartTorrents(ctx context.Context, ids []int64) error
	StopTorrents(ctx context.Context, ids []int64) error
}

// Store persists the pause and the hashes of the torrents it stopped.
type Store interface {
	PauseState(ctx context.Context) (bool, []string, error)
	SetPauseState(ctx context.Context, paused bool, hashes []string) error
}

// Controller switches the global pause on and off.
type Controller struct {
	client Client
	store  Store
	logger *slog.Logger
	// mu serialises Pause and Resume so two clicks cannot interleave and lose
	// the remembered set.
	mu sync.Mutex
}

// New creates a Controller.
func New(client Client, store Store, logger *slog.Logger) *Controller {
	return &Controller{client: client, store: store, logger: logger}
}

// Paused reports whether the global pause is on.
func (c *Controller) Paused(ctx context.Context) (bool, error) {
	paused, _, err := c.store.PauseState(ctx)
	return paused, err
}

// Pause stops every running torrent and remembers it, returning how many were
// stopped. Pausing twice is a no-op: the second call must not overwrite the set
// remembered by the first one with an empty list.
func (c *Controller) Pause(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	paused, _, err := c.store.PauseState(ctx)
	if err != nil {
		return 0, err
	}
	if paused {
		return 0, nil
	}

	torrents, err := c.client.GetTorrents(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch torrents: %w", err)
	}

	var ids []int64
	hashes := []string{}
	for _, t := range torrents {
		if t.Status == 0 {
			continue
		}
		ids = append(ids, t.ID)
		hashes = append(hashes, t.HashString)
	}

	// Remember the set before stopping: if stopping fails half-way, resuming
	// still brings back everything that was running.
	if err := c.store.SetPauseState(ctx, true, hashes); err != nil {
		return 0, err
	}
	if len(ids) > 0 {
		if err := c.client.StopTorrents(ctx, ids); err != nil {
			return 0, fmt.Errorf("stop torrents: %w", err)
		}
	}

	c.logger.Info("global pause on", "stopped", len(ids), "ids", ids)
	return len(ids), nil
}

// Resume starts the torrents stopped by Pause that still exist and switches the
// pause off, returning how many were started.
func (c *Controller) Resume(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	paused, hashes, err := c.store.PauseState(ctx)
	if err != nil {
		return 0, err
	}
	if !paused {
		return 0, nil
	}

	var ids []int64
	if len(hashes) > 0 {
		torrents, err := c.client.GetTorrents(ctx)
		if err != nil {
			return 0, fmt.Errorf("fetch torrents: %w", err)
		}

		wanted := make(map[string]struct{}, len(hashes))
		for _, h := range hashes {
			wanted[h] = struct{}{}
		}
		for _, t := range torrents {
			if _, ok := wanted[t.HashString]; ok && t.Status == 0 {
				ids = append(ids, t.ID)
			}
		}
	}

	// The pause stays on when starting fails, so the user can retry without
	// losing the remembered set.
	if len(ids) > 0 {
		if err := c.client.StartTorrents(ctx, ids); err != nil {
			return 0, fmt.Errorf("start torrents: %w", err)
		}
	}
	if err := c.store.SetPauseState(ctx, false, nil); err != nil {
		return 0, err
	}

	c.logger.Info("global pause off", "started", len(ids), "ids", ids)
	return len(ids), nil
}

// Active reports whether the pause is on for the background workers. A failed
// lookup counts as "not paused": a broken preference must not freeze the
// schedulers for good.
func (c *Controller) Active(ctx context.Context) bool {
	paused, err := c.Paused(ctx)
	if err != nil {
		c.logger.Warn("failed to read global pause state, assuming not paused", "err", err)
		return false
	}
	return paused
}
