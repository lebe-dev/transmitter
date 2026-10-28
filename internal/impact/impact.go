// Package impact stops torrents that gave back everything they took.
//
// "Impact" is the upload ratio of a torrent: how much was uploaded compared to
// how much was downloaded. Once a torrent reaches [Worker]'s target ratio it has
// paid its debt, and — when the user switched automatic stopping on — the worker
// pauses it.
//
// The rule is off by default and lives in internal/prefs, so it is shared by the
// web UI and this worker. Shift schedulers consult the same target through
// [Worker.StopTarget]: without that a shift would restart on its next tick the
// very torrent the worker just paused.
package impact

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

// Toggle reports whether automatic stopping is switched on by the user.
type Toggle interface {
	ImpactAutoStopEnabled(ctx context.Context) (bool, error)
}

// Worker periodically stops torrents that reached the target upload ratio.
type Worker struct {
	client   *transmission.Client
	target   float64
	interval time.Duration
	// toggle gates every reconcile; a nil toggle keeps automatic stopping off.
	toggle Toggle
	logger *slog.Logger
	// lastEnabled remembers the previous toggle state so switching the rule on or
	// off is logged once instead of on every tick. Only touched from Run's goroutine.
	lastEnabled *bool
}

// New creates a Worker that stops torrents whose upload ratio reached target,
// checking every interval as long as toggle reports the rule as enabled.
func New(client *transmission.Client, target float64, interval time.Duration, toggle Toggle, logger *slog.Logger) *Worker {
	return &Worker{
		client:   client,
		target:   target,
		interval: interval,
		toggle:   toggle,
		logger:   logger,
	}
}

// Target returns the upload ratio counted as a full payback.
func (w *Worker) Target() float64 {
	return w.target
}

// StopTarget returns the target ratio and whether automatic stopping is on right
// now. A failed lookup reports the rule as off: leaving a torrent seeding is the
// harmless outcome of a broken preference lookup.
func (w *Worker) StopTarget(ctx context.Context) (float64, bool) {
	if w.toggle == nil {
		return w.target, false
	}
	enabled, err := w.toggle.ImpactAutoStopEnabled(ctx)
	if err != nil {
		w.logger.Warn("impact: failed to read auto-stop state, assuming off", "err", err)
		return w.target, false
	}
	return w.target, enabled
}

// Run blocks until ctx is cancelled, reconciling once on start and then on each tick.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("impact worker started", "target_ratio", w.target, "interval", w.interval)

	w.reconcile(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("impact worker stopped")
			return
		case <-ticker.C:
			w.reconcile(ctx)
		}
	}
}

func (w *Worker) reconcile(ctx context.Context) {
	if !w.enabled(ctx) {
		return
	}

	torrents, err := w.client.GetTorrents(ctx)
	if err != nil {
		w.logger.Warn("impact: failed to fetch torrents", "err", err)
		return
	}

	toStop := ReachedIDs(torrents, w.target)
	if len(toStop) == 0 {
		return
	}

	if err := w.client.StopTorrents(ctx, toStop); err != nil {
		w.logger.Warn("impact: failed to stop torrents", "ids", toStop, "err", err)
		return
	}
	w.logger.Info("impact: stopped torrents that reached the target ratio",
		"target_ratio", w.target, "count", len(toStop), "ids", toStop)
}

// enabled reports whether the user has automatic stopping switched on, logging
// each change of state.
func (w *Worker) enabled(ctx context.Context) bool {
	_, enabled := w.StopTarget(ctx)

	switch {
	case w.lastEnabled == nil:
		w.logger.Info("impact: auto-stop state read", "enabled", enabled)
	case *w.lastEnabled != enabled:
		w.logger.Info("impact: auto-stop switched by user", "enabled", enabled)
	}
	w.lastEnabled = &enabled

	return enabled
}

// ReachedIDs returns the running torrents that reached the target upload ratio.
// Torrents that are already stopped are left out: there is nothing to do.
func ReachedIDs(torrents []transmission.Torrent, target float64) []int64 {
	var ids []int64
	for _, t := range torrents {
		if t.Status != 0 && Reached(t, target) {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// Reached reports whether a torrent uploaded at least target times what it
// downloaded. A torrent that downloaded nothing never counts as reached: there
// is no debt to pay back, and Transmission reports its ratio as negative.
func Reached(t transmission.Torrent, target float64) bool {
	if target <= 0 || t.DownloadedEver <= 0 {
		return false
	}
	return Ratio(t) >= target
}

// Ratio returns the torrent's upload ratio, falling back to the raw counters
// when Transmission reports the ratio as unknown (a negative value).
func Ratio(t transmission.Torrent) float64 {
	if t.UploadRatio >= 0 {
		return t.UploadRatio
	}
	if t.DownloadedEver <= 0 {
		return 0
	}
	return float64(t.UploadedEver) / float64(t.DownloadedEver)
}
