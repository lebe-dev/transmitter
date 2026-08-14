// Package prefs stores user preferences that are changed at runtime from the UI.
//
// Currently it holds the on/off state of the shift schedulers: the time windows
// come from the environment, but switching a shift off is a decision made in the
// web UI, so it is persisted in the local SQLite database (the same file as the
// torrent notes) and survives a restart.
package prefs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lebe-dev/transmitter/internal/sqlitedb"
)

// ErrInvalidShift is returned when a shift name is empty.
var ErrInvalidShift = errors.New("invalid shift name")

const schema = `
CREATE TABLE IF NOT EXISTS prefs (
	key        TEXT    NOT NULL PRIMARY KEY,
	value      TEXT    NOT NULL,
	updated_at INTEGER NOT NULL
) STRICT;
`

// Store persists preferences as key/value pairs in SQLite.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (creating it if needed) the database at path and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sqlitedb.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("apply prefs schema: %w", err)
	}
	return &Store{db: db, now: time.Now}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// ShiftEnabled reports whether the named shift scheduler is switched on.
// A shift without a stored preference is enabled, which keeps a configured
// shift working as before the toggle existed.
func (s *Store) ShiftEnabled(ctx context.Context, shift string) (bool, error) {
	key, err := shiftKey(shift)
	if err != nil {
		return false, err
	}

	// A missing or unparsable value is treated as the default instead of failing
	// the request: the scheduler must keep running.
	return s.boolPref(ctx, key, true)
}

// SetShiftEnabled switches the named shift scheduler on or off.
func (s *Store) SetShiftEnabled(ctx context.Context, shift string, enabled bool) error {
	key, err := shiftKey(shift)
	if err != nil {
		return err
	}

	return s.setBoolPref(ctx, key, enabled)
}

// impactAutoStopKey holds whether torrents that reached the target upload ratio
// are stopped automatically.
const impactAutoStopKey = "impact.autostop.enabled"

// ImpactAutoStopEnabled reports whether the impact worker may stop torrents that
// reached the target upload ratio. Unlike the shifts it is off by default:
// stopping a torrent nobody asked to stop is a surprise, so the user opts in.
func (s *Store) ImpactAutoStopEnabled(ctx context.Context) (bool, error) {
	return s.boolPref(ctx, impactAutoStopKey, false)
}

// SetImpactAutoStopEnabled switches automatic stopping on or off.
func (s *Store) SetImpactAutoStopEnabled(ctx context.Context, enabled bool) error {
	return s.setBoolPref(ctx, impactAutoStopKey, enabled)
}

// boolPref reads a boolean preference, returning def when it was never stored or
// cannot be parsed.
func (s *Store) boolPref(ctx context.Context, key string, def bool) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM prefs WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return false, fmt.Errorf("get pref %q: %w", key, err)
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return def, nil
	}
	return value, nil
}

// setBoolPref stores a boolean preference.
func (s *Store) setBoolPref(ctx context.Context, key string, value bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO prefs (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, strconv.FormatBool(value), s.now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("set pref %q: %w", key, err)
	}
	return nil
}

// shiftKey builds the preference key holding the on/off state of a shift.
func shiftKey(shift string) (string, error) {
	name := strings.TrimSpace(shift)
	if name == "" {
		return "", ErrInvalidShift
	}
	return "shift." + name + ".enabled", nil
}
