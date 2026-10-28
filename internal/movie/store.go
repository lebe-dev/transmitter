package movie

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

// ErrInvalidHash is returned when a torrent hash is not a 40..64 char hex string.
var ErrInvalidHash = errors.New("invalid torrent hash")

// Detection states of a torrent.
const (
	// StateMatched — the torrent is linked to a TMDB title.
	StateMatched = "matched"
	// StateUnmatched — the lookup found nothing, or the user cleared the match.
	StateUnmatched = "unmatched"
	// StateSkipped — the torrent has no video files, so it is not looked up.
	StateSkipped = "skipped"
	// StateFailed — the lookup failed (network, TMDB); it is retried later.
	StateFailed = "failed"
)

const schema = `
CREATE TABLE IF NOT EXISTS torrent_movies (
	hash           TEXT    NOT NULL PRIMARY KEY,
	state          TEXT    NOT NULL DEFAULT '',
	manual         INTEGER NOT NULL DEFAULT 0,
	confident      INTEGER NOT NULL DEFAULT 0,
	media_type     TEXT    NOT NULL DEFAULT '',
	tmdb_id        INTEGER NOT NULL DEFAULT 0,
	imdb_id        TEXT    NOT NULL DEFAULT '',
	title          TEXT    NOT NULL DEFAULT '',
	original_title TEXT    NOT NULL DEFAULT '',
	english_title  TEXT    NOT NULL DEFAULT '',
	year           INTEGER NOT NULL DEFAULT 0,
	season         INTEGER NOT NULL DEFAULT 0,
	overview       TEXT    NOT NULL DEFAULT '',
	poster_path    TEXT    NOT NULL DEFAULT '',
	rating         REAL    NOT NULL DEFAULT 0,
	query          TEXT    NOT NULL DEFAULT '',
	renamed_to     TEXT    NOT NULL DEFAULT '',
	watched        INTEGER NOT NULL DEFAULT 0,
	missing_subs   INTEGER NOT NULL DEFAULT 0,
	attempts       INTEGER NOT NULL DEFAULT 0,
	checked_at     INTEGER NOT NULL DEFAULT 0,
	updated_at     INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE IF NOT EXISTS movie_plugin_meta (
	key   TEXT NOT NULL PRIMARY KEY,
	value TEXT NOT NULL
) STRICT;
`

// migrations add the columns that came after the first release of the
// plugin; a fresh database already has them from schema.
var migrations = []struct{ column, ddl string }{
	{column: "english_title", ddl: `ALTER TABLE torrent_movies ADD COLUMN english_title TEXT NOT NULL DEFAULT ''`},
}

// sinceKey holds the moment the plugin first ran. Torrents added after it are
// "new" and may be renamed automatically; older ones only on request.
const sinceKey = "since"

// Record is everything the plugin knows about one torrent.
type Record struct {
	Hash          string  `json:"-"`
	State         string  `json:"state"`
	Manual        bool    `json:"manual"`
	Confident     bool    `json:"confident"`
	MediaType     string  `json:"mediaType,omitempty"`
	TMDBID        int64   `json:"tmdbId,omitempty"`
	IMDbID        string  `json:"imdbId,omitempty"`
	Title         string  `json:"title,omitempty"`
	OriginalTitle string  `json:"originalTitle,omitempty"`
	EnglishTitle  string  `json:"englishTitle,omitempty"`
	Year          int     `json:"year,omitempty"`
	Season        int     `json:"season,omitempty"`
	Overview      string  `json:"overview,omitempty"`
	PosterPath    string  `json:"-"`
	HasPoster     bool    `json:"hasPoster"`
	Rating        float64 `json:"rating,omitempty"`
	Query         string  `json:"query,omitempty"`
	RenamedTo     string  `json:"renamedTo,omitempty"`
	Watched       bool    `json:"watched"`
	MissingSubs   bool    `json:"missingSubs"`
	Attempts      int     `json:"-"`
	CheckedAt     int64   `json:"-"`
}

// Match returns the matched title of the record.
func (r Record) Match() Match {
	return Match{
		MediaType:     r.MediaType,
		TMDBID:        r.TMDBID,
		IMDbID:        r.IMDbID,
		Title:         r.Title,
		OriginalTitle: r.OriginalTitle,
		EnglishTitle:  r.EnglishTitle,
		Year:          r.Year,
		Season:        r.Season,
		Overview:      r.Overview,
		PosterPath:    r.PosterPath,
		Rating:        r.Rating,
	}
}

