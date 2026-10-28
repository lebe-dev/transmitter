package movie

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lebe-dev/transmitter/internal/impact"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

const (
	// DefaultInterval is how often new torrents are looked for.
	DefaultInterval = time.Minute
	// DefaultBatch caps lookups per round, so a large library is backfilled
	// gradually instead of hammering TMDB at startup.
	DefaultBatch = 10
	// maxRetryDelay caps the backoff of failed lookups.
	maxRetryDelay = 6 * time.Hour
	// candidatesPerSearch is how many TMDB hits are considered for the best match.
	candidatesPerSearch = 5
)

var (
	// ErrAutoRemoveUnavailable is returned when the auto-remove switch has no store behind it.
	ErrAutoRemoveUnavailable = errors.New("auto-remove is not available")
	// ErrNotMatched is returned when an action needs a matched title and there is none.
	ErrNotMatched = errors.New("torrent is not matched to a title")
	// ErrEmptyName is returned when the rename template yields an empty name.
	ErrEmptyName = errors.New("rename template produced an empty name")
	// ErrNoPoster is returned when the matched title has no poster.
	ErrNoPoster = errors.New("no poster")
)

// posterFileRe matches TMDB poster file names; anything else is never written to disk.
var posterFileRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}\.(?:jpg|jpeg|png|webp)$`)

// torrentClient is satisfied by *transmission.Client and allows test injection.
type torrentClient interface {
	GetTorrents(ctx context.Context) ([]transmission.Torrent, error)
	GetTorrentFilesByHash(ctx context.Context, hash string) (transmission.TorrentWithFiles, error)
	RenamePath(ctx context.Context, id int64, path, name string) error
	RemoveTorrents(ctx context.Context, ids []int64, deleteLocalData bool) error
}

// AutoRemovePrefs holds the user's switch for removing watched torrents that
// gave back everything they took; internal/prefs implements it.
type AutoRemovePrefs interface {
	MovieAutoRemoveEnabled(ctx context.Context) (bool, error)
	SetMovieAutoRemoveEnabled(ctx context.Context, enabled bool) error
}

// Source is a movie database; *TMDB implements it.
type Source interface {
	Search(ctx context.Context, mediaType, query string, year int) ([]Match, error)
	Details(ctx context.Context, mediaType string, id int64) (Match, error)
	FindIMDb(ctx context.Context, imdbID string) (Match, error)
	Poster(ctx context.Context, posterPath string) ([]byte, error)
}

// Options configure the background worker.
type Options struct {
	Interval time.Duration
	Batch    int
	// AutoDetect looks up torrents added after the plugin was enabled.
	AutoDetect bool
	// Backfill looks up torrents that were there before.
	Backfill bool
	// AutoRename renames new torrents right after a confident match. Torrents
	// that were there before the plugin are never renamed automatically.
	AutoRename bool
	Template   string
	PosterDir  string
	// AutoRemove is the switch for removing watched torrents whose upload
	// ratio reached TargetRatio; nil keeps the rule off.
	AutoRemove  AutoRemovePrefs
	TargetRatio float64
	// DeleteData removes the downloaded files together with the torrent.
	DeleteData bool
}

// Service detects titles, keeps the marks and renames torrents. One worker
// runs in the background; the HTTP handlers call the same methods.
type Service struct {
	store  *Store
	source Source
	client torrentClient
	opts   Options
	logger *slog.Logger
	now    func() time.Time

	// mu serializes lookups and renames, so the worker and a click in the UI
	// never rename the same torrent twice.
	mu   sync.Mutex
	wake chan struct{}
}

// NewService creates the plugin service.
func NewService(store *Store, source Source, client torrentClient, opts Options, logger *slog.Logger) *Service {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Batch <= 0 {
		opts.Batch = DefaultBatch
	}
	if opts.Template == "" {
		opts.Template = DefaultRenameTemplate
	}
	return &Service{
		store:  store,
		source: source,
		client: client,
		opts:   opts,
		logger: logger,
		now:    time.Now,
		wake:   make(chan struct{}, 1),
	}
}

// Wake asks the worker to look for new torrents now instead of on the next tick.
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run blocks until ctx is cancelled, scanning once on start and then on each
// tick or wake-up.
func (s *Service) Run(ctx context.Context) {
	s.logger.Info("movie plugin started", "interval", s.opts.Interval, "batch", s.opts.Batch,
		"auto_detect", s.opts.AutoDetect, "backfill", s.opts.Backfill, "auto_rename", s.opts.AutoRename)

	s.scan(ctx)

	ticker := time.NewTicker(s.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("movie plugin stopped")
			return
		case <-ticker.C:
		case <-s.wake:
		}
		s.scan(ctx)
	}
}

// scan drops the records of removed torrents and looks up a batch of torrents
// the plugin has not seen yet.
func (s *Service) scan(ctx context.Context) {
	torrents, err := s.client.GetTorrents(ctx)
	if err != nil {
		s.logger.Warn("movie plugin: failed to fetch torrents", "err", err)
		return
	}
	records, err := s.store.All(ctx)
	if err != nil {
		s.logger.Warn("movie plugin: failed to load records", "err", err)
		return
	}
	since, err := s.store.Since(ctx)
	if err != nil {
		s.logger.Warn("movie plugin: failed to read start time", "err", err)
		return
	}

	s.removeWatched(ctx, torrents, records)
	s.dropOrphans(ctx, torrents, records)

	type candidate struct {
		hash  string
		added int64
		isNew bool
	}
	var candidates []candidate
	for _, t := range torrents {
		hash := strings.ToLower(t.HashString)
		if hash == "" {
			continue
		}
		if rec, ok := records[hash]; ok && !s.due(rec) {
			continue
		}
		isNew := t.AddedDate >= since.Unix()
		if (isNew && !s.opts.AutoDetect) || (!isNew && !s.opts.Backfill) {
			continue
		}
		candidates = append(candidates, candidate{hash: hash, added: t.AddedDate, isNew: isNew})
	}
	// Fresh torrents first: the user is most likely looking at them.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].added > candidates[j].added })
	if len(candidates) > s.opts.Batch {
		candidates = candidates[:s.opts.Batch]
	}

	for _, c := range candidates {
		if ctx.Err() != nil {
			return
		}
		s.autoDetect(ctx, c.hash, c.isNew)
	}
}

// removeWatched removes the torrents the user marked as watched once their
// upload ratio reached the target, while the user has the rule switched on.
// Records of removed torrents are dropped on the next round, like those of
// torrents removed by hand.
func (s *Service) removeWatched(ctx context.Context, torrents []transmission.Torrent, records map[string]Record) {
	if !s.AutoRemoveEnabled(ctx) {
		return
	}
	var ids []int64
	var names []string
	for _, t := range torrents {
		rec, ok := records[strings.ToLower(t.HashString)]
		if !ok || !rec.Watched || t.PercentDone < 1 || !impact.Reached(t, s.opts.TargetRatio) {
			continue
		}
		ids = append(ids, t.ID)
		names = append(names, t.Name)
	}
	if len(ids) == 0 {
		return
	}
	if err := s.client.RemoveTorrents(ctx, ids, s.opts.DeleteData); err != nil {
		s.logger.Warn("movie plugin: failed to remove watched torrents", "ids", ids, "err", err)
		return
	}
	s.logger.Info("movie plugin: removed watched torrents that reached the target ratio",
		"target_ratio", s.opts.TargetRatio, "delete_data", s.opts.DeleteData, "count", len(ids), "names", names)
}

// AutoRemoveEnabled reports whether watched torrents that reached the target
// ratio are removed. A failed lookup reports the rule as off: keeping a torrent
// is the harmless outcome.
func (s *Service) AutoRemoveEnabled(ctx context.Context) bool {
	if s.opts.AutoRemove == nil {
		return false
	}
	enabled, err := s.opts.AutoRemove.MovieAutoRemoveEnabled(ctx)
	if err != nil {
		s.logger.Warn("movie plugin: failed to read auto-remove state, assuming off", "err", err)
		return false
	}
	return enabled
}

// SetAutoRemoveEnabled switches the removal of watched torrents on or off.
func (s *Service) SetAutoRemoveEnabled(ctx context.Context, enabled bool) error {
	if s.opts.AutoRemove == nil {
		return ErrAutoRemoveUnavailable
	}
	return s.opts.AutoRemove.SetMovieAutoRemoveEnabled(ctx, enabled)
}

// due reports whether a torrent with a record should be looked up again:
// it has only marks so far, or its last lookup failed and the backoff passed.
func (s *Service) due(rec Record) bool {
	switch rec.State {
	case "":
		return true
	case StateFailed:
		delay := time.Duration(float64(s.opts.Interval) * math.Pow(2, float64(rec.Attempts)))
		if delay > maxRetryDelay || delay <= 0 {
			delay = maxRetryDelay
		}
		return s.now().Sub(time.Unix(rec.CheckedAt, 0)) >= delay
	}
	return false
}

// dropOrphans removes the records and cached posters of torrents that no
// longer exist in Transmission.
func (s *Service) dropOrphans(ctx context.Context, torrents []transmission.Torrent, records map[string]Record) {
	keep := make([]string, 0, len(torrents))
	present := make(map[string]bool, len(torrents))
	for _, t := range torrents {
		if t.HashString != "" {
			keep = append(keep, t.HashString)
			present[strings.ToLower(t.HashString)] = true
		}
	}

	deleted, err := s.store.DeleteExcept(ctx, keep)
	if err != nil {
		s.logger.Warn("movie plugin: failed to delete orphaned records", "err", err)
		return
	}
	if deleted == 0 {
		return
	}
	s.logger.Info("movie plugin: removed records of deleted torrents", "count", deleted)

	posters := map[string]bool{}
	for hash, rec := range records {
		if present[hash] && rec.PosterPath != "" {
			posters[filepath.Base(rec.PosterPath)] = true
		}
	}
	s.prunePosters(posters)
}

// prunePosters deletes cached posters that no record refers to.
func (s *Service) prunePosters(keep map[string]bool) {
	if s.opts.PosterDir == "" {
		return
	}
	entries, err := os.ReadDir(s.opts.PosterDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || keep[e.Name()] || !posterFileRe.MatchString(e.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(s.opts.PosterDir, e.Name())); err != nil {
			s.logger.Debug("movie plugin: failed to remove poster", "file", e.Name(), "err", err)
		}
	}
}

// autoDetect looks up one torrent in the background and renames it when that
// is allowed.
func (s *Service) autoDetect(ctx context.Context, hash string, isNew bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.client.GetTorrentFilesByHash(ctx, hash)
	if err != nil {
		s.logger.Warn("movie plugin: failed to fetch torrent files", "hash", hash, "err", err)
		return
	}
	if len(t.Files) == 0 {
		// A magnet link without metadata yet; the next round will see the files.
		return
	}
	if !hasVideo(t.Files) {
		if err := s.store.SaveState(ctx, hash, StateSkipped, "", false); err != nil {
			s.logger.Warn("movie plugin: failed to save state", "hash", hash, "err", err)
		}
		return
	}

	rec, err := s.detectLocked(ctx, hash, t.Name)
	if err != nil {
		s.logger.Warn("movie plugin: lookup failed", "torrent", t.Name, "err", err)
		return
	}
	if rec.State != StateMatched {
		s.logger.Info("movie plugin: no match", "torrent", t.Name, "query", rec.Query)
		return
	}
	s.logger.Info("movie plugin: matched", "torrent", t.Name, "title", rec.Title, "year", rec.Year,
		"type", rec.MediaType, "confident", rec.Confident)

	if !s.opts.AutoRename || !isNew || !rec.Confident || rec.RenamedTo != "" {
		return
	}
	name, err := s.renameLocked(ctx, t, rec)
	if err != nil {
		s.logger.Warn("movie plugin: auto-rename failed", "torrent", t.Name, "err", err)
		return
	}
	s.logger.Info("movie plugin: renamed", "from", t.Name, "to", name)
}

func hasVideo(files []transmission.TorrentFile) bool {
	for _, f := range files {
		if IsVideoFile(f.Name) {
			return true
		}
	}
	return false
}

// Get returns the record of a torrent; ok is false when there is none.
func (s *Service) Get(ctx context.Context, hash string) (Record, bool, error) {
	return s.store.Get(ctx, hash)
}

// All returns every record keyed by torrent hash.
func (s *Service) All(ctx context.Context) (map[string]Record, error) {
	return s.store.All(ctx)
}

// Detect looks the torrent up by its name again, replacing a previous match.
func (s *Service) Detect(ctx context.Context, hash string) (Record, error) {
	if _, err := normalizeHash(hash); err != nil {
		return Record{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.client.GetTorrentFilesByHash(ctx, hash)
	if err != nil {
		return Record{}, err
	}
	return s.detectLocked(ctx, hash, t.Name)
}

// detectLocked searches the source for the torrent name and stores the result.
// A lookup that found nothing is not an error: the record says unmatched.
func (s *Service) detectLocked(ctx context.Context, hash, name string) (Record, error) {
	rel := ParseRelease(name)
	query := strings.Join(rel.Titles, " / ")
	if rel.Year > 0 {
		query += " (" + strconv.Itoa(rel.Year) + ")"
	}

	m, confident, err := s.lookup(ctx, rel)
	switch {
	case errors.Is(err, ErrNotFound):
		err = s.store.SaveState(ctx, hash, StateUnmatched, query, false)
	case err != nil:
		if saveErr := s.store.SaveState(ctx, hash, StateFailed, query, false); saveErr != nil {
			s.logger.Warn("movie plugin: failed to save state", "hash", hash, "err", saveErr)
		}
		return Record{}, err
	default:
		err = s.store.SaveMatch(ctx, hash, m, false, confident, query)
	}
	if err != nil {
		return Record{}, err
	}

	rec, _, err := s.store.Get(ctx, hash)
	return rec, err
}

// lookup finds the best title for a parsed release. Series markers make it
// search series first; otherwise movies come first, then series (miniseries
// often have no marker).
func (s *Service) lookup(ctx context.Context, rel Release) (Match, bool, error) {
	if len(rel.Titles) == 0 {
		return Match{}, false, ErrNotFound
	}
	order := []string{MediaMovie, MediaTV}
	if rel.TV {
		order = []string{MediaTV, MediaMovie}
	}

	for _, mediaType := range order {
		for _, title := range rel.Titles {
			results, err := s.source.Search(ctx, mediaType, title, rel.Year)
			if err != nil {
				return Match{}, false, err
			}
			if len(results) == 0 && rel.Year > 0 {
				// The year may be off: a re-release, or the season of a series.
				if results, err = s.source.Search(ctx, mediaType, title, 0); err != nil {
					return Match{}, false, err
				}
			}
			if len(results) == 0 {
				continue
			}

			best, confident := choose(results, rel)
			// Search hits lack the IMDb ID; the details have it. A failure here
			// is not worth losing the match over.
			if full, err := s.source.Details(ctx, best.MediaType, best.TMDBID); err == nil {
				best = full
			} else {
				s.logger.Debug("movie plugin: failed to fetch details", "tmdb_id", best.TMDBID, "err", err)
			}
			if best.MediaType == MediaTV {
				best.Season = rel.Season
			}
			return best, confident, nil
		}
	}
	return Match{}, false, ErrNotFound
}

// choose picks the hit whose title matches one of the release titles, with
// the year (for movies) as tie-breaker. The match is confident when the title
// matches exactly and, for movies, the year is known and within a year of the
// release — only then may the torrent be renamed without asking.
func choose(results []Match, rel Release) (Match, bool) {
	if len(results) > candidatesPerSearch {
		results = results[:candidatesPerSearch]
	}
	wanted := map[string]bool{}
	for _, t := range rel.Titles {
		wanted[normalizeForCompare(t)] = true
	}
	titleMatches := func(m Match) bool {
		return wanted[normalizeForCompare(m.Title)] || wanted[normalizeForCompare(m.OriginalTitle)]
	}
	yearMatches := func(m Match) bool {
		return rel.Year > 0 && m.Year > 0 && abs(m.Year-rel.Year) <= 1
	}

	for _, m := range results {
		if titleMatches(m) && (m.MediaType == MediaTV || yearMatches(m)) {
			return m, true
		}
	}
	for _, m := range results {
		if titleMatches(m) {
			return m, false
		}
	}
	return results[0], false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Link matches a torrent to the title behind a TMDB or IMDb link.
func (s *Service) Link(ctx context.Context, hash, rawLink string) (Record, error) {
	if _, err := normalizeHash(hash); err != nil {
		return Record{}, err
	}
	link, err := ParseLink(rawLink)
	if err != nil {
		return Record{}, err
	}

	var m Match
	if link.TMDBID > 0 {
		m, err = s.source.Details(ctx, link.MediaType, link.TMDBID)
	} else {
		m, err = s.source.FindIMDb(ctx, link.IMDbID)
	}
	if err != nil {
		return Record{}, err
	}
	if m.MediaType == MediaTV {
		// The link names the series; the season still comes from the torrent.
		if t, err := s.client.GetTorrentFilesByHash(ctx, hash); err == nil {
			m.Season = ParseRelease(t.Name).Season
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.SaveMatch(ctx, hash, m, true, true, strings.TrimSpace(rawLink)); err != nil {
		return Record{}, err
	}
	rec, _, err := s.store.Get(ctx, hash)
	return rec, err
}

// Clear forgets the match of a torrent. The torrent is not looked up again
// automatically; the marks are kept.
func (s *Service) Clear(ctx context.Context, hash string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.SaveState(ctx, hash, StateUnmatched, "", true); err != nil {
		return Record{}, err
	}
	rec, _, err := s.store.Get(ctx, hash)
	return rec, err
}

// SetFlags updates the watched / missing subtitles marks.
func (s *Service) SetFlags(ctx context.Context, hash string, f Flags) (Record, error) {
	if err := s.store.SetFlags(ctx, hash, f); err != nil {
		return Record{}, err
	}
	rec, _, err := s.store.Get(ctx, hash)
	return rec, err
}

// Rename renames the torrent's folder (or single file) after the matched
// title and returns the new name.
func (s *Service) Rename(ctx context.Context, hash string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok, err := s.store.Get(ctx, hash)
	if err != nil {
		return "", err
	}
	if !ok || rec.State != StateMatched {
		return "", ErrNotMatched
	}
	t, err := s.client.GetTorrentFilesByHash(ctx, hash)
	if err != nil {
		return "", err
	}
	return s.renameLocked(ctx, t, s.withEnglishTitle(ctx, rec))
}

// RenamePreview returns the current name of the torrent and the name Rename
// would give it, without renaming anything.
func (s *Service) RenamePreview(ctx context.Context, hash string) (current, target string, err error) {
	rec, ok, err := s.store.Get(ctx, hash)
	if err != nil {
		return "", "", err
	}
	if !ok || rec.State != StateMatched {
		return "", "", ErrNotMatched
	}
	t, err := s.client.GetTorrentFilesByHash(ctx, hash)
	if err != nil {
		return "", "", err
	}
	return t.Name, s.targetName(t, s.withEnglishTitle(ctx, rec)), nil
}

// withEnglishTitle fills in the English title of a match made before the
// plugin stored one, so such a torrent is not renamed after its localized
// title. The title is fetched once and saved; a failure only means the
// rename falls back to the original title.
func (s *Service) withEnglishTitle(ctx context.Context, rec Record) Record {
	if rec.EnglishTitle != "" || rec.TMDBID == 0 || !strings.Contains(s.opts.Template, "{english_title}") {
		return rec
	}
	full, err := s.source.Details(ctx, rec.MediaType, rec.TMDBID)
	if err != nil {
		s.logger.Debug("movie plugin: failed to fetch the English title", "tmdb_id", rec.TMDBID, "err", err)
		return rec
	}
	if full.EnglishTitle == "" {
		return rec
	}
	if err := s.store.SetEnglishTitle(ctx, rec.Hash, full.EnglishTitle); err != nil {
		s.logger.Warn("movie plugin: failed to save the English title", "hash", rec.Hash, "err", err)
	}
	rec.EnglishTitle = full.EnglishTitle
	return rec
}

// targetName renders the new name of a torrent. A single-file torrent has one
// file at the root; renaming the "torrent" renames that file, so it keeps its
// extension.
func (s *Service) targetName(t transmission.TorrentWithFiles, rec Record) string {
	single := len(t.Files) == 1 && !strings.Contains(t.Files[0].Name, "/")
	return TargetName(RenderName(s.opts.Template, rec.Match()), t.Name, single)
}

func (s *Service) renameLocked(ctx context.Context, t transmission.TorrentWithFiles, rec Record) (string, error) {
	target := s.targetName(t, rec)
	if target == "" {
		return "", ErrEmptyName
	}
	if target != t.Name {
		if err := s.client.RenamePath(ctx, t.ID, t.Name, target); err != nil {
			return "", err
		}
	}
	if err := s.store.SetRenamed(ctx, rec.Hash, target); err != nil {
		return "", err
	}
	return target, nil
}

// PosterFile returns the path of the cached poster of a torrent's title,
// downloading it on first use.
func (s *Service) PosterFile(ctx context.Context, hash string) (string, error) {
	rec, ok, err := s.store.Get(ctx, hash)
	if err != nil {
		return "", err
	}
	if !ok || rec.State != StateMatched || rec.PosterPath == "" {
		return "", ErrNoPoster
	}
	name := filepath.Base(rec.PosterPath)
	if !posterFileRe.MatchString(name) || s.opts.PosterDir == "" {
		return "", ErrNoPoster
	}

	file := filepath.Join(s.opts.PosterDir, name)
	if _, err := os.Stat(file); err == nil {
		return file, nil
	}

	data, err := s.source.Poster(ctx, "/"+name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.opts.PosterDir, 0o750); err != nil {
		return "", fmt.Errorf("create poster dir: %w", err)
	}
	// Write to a temp file first: a concurrent request must never serve half an image.
	tmp, err := os.CreateTemp(s.opts.PosterDir, ".poster-*")
	if err != nil {
		return "", fmt.Errorf("create poster file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()           //nolint:errcheck
		os.Remove(tmp.Name()) //nolint:errcheck
		return "", fmt.Errorf("write poster: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name()) //nolint:errcheck
		return "", fmt.Errorf("write poster: %w", err)
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		os.Remove(tmp.Name()) //nolint:errcheck
		return "", fmt.Errorf("store poster: %w", err)
	}
	return file, nil
}
