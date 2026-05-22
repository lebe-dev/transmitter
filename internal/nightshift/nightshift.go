// Package nightshift schedules start/stop of torrents tagged with the night-shift label.
//
// A torrent is considered "night-shift" when its labels include [Label].
// During the configured window, all such torrents that still need data are started.
// Outside the window, the same torrents are stopped. Already-completed torrents are
// left alone — Transmission keeps them seeding regardless of the schedule.
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

	inWindow := InWindow(s.now(), s.start, s.end)

	var toStart, toStop []int64
	for _, t := range torrents {
		if !slices.Contains(t.Labels, Label) {
			continue
		}
		// Completed torrents seed on their own — Transmission keeps them in
		// "seeding" status (6) or paused after completion. We only manage the
		// download lifecycle.
		if t.PercentDone >= 1 {
			continue
		}
		switch {
		case inWindow && t.Status == 0:
			toStart = append(toStart, t.ID)
		case !inWindow && t.Status != 0:
			toStop = append(toStop, t.ID)
		}
	}

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