// Store persists plugin data in SQLite, in the same file as the other stores.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// OpenStore opens (creating it if needed) the database at path and applies the schema.
func OpenStore(path string) (*Store, error) {
	db, err := sqlitedb.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("apply movie schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, err
	}
	return &Store{db: db, now: time.Now}, nil
}

// migrate adds the columns a database created by an older version lacks.
func migrate(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info('torrent_movies')`)
	if err != nil {
		return fmt.Errorf("read movie columns: %w", err)
	}
	existing := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close() //nolint:errcheck
			return fmt.Errorf("read movie columns: %w", err)
		}
		existing[name] = true
	}
	rows.Close() //nolint:errcheck
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read movie columns: %w", err)
	}

	for _, m := range migrations {
		if existing[m.column] {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return fmt.Errorf("add movie column %s: %w", m.column, err)
		}
	}
	return nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

const recordColumns = `hash, state, manual, confident, media_type, tmdb_id, imdb_id, title,
	original_title, english_title, year, season, overview, poster_path, rating, query, renamed_to, watched,
	missing_subs, attempts, checked_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanRecord(row scanner) (Record, error) {
	var r Record
	err := row.Scan(&r.Hash, &r.State, &r.Manual, &r.Confident, &r.MediaType, &r.TMDBID, &r.IMDbID,
		&r.Title, &r.OriginalTitle, &r.EnglishTitle, &r.Year, &r.Season, &r.Overview, &r.PosterPath, &r.Rating, &r.Query,
		&r.RenamedTo, &r.Watched, &r.MissingSubs, &r.Attempts, &r.CheckedAt)
	r.HasPoster = r.PosterPath != ""
	return r, err
}

// Get returns the record of a torrent; ok is false when there is none.
func (s *Store) Get(ctx context.Context, hash string) (rec Record, ok bool, err error) {
	key, err := normalizeHash(hash)
	if err != nil {
		return Record{}, false, err
	}
	rec, err = scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM torrent_movies WHERE hash = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("get movie: %w", err)
	}
	return rec, true, nil
}

// All returns every record keyed by torrent hash.
func (s *Store) All(ctx context.Context) (map[string]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordColumns+` FROM torrent_movies`)
	if err != nil {
		return nil, fmt.Errorf("list movies: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]Record)
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan movie: %w", err)
		}
		result[rec.Hash] = rec
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate movies: %w", err)
	}
	return result, nil
}

// SaveMatch links a torrent to a title. The marks (watched, missing subtitles)
// survive; a previous rename is forgotten, since it was made for another title.
func (s *Store) SaveMatch(ctx context.Context, hash string, m Match, manual, confident bool, query string) error {
	key, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	now := s.now().Unix()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO torrent_movies (hash, state, manual, confident, media_type, tmdb_id, imdb_id, title,
			original_title, english_title, year, season, overview, poster_path, rating, query, renamed_to, attempts, checked_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, ?, ?)
		ON CONFLICT(hash) DO UPDATE SET
			state = excluded.state, manual = excluded.manual, confident = excluded.confident,
			media_type = excluded.media_type, tmdb_id = excluded.tmdb_id, imdb_id = excluded.imdb_id,
			title = excluded.title, original_title = excluded.original_title,
			english_title = excluded.english_title, year = excluded.year,
			season = excluded.season, overview = excluded.overview, poster_path = excluded.poster_path, rating = excluded.rating,
			query = excluded.query,
			renamed_to = CASE WHEN torrent_movies.tmdb_id = excluded.tmdb_id
				AND torrent_movies.media_type = excluded.media_type
				AND torrent_movies.season = excluded.season THEN torrent_movies.renamed_to ELSE '' END,
			attempts = 0, checked_at = excluded.checked_at, updated_at = excluded.updated_at`,
		key, StateMatched, manual, confident, m.MediaType, m.TMDBID, m.IMDbID, m.Title,
		m.OriginalTitle, m.EnglishTitle, m.Year, m.Season, m.Overview, m.PosterPath, m.Rating, query, now, now)
	if err != nil {
		return fmt.Errorf("save movie match: %w", err)
	}
	return nil
}

// SaveState records a lookup without a match: unmatched, skipped or failed.
// The title fields are cleared; failed lookups count their attempts.
func (s *Store) SaveState(ctx context.Context, hash, state, query string, manual bool) error {
	key, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	attempts := 0
	if state == StateFailed {
		attempts = 1
	}
	now := s.now().Unix()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO torrent_movies (hash, state, manual, query, attempts, checked_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hash) DO UPDATE SET
			state = excluded.state, manual = excluded.manual, confident = 0, media_type = '', tmdb_id = 0,
			imdb_id = '', title = '', original_title = '', english_title = '', year = 0, season = 0, overview = '', poster_path = '',
			rating = 0, query = excluded.query, renamed_to = '',
			attempts = CASE WHEN excluded.state = 'failed' THEN torrent_movies.attempts + 1 ELSE 0 END,
			checked_at = excluded.checked_at, updated_at = excluded.updated_at`,
		key, state, manual, query, attempts, now, now)
	if err != nil {
		return fmt.Errorf("save movie state: %w", err)
	}
	return nil
}

