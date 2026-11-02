package bot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lebe-dev/transmitter/internal/shift"
	"gopkg.in/telebot.v4"
)

// Callback data prefixes for the shift card: "sh:<label>:<0|1>" toggles a shift,
// shiftsRefreshData redraws the card.
const (
	shiftTogglePrefix = "sh:"
	shiftsRefreshData = "shr"
)

// errUnknownShift marks callback data naming a shift that is not configured.
var errUnknownShift = errors.New("unknown shift")

// ShiftStore reads and writes the on/off state of the shift schedulers.
// It is satisfied by *prefs.Store.
type ShiftStore interface {
	ShiftEnabled(ctx context.Context, shift string) (bool, error)
	SetShiftEnabled(ctx context.Context, shift string, enabled bool) error
}

// Shifts gives the bot control over the shift schedulers: the windows configured
// in the environment plus the store holding their on/off state. A zero value
// means no shift is configured and /shifts only reports that.
type Shifts struct {
	Configured []shift.Options
	Store      ShiftStore
}

// shiftView is one shift rendered into the card: its window, whether that window
// is open right now and whether the user has the scheduler switched on.
type shiftView struct {
	Label    string
	Window   string
	Interval time.Duration
	InWindow bool
	Enabled  bool
}

// shiftViews builds the card state for the configured shifts. A shift missing
// from enabled is treated as switched on — the same default as the store and the
// scheduler, so a failed lookup never shows a running shift as off.
func shiftViews(opts []shift.Options, now time.Time, enabled map[string]bool) []shiftView {
	views := make([]shiftView, 0, len(opts))
	for _, o := range opts {
		on, ok := enabled[o.Label]
		views = append(views, shiftView{
			Label:    o.Label,
			Window:   o.Start.String() + "–" + o.End.String(),
			Interval: o.Interval,
			InWindow: shift.InWindow(now, o.Start, o.End),
			Enabled:  on || !ok,
		})
	}
	return views
}

// shiftTitle renders a shift label as an emoji and a human name.
func shiftTitle(label string) string {
	switch label {
	case shift.NightLabel:
		return "🌙 Night"
	case shift.DayLabel:
		return "☀️ Day"
	}
	return label
}

// formatInterval renders a reconcile interval without the zero parts
// time.Duration.String leaves behind ("1m0s" → "1m").
func formatInterval(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

func formatShifts(views []shiftView) string {
	if len(views) == 0 {
		return "🕒 <b>Shifts</b>\n\nNo shifts configured."
	}

	var sb strings.Builder
	sb.WriteString("🕒 <b>Shifts</b>")

	for _, v := range views {
		window := "inactive"
		if v.InWindow {
			window = "<b>active now</b>"
		}
		state := "⛔ off"
		if v.Enabled {
			state = "✅ on"
		}
		fmt.Fprintf(&sb, "\n\n<b>%s</b> — %s (every %s)\nWindow: %s\nState: %s",
			shiftTitle(v.Label), v.Window, formatInterval(v.Interval), window, state)
	}

	return sb.String()
}

// shiftsKeyboard builds one toggle button per shift plus a refresh button,
// returning nil when there is no shift to control.
func shiftsKeyboard(views []shiftView) *telebot.ReplyMarkup {
	if len(views) == 0 {
		return nil
	}

	rm := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(views)+1)

	for _, v := range views {
		action, next := "Turn on", "1"
		if v.Enabled {
			action, next = "Turn off", "0"
		}
		text := fmt.Sprintf("%s — %s", shiftTitle(v.Label), action)
		data := shiftTogglePrefix + v.Label + ":" + next
		rows = append(rows, rm.Row(rm.Data(text, "sh_"+v.Label, data)))
	}
	rows = append(rows, rm.Row(rm.Data("🔄 Refresh", "shr", shiftsRefreshData)))

	rm.Inline(rows...)
	return rm
}

