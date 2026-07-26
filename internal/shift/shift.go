// Package shift schedules start/stop of torrents tagged with a shift label.
//
// A torrent belongs to a shift when its labels include the shift's [Options.Label].
// During the configured window all such torrents are started (downloading or
// seeding, depending on progress). Outside the window the same torrents are
// stopped.
//
// Two shifts are supported: the night shift ([NightLabel]) and the day shift
// ([DayLabel]). They run as independent schedulers and are not coordinated: a
// torrent carrying both labels is claimed by both, so with non-overlapping
// windows the schedulers will start and stop it in turn.
package shift

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

// Transmission labels that mark a torrent as part of a shift.
const (
	NightLabel = "night-shift"
	DayLabel   = "day-shift"
)

// Options describes a single shift: its label, time window and reconcile interval.
type Options struct {
	// Name identifies the shift in log messages.
	Name     string
	Label    string
	Start    config.DayTime
	End      config.DayTime
	Interval time.Duration
}

// NightOptions builds the night-shift options from the application config.
func NightOptions(cfg *config.Config) Options {
	return Options{
		Name:     NightLabel,
		Label:    NightLabel,
		Start:    cfg.NightShiftStart,
		End:      cfg.NightShiftEnd,
		Interval: cfg.NightShiftInterval,
	}
}

// DayOptions builds the day-shift options from the application config.
func DayOptions(cfg *config.Config) Options {
	return Options{
		Name:     DayLabel,
		Label:    DayLabel,
		Start:    cfg.DayShiftStart,
		End:      cfg.DayShiftEnd,
		Interval: cfg.DayShiftInterval,
	}
}

// Scheduler periodically reconciles shift torrent state with the configured time window.
type Scheduler struct {
	client *transmission.Client
	opts   Options
	now    func() time.Time
	logger *slog.Logger
}

// New creates a Scheduler that reconciles the given shift's torrents on its interval.
func New(client *transmission.Client, opts Options, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		client: client,
		opts:   opts,
		now:    time.Now,
		logger: logger,
	}
}

// Run blocks until ctx is cancelled, reconciling once on start and then on each tick.
func (s *Scheduler) Run(ctx context.Context) {
	s.logger.Info(s.opts.Name+" scheduler started",
		"start", s.opts.Start.String(),
		"end", s.opts.End.String(),
		"interval", s.opts.Interval,
	)

	s.reconcile(ctx)

	ticker := time.NewTicker(s.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info(s.opts.Name + " scheduler stopped")
			return
		case <-ticker.C:
			s.reconcile(ctx)
		}
	}
}

func (s *Scheduler) reconcile(ctx context.Context) {
	torrents, err := s.client.GetTorrents(ctx)
	if err != nil {
		s.logger.Warn(s.opts.Name+": failed to fetch torrents", "err", err)
		return
	}

	now := s.now()
	inWindow := InWindow(now, s.opts.Start, s.opts.End)

	tagged, toStart, toStop := classify(torrents, s.opts.Label, inWindow)

	s.logger.Info(s.opts.Name+": reconcile",
		"now", now.Format("15:04 MST"),
		"window", s.opts.Start.String()+"-"+s.opts.End.String(),
		"in_window", inWindow,
		"torrents_total", len(torrents),
		"tagged", len(tagged),
		"tagged_ids", tagged,
		"to_start", toStart,
		"to_stop", toStop,
	)

	if len(toStart) > 0 {
		if err := s.client.StartTorrents(ctx, toStart); err != nil {
			s.logger.Warn(s.opts.Name+": failed to start torrents", "ids", toStart, "err", err)
		} else {
			s.logger.Info(s.opts.Name+": started torrents", "count", len(toStart), "ids", toStart)
		}
	}
	if len(toStop) > 0 {
		if err := s.client.StopTorrents(ctx, toStop); err != nil {
			s.logger.Warn(s.opts.Name+": failed to stop torrents", "ids", toStop, "err", err)
		} else {
			s.logger.Info(s.opts.Name+": stopped torrents", "count", len(toStop), "ids", toStop)
		}
	}
}

// classify splits torrents into those tagged with the given label and the subsets
// that need starting or stopping given whether the current time is inside the window.
func classify(torrents []transmission.Torrent, label string, inWindow bool) (tagged, toStart, toStop []int64) {
	for _, t := range torrents {
		if !slices.Contains(t.Labels, label) {
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
