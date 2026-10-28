// Package notes stores free-form user notes for torrents in a local SQLite database.
//
// A note is keyed by the torrent hash (Transmission's hashString), not by the
// numeric torrent ID: IDs are assigned per Transmission session and would break
// the association after a restart, while the hash is stable for the lifetime of
// the torrent.
package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lebe-dev/transmitter/internal/sqlitedb"
)

// DefaultMaxLength is used when TORRENT_NOTE_MAX_LENGTH is not configured.
const DefaultMaxLength = 200

var (
	// ErrTooLong is returned when a note exceeds the configured length limit.
	ErrTooLong = errors.New("note exceeds maximum length")
	// ErrInvalidHash is returned when a torrent hash is not a 40..64 char hex string.
	ErrInvalidHash = errors.New("invalid torrent hash")
)

const schema = `
CREATE TABLE IF NOT EXISTS torrent_notes (
	hash       TEXT    NOT NULL PRIMARY KEY,
	text       TEXT    NOT NULL,
	updated_at INTEGER NOT NULL
) STRICT;
`

// Store persists torrent notes in SQLite.
type Store struct {
	db     *sql.DB
	maxLen int
	now    func() time.Time
}

// Open opens (creating it if needed) the SQLite database at path and applies the schema.
// Notes longer than maxLen runes are rejected by Set.
func Open(path string, maxLen int) (*Store, error) {
	if maxLen <= 0 {
		maxLen = DefaultMaxLength
	}

	db, err := sqlitedb.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Store{db: db, maxLen: maxLen, now: time.Now}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// MaxLength returns the configured note length limit in runes.
func (s *Store) MaxLength() int {
	return s.maxLen
}

// Get returns the note for a torrent hash, or an empty string when there is none.
func (s *Store) Get(ctx context.Context, hash string) (string, error) {
	key, err := normalizeHash(hash)
	if err != nil {
		return "", err
	}

	var text string
	err = s.db.QueryRowContext(ctx, `SELECT text FROM torrent_notes WHERE hash = ?`, key).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get note: %w", err)
	}
	return text, nil
}

// All returns every stored note keyed by torrent hash.
func (s *Store) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hash, text FROM torrent_notes`)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]string)
	for rows.Next() {
		var hash, text string
		if err := rows.Scan(&hash, &text); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		result[hash] = text
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notes: %w", err)
	}
	return result, nil
}

// Set stores a note for a torrent hash. An empty (or whitespace-only) text
// removes the note. Returns ErrTooLong when the text exceeds the limit.
func (s *Store) Set(ctx context.Context, hash, text string) error {
	key, err := normalizeHash(hash)
	if err != nil {
		return err
	}

	clean := sanitize(text)
	if clean == "" {
		return s.Delete(ctx, key)
	}
	if utf8.RuneCountInString(clean) > s.maxLen {
		return ErrTooLong
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO torrent_notes (hash, text, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(hash) DO UPDATE SET text = excluded.text, updated_at = excluded.updated_at`,
		key, clean, s.now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("set note: %w", err)
	}
	return nil
}

// Delete removes the notes of the given torrent hashes. Unknown hashes are ignored.
func (s *Store) Delete(ctx context.Context, hashes ...string) error {
	keys := make([]any, 0, len(hashes))
	for _, hash := range hashes {
		key, err := normalizeHash(hash)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}

	query := `DELETE FROM torrent_notes WHERE hash IN (?` + strings.Repeat(",?", len(keys)-1) + `)`
	if _, err := s.db.ExecContext(ctx, query, keys...); err != nil {
		return fmt.Errorf("delete notes: %w", err)
	}
	return nil
}

// DeleteExcept removes every note whose hash is not in keep and reports how many
// rows were deleted. It is used to drop notes of torrents that no longer exist.
// An empty keep list wipes all notes.
func (s *Store) DeleteExcept(ctx context.Context, keep []string) (int64, error) {
	args := make([]any, 0, len(keep))
	for _, hash := range keep {
		key, err := normalizeHash(hash)
		if err != nil {
			// A hash we cannot parse must not cause the notes of valid torrents
			// to be deleted, so bail out instead of narrowing the keep set.
			return 0, err
		}
		args = append(args, key)
	}

	query := `DELETE FROM torrent_notes`
	if len(args) > 0 {
		query += ` WHERE hash NOT IN (?` + strings.Repeat(",?", len(args)-1) + `)`
	}

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete orphaned notes: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted notes: %w", err)
	}
	return deleted, nil
}

// normalizeHash validates a Transmission hashString and lowercases it.
// v1 torrents use a 40-char SHA-1 hash, v2 torrents up to 64 hex chars.
func normalizeHash(hash string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(hash))
	if len(key) < 40 || len(key) > 64 {
		return "", ErrInvalidHash
	}
	for _, r := range key {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isHex {
			return "", ErrInvalidHash
		}
	}
	return key, nil
}

// sanitize trims surrounding whitespace and strips control characters other
// than newline and tab, so a note can never break the UI or Telegram markup.
func sanitize(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, normalized)
	return strings.TrimSpace(cleaned)
}