// SetRenamed remembers the name the torrent was renamed to.
func (s *Store) SetRenamed(ctx context.Context, hash, name string) error {
	key, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE torrent_movies SET renamed_to = ?, updated_at = ? WHERE hash = ?`,
		name, s.now().Unix(), key)
	if err != nil {
		return fmt.Errorf("save rename: %w", err)
	}
	return nil
}

// SetEnglishTitle stores the English title of a match made before the plugin
// knew about English titles.
func (s *Store) SetEnglishTitle(ctx context.Context, hash, title string) error {
	key, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE torrent_movies SET english_title = ?, updated_at = ? WHERE hash = ?`,
		title, s.now().Unix(), key)
	if err != nil {
		return fmt.Errorf("save english title: %w", err)
	}
	return nil
}

// Flags are the user's marks on a torrent; nil leaves a mark unchanged.
type Flags struct {
	Watched     *bool `json:"watched"`
	MissingSubs *bool `json:"missingSubs"`
}

// SetFlags updates the marks of a torrent, creating its record if needed. A
// torrent the plugin has not looked at yet keeps an empty state, so detection
// still runs for it.
func (s *Store) SetFlags(ctx context.Context, hash string, f Flags) error {
	key, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set movie flags: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := s.now().Unix()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO torrent_movies (hash, updated_at) VALUES (?, ?) ON CONFLICT(hash) DO NOTHING`, key, now); err != nil {
		return fmt.Errorf("set movie flags: %w", err)
	}
	if f.Watched != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE torrent_movies SET watched = ?, updated_at = ? WHERE hash = ?`,
			*f.Watched, now, key); err != nil {
			return fmt.Errorf("set watched: %w", err)
		}
	}
	if f.MissingSubs != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE torrent_movies SET missing_subs = ?, updated_at = ? WHERE hash = ?`,
			*f.MissingSubs, now, key); err != nil {
			return fmt.Errorf("set missing subs: %w", err)
		}
	}
	return tx.Commit()
}

// DeleteExcept removes the records of torrents not in keep and reports how
// many were deleted. An empty keep list wipes all records.
func (s *Store) DeleteExcept(ctx context.Context, keep []string) (int64, error) {
	args := make([]any, 0, len(keep))
	for _, hash := range keep {
		key, err := normalizeHash(hash)
		if err != nil {
			return 0, err
		}
		args = append(args, key)
	}

	query := `DELETE FROM torrent_movies`
	if len(args) > 0 {
		query += ` WHERE hash NOT IN (?` + strings.Repeat(",?", len(args)-1) + `)`
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete orphaned movies: %w", err)
	}
	return res.RowsAffected()
}

// Since returns when the plugin first ran, recording the current time on the
// first call.
func (s *Store) Since(ctx context.Context) (time.Time, error) {
	now := strconv.FormatInt(s.now().Unix(), 10)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO movie_plugin_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`, sinceKey, now); err != nil {
		return time.Time{}, fmt.Errorf("init movie plugin start: %w", err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM movie_plugin_meta WHERE key = ?`, sinceKey).Scan(&raw); err != nil {
		return time.Time{}, fmt.Errorf("read movie plugin start: %w", err)
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse movie plugin start %q: %w", raw, err)
	}
	return time.Unix(sec, 0), nil
}

// normalizeHash validates a Transmission hashString and lowercases it, the
// same way the notes store keys its rows.
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
