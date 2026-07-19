// Package nightshift schedules start/stop of torrents tagged with the night-shift label.
//
// A torrent is considered "night-shift" when its labels include [Label].
// During the configured window all such torrents are started (downloading or
// seeding, depending on progress). Outside the window the same torrents are
// stopped.
package nightshift

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

// Label is the Transmission label that marks a torrent as part of the night shift.
const Label = "night-shift"

// Scheduler periodically reconciles night-shift torrent state with the configured time window.
type Scheduler struct {
	client   *transmission.Client
	start    config.DayTime
	end      config.DayTime
	interval time.Duration
	now      func() time.Time
	logger   *slog.Logger
}

// New creates a Scheduler that reconciles night-shift torrents on the given interval.
func New(client *transmission.Client, cfg *config.Config, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		client:   client,
		start:    cfg.NightShiftStart,
		end:      cfg.NightShiftEnd,
		interval: cfg.NightShiftInterval,
		now:      time.Now,
		logger:   logger,
	}
}

// Run blocks until ctx is cancelled, reconciling once on start and then on each tick.
func (s *Scheduler) Run(ctx context.Context) {
	s.logger.Info("night-shift scheduler started",
		"start", s.start.String(),
		"end", s.end.String(),
		"interval", s.interval,
	)

	s.reconcile(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("night-shift scheduler stopped")
			return
		case <-ticker.C:
			s.reconcile(ctx)
		}
	}
}

func (s *Scheduler) reconcile(ctx context.Context) {
	torrents, err := s.client.GetTorrents(ctx)
	if err != nil {
		s.logger.Warn("night-shift: failed to fetch torrents", "err", err)
		return
	}

	now := s.now()
	inWindow := InWindow(now, s.start, s.end)

	tagged, toStart, toStop := classify(torrents, inWindow)

	s.logger.Info("night-shift: reconcile",
		"now", now.Format("15:04 MST"),
		"window", s.start.String()+"-"+s.end.String(),
		"in_window", inWindow,
		"torrents_total", len(torrents),
		"tagged", len(tagged),
		"tagged_ids", tagged,
		"to_start", toStart,
		"to_stop", toStop,
	)

	if len(toStart) > 0 {
		if err := s.client.StartTorrents(ctx, toStart); err != nil {
			s.logger.Warn("night-shift: failed to start torrents", "ids", toStart, "err", err)
		} else {
			s.logger.Info("night-shift: started torrents", "count", len(toStart), "ids", toStart)
		}
	}
	if len(toStop) > 0 {
		if err := s.client.StopTorrents(ctx, toStop); err != nil {
			s.logger.Warn("night-shift: failed to stop torrents", "ids", toStop, "err", err)
		} else {
			s.logger.Info("night-shift: stopped torrents", "count", len(toStop), "ids", toStop)
		}
	}
}

// classify splits torrents into those tagged for the night shift and the subsets
// that need starting or stopping given whether the current time is inside the window.
func classify(torrents []transmission.Torrent, inWindow bool) (tagged, toStart, toStop []int64) {
	for _, t := range torrents {
		if !slices.Contains(t.Labels, Label) {
			continue
		}
		tagged = append(tagged, t.ID)
		// Apply the same window to both downloading and seeding torrents:
		// completed (100%) torrents resume seeding inside the window and are
		// paused outside of it, just like in-progress downloads.
		switch {
		case inWindow && t.Status == 0:
			toStart = append(toStart, t.ID)
		case !inWindow && t.Status != 0:
			toStop = append(toStop, t.ID)
		}
	}
	return tagged, toStart, toStop
}

// InWindow reports whether the given moment falls inside the [start, end) window.
// The window wraps over midnight when end <= start (e.g. 23:00..07:00).
func InWindow(now time.Time, start, end config.DayTime) bool {
	cur := now.Hour()*60 + now.Minute()
	a, b := start.Minutes(), end.Minutes()
	if a == b {
		return false
	}
	if a < b {
		return cur >= a && cur < b
	}
	return cur >= a || cur < b
}
