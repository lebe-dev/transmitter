// Package shift schedules start/stop of torrents tagged with a shift label.
//
// A torrent belongs to a shift when its labels include the shift's [Options.Label].
// During the configured window all such torrents are started (downloading or
// seeding, depending on progress). Outside the window the same torrents are
// stopped.
//
// Two shifts are supported: the night shift ([NightLabel]) and the day shift
// ([DayLabel]). They run as independent schedulers, but a torrent carrying both
// labels is claimed by both, so stopping is coordinated: a scheduler leaves a
// torrent alone when another enabled shift claims it and that shift's window is
// open right now. In other words a torrent runs while any of its shifts is on
// duty and is stopped once all of them are off.
//
// A scheduler can be switched off from the UI (see [Toggle]). While it is off
// the scheduler touches nothing at all: torrents keep whatever state they are
// in, including the ones it paused before.
package shift

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/impact"
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

// Impact reports the upload ratio at which a torrent counts as fully paid back,
// and whether torrents reaching it are stopped automatically right now (see
// internal/impact). While that rule is on, a shift must not restart such a
// torrent: it would undo the stop on every tick.
type Impact interface {
	StopTarget(ctx context.Context) (float64, bool)
}

// Toggle reports whether a shift is currently switched on by the user.
// The state is persisted outside the scheduler (see internal/prefs) so it
// survives a restart.
type Toggle interface {
	ShiftEnabled(ctx context.Context, shift string) (bool, error)
}

// Scheduler periodically reconciles shift torrent state with the configured time window.
type Scheduler struct {
	client *transmission.Client
	opts   Options
	// peers are the other configured shifts. They are consulted before stopping:
	// a torrent that also belongs to a peer whose window is open stays running.
	peers []Options
	// toggle gates every reconcile; a nil toggle keeps the shift always on.
	toggle Toggle
	// impact keeps the shift from restarting torrents that already paid back;
	// a nil impact means the rule never applies.
	impact Impact
	now    func() time.Time
	logger *slog.Logger
	// lastEnabled remembers the previous toggle state so switching a shift on or
	// off is logged once instead of on every tick. Only touched from Run's goroutine.
	lastEnabled *bool
}

// New creates a Scheduler that reconciles the given shift's torrents on its
// interval, as long as toggle reports the shift as enabled. all lists every
// configured shift (opts included); the rest become this scheduler's peers.
// impact may be nil; otherwise it holds back torrents that reached their upload
// target while automatic stopping is on.
func New(client *transmission.Client, opts Options, all []Options, toggle Toggle, impact Impact, logger *slog.Logger) *Scheduler {
	var peers []Options
	for _, o := range all {
		if o.Label != opts.Label {
			peers = append(peers, o)
		}
	}

	return &Scheduler{
		client: client,
		opts:   opts,
		peers:  peers,
		toggle: toggle,
		impact: impact,
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
	if !s.enabled(ctx) {
		return
	}

	torrents, err := s.client.GetTorrents(ctx)
	if err != nil {
		s.logger.Warn(s.opts.Name+": failed to fetch torrents", "err", err)
		return
	}

	now := s.now()
	inWindow := InWindow(now, s.opts.Start, s.opts.End)

	// Outside the window a torrent may still be on duty for another shift.
	var onDuty []string
	if !inWindow {
		onDuty = s.peersOnDuty(ctx, now)
	}

	tagged, toStart, toStop := classify(torrents, s.opts.Label, inWindow, onDuty, s.impactTarget(ctx))

	s.logger.Info(s.opts.Name+": reconcile",
		"now", now.Format("15:04 MST"),
		"window", s.opts.Start.String()+"-"+s.opts.End.String(),
		"in_window", inWindow,
		"peers_on_duty", onDuty,
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

// enabled reports whether the user has the shift switched on, logging each
// change of state. A failed lookup is treated as enabled: a broken preference
// must not silently leave tagged torrents unmanaged.
func (s *Scheduler) enabled(ctx context.Context) bool {
	if s.toggle == nil {
		return true
	}

	enabled, err := s.toggle.ShiftEnabled(ctx, s.opts.Name)
	if err != nil {
		s.logger.Warn(s.opts.Name+": failed to read enabled state, assuming enabled", "err", err)
		return true
	}

	switch {
	case s.lastEnabled == nil:
		s.logger.Info(s.opts.Name+": enabled state read", "enabled", enabled)
	case *s.lastEnabled != enabled:
		s.logger.Info(s.opts.Name+": switched by user", "enabled", enabled)
	}
	s.lastEnabled = &enabled

	return enabled
}

// impactTarget returns the upload ratio above which torrents must not be
// started, or 0 when automatic stopping is off and the shift may start anything.
func (s *Scheduler) impactTarget(ctx context.Context) float64 {
	if s.impact == nil {
		return 0
	}
	target, on := s.impact.StopTarget(ctx)
	if !on {
		return 0
	}
	return target
}

// peersOnDuty returns the labels of the other configured shifts that are
// switched on and whose window is open at the given moment. A peer whose
// enabled state cannot be read counts as on duty: keeping a torrent seeding is
// the harmless outcome of a broken preference lookup.
func (s *Scheduler) peersOnDuty(ctx context.Context, now time.Time) []string {
	var labels []string
	for _, p := range s.peers {
		if !InWindow(now, p.Start, p.End) {
			continue
		}
		if s.toggle != nil {
			enabled, err := s.toggle.ShiftEnabled(ctx, p.Name)
			if err != nil {
				s.logger.Warn(s.opts.Name+": failed to read peer enabled state, assuming enabled",
					"peer", p.Name, "err", err)
			} else if !enabled {
				continue
			}
		}
		labels = append(labels, p.Label)
	}
	return labels
}

// classify splits torrents into those tagged with the given label and the subsets
// that need starting or stopping given whether the current time is inside the window.
// Torrents also carrying one of the onDuty labels are never stopped: another shift
// is running them right now. A positive impactTarget holds back torrents that
// already uploaded that much of what they downloaded — the impact worker stops
// them, so starting them again would just fight it.
func classify(torrents []transmission.Torrent, label string, inWindow bool, onDuty []string, impactTarget float64) (tagged, toStart, toStop []int64) {
	for _, t := range torrents {
		if !slices.Contains(t.Labels, label) {
			continue
		}
		tagged = append(tagged, t.ID)
		// Apply the same window to both downloading and seeding torrents:
		// completed (100%) torrents resume seeding inside the window and are
		// paused outside of it, just like in-progress downloads.
		switch {
		case inWindow && t.Status == 0 && !impact.Reached(t, impactTarget):
			toStart = append(toStart, t.ID)
		case !inWindow && t.Status != 0 && !claimedBy(t, onDuty):
			toStop = append(toStop, t.ID)
		}
	}
	return tagged, toStart, toStop
}

// claimedBy reports whether the torrent carries any of the given shift labels.
func claimedBy(t transmission.Torrent, labels []string) bool {
	return slices.ContainsFunc(labels, func(l string) bool { return slices.Contains(t.Labels, l) })
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
