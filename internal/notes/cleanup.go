package notes

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

// DefaultCleanupInterval is how often orphaned notes are collected by default.
const DefaultCleanupInterval = time.Hour

// torrentLister is satisfied by *transmission.Client and allows test injection.
type torrentLister interface {
	GetTorrents(ctx context.Context) ([]transmission.Torrent, error)
}

// Cleaner periodically drops notes whose torrent no longer exists in Transmission.
// Notes of torrents removed through Transmitter are deleted right away by the RPC
// proxy; this catches the rest (removals done from another client).
type Cleaner struct {
	store    *Store
	client   torrentLister
	interval time.Duration
	logger   *slog.Logger
}

// NewCleaner creates a Cleaner running on the given interval.
func NewCleaner(store *Store, client torrentLister, interval time.Duration, logger *slog.Logger) *Cleaner {
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	return &Cleaner{store: store, client: client, interval: interval, logger: logger}
}

// Run blocks until ctx is cancelled, sweeping once on start and then on each tick.
func (c *Cleaner) Run(ctx context.Context) {
	c.logger.Info("torrent notes cleaner started", "interval", c.interval)

	c.sweep(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("torrent notes cleaner stopped")
			return
		case <-ticker.C:
			c.sweep(ctx)
		}
	}
}

// sweep deletes notes that no longer have a matching torrent.
func (c *Cleaner) sweep(ctx context.Context) {
	torrents, err := c.client.GetTorrents(ctx)
	if err != nil {
		// Without a torrent list every note would look orphaned — skip this round.
		c.logger.Warn("notes cleanup: failed to fetch torrents", "err", err)
		return
	}

	hashes := make([]string, 0, len(torrents))
	for _, t := range torrents {
		if t.HashString != "" {
			hashes = append(hashes, t.HashString)
		}
	}

	deleted, err := c.store.DeleteExcept(ctx, hashes)
	if err != nil {
		c.logger.Warn("notes cleanup: failed to delete orphaned notes", "err", err)
		return
	}
	if deleted > 0 {
		c.logger.Info("notes cleanup: removed orphaned notes", "count", deleted)
	} else {
		c.logger.Debug("notes cleanup: nothing to remove", "torrents", len(hashes))
	}
}