// parseShiftToggle splits "sh:<label>:<0|1>" into the shift label and the state
// requested by the button, rejecting labels that are not configured.
func parseShiftToggle(data string, configured []shift.Options) (string, bool, error) {
	rest := strings.TrimPrefix(data, shiftTogglePrefix)
	label, state, found := strings.Cut(rest, ":")
	if !found {
		return "", false, fmt.Errorf("malformed shift toggle %q", data)
	}

	if !slices.ContainsFunc(configured, func(o shift.Options) bool { return o.Label == label }) {
		return "", false, fmt.Errorf("%w: %q", errUnknownShift, label)
	}

	switch state {
	case "1":
		return label, true, nil
	case "0":
		return label, false, nil
	}
	return "", false, fmt.Errorf("malformed shift state %q", state)
}

// shiftsConfigured reports whether any shift has a window set, i.e. whether
// there is anything for /shifts to show.
func (b *Bot) shiftsConfigured() bool {
	return len(b.shifts.Configured) > 0
}

// shiftCard renders the current state of every configured shift.
func (b *Bot) shiftCard(ctx context.Context) (string, *telebot.ReplyMarkup) {
	views := shiftViews(b.shifts.Configured, time.Now(), b.shiftToggles(ctx))
	return formatShifts(views), shiftsKeyboard(views)
}

// shiftToggles reads the on/off state of every configured shift. A failed lookup
// is logged and left out of the map, which shiftViews reads as enabled.
func (b *Bot) shiftToggles(ctx context.Context) map[string]bool {
	if b.shifts.Store == nil {
		return nil
	}

	toggles := make(map[string]bool, len(b.shifts.Configured))
	for _, o := range b.shifts.Configured {
		enabled, err := b.shifts.Store.ShiftEnabled(ctx, o.Label)
		if err != nil {
			b.logger.Warn("failed to read shift state, assuming enabled", "shift", o.Label, "err", err)
			continue
		}
		toggles[o.Label] = enabled
	}
	return toggles
}

func (b *Bot) handleShifts(c telebot.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	text, kb := b.shiftCard(ctx)
	if kb == nil {
		return c.Send(text, telebot.ModeHTML)
	}
	return c.Send(text, telebot.ModeHTML, kb)
}

func (b *Bot) callbackShiftToggle(c telebot.Context, data string) error {
	label, enabled, err := parseShiftToggle(data, b.shifts.Configured)
	if err != nil {
		b.logger.Warn("invalid shift toggle callback", "data", data, "err", err)
		return c.Respond(&telebot.CallbackResponse{Text: msgInvalidData, ShowAlert: true})
	}
	if b.shifts.Store == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Shifts are not available", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.shifts.Store.SetShiftEnabled(ctx, label, enabled); err != nil {
		b.logger.Error("failed to store shift state", "shift", label, "enabled", enabled, "err", err)
		return c.Respond(&telebot.CallbackResponse{Text: errPrefix + err.Error(), ShowAlert: true})
	}
	b.logger.Info("shift switched from telegram", "shift", label, "enabled", enabled,
		"username", c.Sender().Username)

	state := "⛔ off"
	if enabled {
		state = "✅ on"
	}
	_ = c.Respond(&telebot.CallbackResponse{Text: shiftTitle(label) + ": " + state})

	return b.editShiftCard(ctx, c)
}

func (b *Bot) callbackShiftsRefresh(c telebot.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = c.Respond()
	return b.editShiftCard(ctx, c)
}

// editShiftCard redraws the card in place so the buttons match the new state.
// A refresh that found nothing changed is not an error, so Telegram's complaint
// about identical content is swallowed.
func (b *Bot) editShiftCard(ctx context.Context, c telebot.Context) error {
	text, kb := b.shiftCard(ctx)

	var err error
	if kb == nil {
		err = c.Edit(text, telebot.ModeHTML)
	} else {
		err = c.Edit(text, kb, telebot.ModeHTML)
	}

	if errors.Is(err, telebot.ErrSameMessageContent) || (err != nil && strings.Contains(err.Error(), "message is not modified")) {
		return nil
	}
	return err
}
