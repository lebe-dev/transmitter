package movie

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hashC = "cccccccccccccccccccccccccccccccccccccccc"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "movies.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	return store
}

// fakeClient is an in-memory Transmission with renames applied to the names.
type fakeClient struct {
	torrents map[string]*transmission.TorrentWithFiles
	// stats are the transfer counters reported by GetTorrents, keyed by hash.
	stats   map[string]transmission.Torrent
	renames []string
	removed []int64
	// removedWithData remembers the deleteLocalData flag of the last removal.
	removedWithData bool
	err             error
}

func (f *fakeClient) GetTorrents(context.Context) ([]transmission.Torrent, error) {
	if f.err != nil {
		return nil, f.err
	}
	var list []transmission.Torrent
	for _, t := range f.torrents {
		st := f.stats[t.HashString]
		list = append(list, transmission.Torrent{ID: t.ID, HashString: t.HashString, Name: t.Name, AddedDate: t.AddedDate,
			PercentDone: st.PercentDone, UploadRatio: st.UploadRatio, DownloadedEver: st.DownloadedEver, UploadedEver: st.UploadedEver})
	}
	return list, nil
}

func (f *fakeClient) GetTorrentFilesByHash(_ context.Context, hash string) (transmission.TorrentWithFiles, error) {
	t, ok := f.torrents[hash]
	if !ok {
		return transmission.TorrentWithFiles{}, transmission.ErrTorrentNotFound
	}
	return *t, nil
}

func (f *fakeClient) RenamePath(_ context.Context, id int64, path, name string) error {
	for _, t := range f.torrents {
		if t.ID == id && t.Name == path {
			t.Name = name
			f.renames = append(f.renames, name)
			return nil
		}
	}
	return errors.New("invalid argument")
}

func (f *fakeClient) RemoveTorrents(_ context.Context, ids []int64, deleteLocalData bool) error {
	f.removed = append(f.removed, ids...)
	f.removedWithData = deleteLocalData
	for hash, t := range f.torrents {
		for _, id := range ids {
			if t.ID == id {
				delete(f.torrents, hash)
			}
		}
	}
	return nil
}

func (f *fakeClient) add(hash, name string, added int64, files ...string) {
	if f.torrents == nil {
		f.torrents = map[string]*transmission.TorrentWithFiles{}
	}
	t := &transmission.TorrentWithFiles{ID: int64(len(f.torrents) + 1), HashString: hash, Name: name, AddedDate: added}
	for _, file := range files {
		t.Files = append(t.Files, transmission.TorrentFile{Name: file})
	}
	f.torrents[hash] = t
}

// fakeSource answers searches from a fixed catalogue.
type fakeSource struct {
	catalogue []Match
	searches  int
	err       error
}

func (f *fakeSource) Search(_ context.Context, mediaType, query string, year int) ([]Match, error) {
	f.searches++
	if f.err != nil {
		return nil, f.err
	}
	var out []Match
	for _, m := range f.catalogue {
		if m.MediaType != mediaType {
			continue
		}
		if normalizeForCompare(m.Title) != normalizeForCompare(query) && normalizeForCompare(m.OriginalTitle) != normalizeForCompare(query) {
			continue
		}
		if year > 0 && m.Year != year {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeSource) Details(_ context.Context, mediaType string, id int64) (Match, error) {
	for _, m := range f.catalogue {
		if m.MediaType == mediaType && m.TMDBID == id {
			m.IMDbID = "tt" + "0000001"
			return m, nil
		}
	}
	return Match{}, ErrNotFound
}

func (f *fakeSource) FindIMDb(_ context.Context, imdbID string) (Match, error) {
	if imdbID == "tt0816692" {
		return f.Details(context.Background(), MediaMovie, 157336)
	}
	return Match{}, ErrNotFound
}

func (f *fakeSource) Poster(context.Context, string) ([]byte, error) {
	return []byte("jpeg"), nil
}

var interstellar = Match{MediaType: MediaMovie, TMDBID: 157336, Title: "Интерстеллар", OriginalTitle: "Interstellar", EnglishTitle: "Interstellar", Year: 2014, PosterPath: "/poster.jpg"}

func newService(t *testing.T, client *fakeClient, source *fakeSource, opts Options) (*Service, *Store) {
	t.Helper()
	store := newStore(t)
	if opts.PosterDir == "" {
		opts.PosterDir = filepath.Join(t.TempDir(), "posters")
	}
	return NewService(store, source, client, opts, discardLogger()), store
}

func TestScanDetectsAndRenamesNewTorrents(t *testing.T) {
	client := &fakeClient{}
	source := &fakeSource{catalogue: []Match{interstellar}}
	svc, store := newService(t, client, source, Options{AutoDetect: true, Backfill: true, AutoRename: true})
	ctx := context.Background()

	// The plugin starts now: a torrent added later is new.
	since, _ := store.Since(ctx)
	client.add(hashA, "Interstellar.2014.1080p.BluRay.x264", since.Unix()+10, "Interstellar.2014.1080p.BluRay.x264/movie.mkv")

	svc.scan(ctx)

	rec, ok, _ := store.Get(ctx, hashA)
	if !ok || rec.State != StateMatched || rec.TMDBID != 157336 || !rec.Confident {
		t.Fatalf("record = %+v, want a confident match", rec)
	}
	if rec.IMDbID == "" {
		t.Error("IMDb ID must come from the details")
	}
	if got := client.torrents[hashA].Name; got != "Interstellar_(2014)" {
		t.Errorf("torrent name = %q, want it renamed after the English title", got)
	}
	if rec.RenamedTo != "Interstellar_(2014)" {
		t.Errorf("RenamedTo = %q", rec.RenamedTo)
	}

	// A second round neither searches nor renames again.
	searches := source.searches
	svc.scan(ctx)
	if source.searches != searches || len(client.renames) != 1 {
		t.Errorf("second scan searched %d times and renamed %d times", source.searches-searches, len(client.renames))
	}
}

func TestScanNeverAutoRenamesOldTorrents(t *testing.T) {
	client := &fakeClient{}
	svc, store := newService(t, client, &fakeSource{catalogue: []Match{interstellar}}, Options{AutoDetect: true, Backfill: true, AutoRename: true})
	ctx := context.Background()

	since, _ := store.Since(ctx)
	client.add(hashA, "Interstellar (2014) [1080p]", since.Unix()-3600, "Interstellar (2014) [1080p].mkv")

	svc.scan(ctx)

	rec, _, _ := store.Get(ctx, hashA)
	if rec.State != StateMatched {
		t.Fatalf("state = %q, want the old torrent backfilled", rec.State)
	}
	if len(client.renames) != 0 {
		t.Errorf("renames = %v, want none for a torrent that predates the plugin", client.renames)
	}
}

func TestScanRespectsDetectionSwitches(t *testing.T) {
	client := &fakeClient{}
	source := &fakeSource{catalogue: []Match{interstellar}}
	svc, store := newService(t, client, source, Options{AutoDetect: false, Backfill: false})
	ctx := context.Background()

	since, _ := store.Since(ctx)
	client.add(hashA, "Interstellar 2014", since.Unix()+10, "a.mkv")
	client.add(hashB, "Interstellar 2014", since.Unix()-10, "b.mkv")

	svc.scan(ctx)

	if source.searches != 0 {
		t.Errorf("searches = %d, want none with detection switched off", source.searches)
	}
}

func TestScanSkipsNonVideoAndWaitsForMetadata(t *testing.T) {
	client := &fakeClient{}
	source := &fakeSource{}
	svc, store := newService(t, client, source, Options{AutoDetect: true, Backfill: true})
	ctx := context.Background()

	client.add(hashA, "ubuntu-24.04-desktop-amd64.iso", 1, "ubuntu-24.04-desktop-amd64.iso")
	client.add(hashB, "magnet without metadata", 2)

	svc.scan(ctx)

	if rec, _, _ := store.Get(ctx, hashA); rec.State != StateSkipped {
		t.Errorf("iso state = %q, want skipped", rec.State)
	}
	if _, ok, _ := store.Get(ctx, hashB); ok {
		t.Error("a torrent without files yet must not get a record")
	}
	if source.searches != 0 {
		t.Errorf("searches = %d, want none", source.searches)
	}
}

func TestScanRecordsFailuresAndBacksOff(t *testing.T) {
	client := &fakeClient{}
	source := &fakeSource{err: errors.New("tmdb unreachable")}
	svc, store := newService(t, client, source, Options{AutoDetect: true, Backfill: true, Interval: time.Minute})
	ctx := context.Background()
	client.add(hashA, "Interstellar 2014", 1, "a.mkv")

	svc.scan(ctx)
	rec, _, _ := store.Get(ctx, hashA)
	if rec.State != StateFailed || rec.Attempts != 1 {
		t.Fatalf("record = %+v, want one failed attempt", rec)
	}

	searches := source.searches
	svc.scan(ctx)
	if source.searches != searches {
		t.Error("a failed lookup must wait for the backoff before retrying")
	}

	// After the backoff the lookup is retried and may succeed.
	source.err = nil
	source.catalogue = []Match{interstellar}
	svc.now = func() time.Time { return time.Now().Add(time.Hour) }
	svc.scan(ctx)
	if rec, _, _ := store.Get(ctx, hashA); rec.State != StateMatched {
		t.Errorf("state = %q, want matched after the retry", rec.State)
	}
}

func TestScanDropsRecordsOfRemovedTorrents(t *testing.T) {
	client := &fakeClient{}
	svc, store := newService(t, client, &fakeSource{}, Options{})
	ctx := context.Background()
	store.SetFlags(ctx, hashC, Flags{Watched: ptr(true)}) //nolint:errcheck

	svc.scan(ctx)

	if _, ok, _ := store.Get(ctx, hashC); ok {
		t.Error("the record of a removed torrent must be dropped")
	}
}

func TestScanKeepsRecordsWhenTransmissionFails(t *testing.T) {
	client := &fakeClient{err: errors.New("connection refused")}
	svc, store := newService(t, client, &fakeSource{}, Options{})
	ctx := context.Background()
	store.SetFlags(ctx, hashC, Flags{Watched: ptr(true)}) //nolint:errcheck

	svc.scan(ctx)

	if _, ok, _ := store.Get(ctx, hashC); !ok {
		t.Error("records must survive an unreachable Transmission")
	}
}

func TestDetectPrefersSeriesForSeasonMarkers(t *testing.T) {
	client := &fakeClient{}
	lost := Match{MediaType: MediaTV, TMDBID: 4607, Title: "Остаться в живых", OriginalTitle: "Lost", Year: 2004}
	lostMovie := Match{MediaType: MediaMovie, TMDBID: 1, Title: "Lost", Year: 2004}
	svc, _ := newService(t, client, &fakeSource{catalogue: []Match{lostMovie, lost}}, Options{})
	client.add(hashA, "Lost.S01.1080p.BluRay", 1, "Lost/e01.mkv")

	rec, err := svc.Detect(context.Background(), hashA)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if rec.MediaType != MediaTV || rec.TMDBID != 4607 || rec.Season != 1 {
		t.Errorf("record = %+v, want season 1 of the series", rec)
	}
}

func TestDetectWithoutYearIsNotConfident(t *testing.T) {
	client := &fakeClient{}
	svc, _ := newService(t, client, &fakeSource{catalogue: []Match{interstellar}}, Options{})
	client.add(hashA, "Interstellar.1080p.BluRay", 1, "a.mkv")

	rec, err := svc.Detect(context.Background(), hashA)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if rec.State != StateMatched || rec.Confident {
		t.Errorf("record = %+v, want a match that is not confident", rec)
	}
}

func TestDetectUnmatched(t *testing.T) {
	client := &fakeClient{}
	svc, _ := newService(t, client, &fakeSource{}, Options{})
	client.add(hashA, "Some.Home.Video.2019.mkv", 1, "Some.Home.Video.2019.mkv")

	rec, err := svc.Detect(context.Background(), hashA)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if rec.State != StateUnmatched || rec.Query != "Some Home Video (2019)" {
		t.Errorf("record = %+v, want unmatched with the query", rec)
	}
}

func TestLinkAndClear(t *testing.T) {
	client := &fakeClient{}
	svc, store := newService(t, client, &fakeSource{catalogue: []Match{interstellar}}, Options{AutoDetect: true, Backfill: true})
	ctx := context.Background()

	if _, err := svc.Link(ctx, hashA, "https://example.com/film"); !errors.Is(err, ErrUnsupportedLink) {
		t.Errorf("Link(unsupported) error = %v", err)
	}

	rec, err := svc.Link(ctx, hashA, "https://www.imdb.com/title/tt0816692/")
	if err != nil {
		t.Fatalf("Link(imdb) error = %v", err)
	}
	if rec.TMDBID != 157336 || !rec.Manual || !rec.Confident {
		t.Errorf("record = %+v, want a manual match", rec)
	}

	rec, err = svc.Clear(ctx, hashA)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if rec.State != StateUnmatched || !rec.Manual || rec.TMDBID != 0 {
		t.Errorf("record = %+v, want a cleared manual record", rec)
	}

	// The cleared torrent is not looked up again in the background.
	client.add(hashA, "Interstellar 2014", 1, "a.mkv")
	svc.scan(ctx)
	if rec, _, _ := store.Get(ctx, hashA); rec.State != StateUnmatched {
		t.Errorf("state = %q, want the user's choice kept", rec.State)
	}
}

func TestRenameSingleFileKeepsExtension(t *testing.T) {
	client := &fakeClient{}
	svc, store := newService(t, client, &fakeSource{}, Options{Template: "{original_title} ({year})"})
	ctx := context.Background()
	client.add(hashA, "interstellar.2014.mkv", 1, "interstellar.2014.mkv")

	if _, err := svc.Rename(ctx, hashA); !errors.Is(err, ErrNotMatched) {
		t.Errorf("Rename(unmatched) error = %v", err)
	}

	store.SaveMatch(ctx, hashA, interstellar, true, true, "") //nolint:errcheck
	current, target, err := svc.RenamePreview(ctx, hashA)
	if err != nil || current != "interstellar.2014.mkv" || target != "Interstellar_(2014).mkv" {
		t.Errorf("RenamePreview() = %q, %q, %v", current, target, err)
	}

	name, err := svc.Rename(ctx, hashA)
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if name != "Interstellar_(2014).mkv" || client.torrents[hashA].Name != name {
		t.Errorf("renamed to %q, torrent is %q", name, client.torrents[hashA].Name)
	}
}

func TestPosterFileIsCached(t *testing.T) {
	client := &fakeClient{}
	dir := filepath.Join(t.TempDir(), "posters")
	svc, store := newService(t, client, &fakeSource{}, Options{PosterDir: dir})
	ctx := context.Background()

	if _, err := svc.PosterFile(ctx, hashA); !errors.Is(err, ErrNoPoster) {
		t.Errorf("PosterFile(no record) error = %v", err)
	}

	store.SaveMatch(ctx, hashA, interstellar, true, true, "") //nolint:errcheck
	file, err := svc.PosterFile(ctx, hashA)
	if err != nil {
		t.Fatalf("PosterFile() error = %v", err)
	}
	if file != filepath.Join(dir, "poster.jpg") {
		t.Errorf("file = %q", file)
	}
	if data, _ := os.ReadFile(file); string(data) != "jpeg" {
		t.Errorf("poster content = %q", data)
	}
}

func TestPosterPathCannotEscapeTheCache(t *testing.T) {
	client := &fakeClient{}
	svc, store := newService(t, client, &fakeSource{}, Options{})
	ctx := context.Background()
	evil := interstellar
	evil.PosterPath = "/../../etc/passwd"
	store.SaveMatch(ctx, hashA, evil, true, true, "") //nolint:errcheck

	if _, err := svc.PosterFile(ctx, hashA); !errors.Is(err, ErrNoPoster) {
		t.Errorf("PosterFile() error = %v, want ErrNoPoster", err)
	}
}

func ptr[T any](v T) *T { return &v }

// fakeAutoRemove is the user's auto-remove switch.
type fakeAutoRemove struct {
	enabled bool
	err     error
}

func (f *fakeAutoRemove) MovieAutoRemoveEnabled(context.Context) (bool, error) {
	return f.enabled, f.err
}

func (f *fakeAutoRemove) SetMovieAutoRemoveEnabled(_ context.Context, enabled bool) error {
	f.enabled = enabled
	return f.err
}

func TestScanRemovesWatchedTorrentsThatPaidBack(t *testing.T) {
	ctx := context.Background()
	repaid := transmission.Torrent{PercentDone: 1, UploadRatio: 1.2, DownloadedEver: 100, UploadedEver: 120}
	owing := transmission.Torrent{PercentDone: 1, UploadRatio: 0.5, DownloadedEver: 100, UploadedEver: 50}

	setup := func(t *testing.T, toggle *fakeAutoRemove) (*fakeClient, *Store, *Service) {
		t.Helper()
		client := &fakeClient{}
		svc, store := newService(t, client, &fakeSource{}, Options{AutoRemove: toggle, TargetRatio: 1, DeleteData: true})
		client.add(hashA, "watched and repaid", 1, "a.mkv")
		client.add(hashB, "watched but owing", 1, "b.mkv")
		client.add(hashC, "repaid, not watched", 1, "c.mkv")
		client.stats = map[string]transmission.Torrent{hashA: repaid, hashB: owing, hashC: repaid}
		store.SetFlags(ctx, hashA, Flags{Watched: ptr(true)}) //nolint:errcheck
		store.SetFlags(ctx, hashB, Flags{Watched: ptr(true)}) //nolint:errcheck
		return client, store, svc
	}

	t.Run("on", func(t *testing.T) {
		client, _, svc := setup(t, &fakeAutoRemove{enabled: true})
		svc.scan(ctx)
		if len(client.removed) != 1 || client.removed[0] != 1 || !client.removedWithData {
			t.Errorf("removed = %v (with data %v), want only the watched and repaid torrent with its data", client.removed, client.removedWithData)
		}
	})

	t.Run("off by default", func(t *testing.T) {
		client, _, svc := setup(t, &fakeAutoRemove{})
		svc.scan(ctx)
		if len(client.removed) != 0 {
			t.Errorf("removed = %v, want nothing while the rule is off", client.removed)
		}
	})

	t.Run("failed lookup counts as off", func(t *testing.T) {
		client, _, svc := setup(t, &fakeAutoRemove{enabled: true, err: errors.New("db down")})
		svc.scan(ctx)
		if len(client.removed) != 0 {
			t.Errorf("removed = %v, want nothing when the switch cannot be read", client.removed)
		}
	})

	t.Run("incomplete download is kept", func(t *testing.T) {
		client, _, svc := setup(t, &fakeAutoRemove{enabled: true})
		client.stats[hashA] = transmission.Torrent{PercentDone: 0.9, UploadRatio: 1.5, DownloadedEver: 100, UploadedEver: 150}
		svc.scan(ctx)
		if len(client.removed) != 0 {
			t.Errorf("removed = %v, want an unfinished torrent kept", client.removed)
		}
	})
}

func TestAutoRemoveSwitch(t *testing.T) {
	ctx := context.Background()
	toggle := &fakeAutoRemove{}
	svc, _ := newService(t, &fakeClient{}, &fakeSource{}, Options{AutoRemove: toggle})

	if svc.AutoRemoveEnabled(ctx) {
		t.Error("AutoRemoveEnabled() = true, want off by default")
	}
	if err := svc.SetAutoRemoveEnabled(ctx, true); err != nil || !svc.AutoRemoveEnabled(ctx) {
		t.Errorf("SetAutoRemoveEnabled(true) error = %v, enabled = %v", err, svc.AutoRemoveEnabled(ctx))
	}

	bare, _ := newService(t, &fakeClient{}, &fakeSource{}, Options{})
	if err := bare.SetAutoRemoveEnabled(ctx, true); !errors.Is(err, ErrAutoRemoveUnavailable) {
		t.Errorf("SetAutoRemoveEnabled() without a store error = %v", err)
	}
}

// A match saved by 1.6.0 has no English title; renaming fetches and keeps it.
func TestRenameFetchesMissingEnglishTitle(t *testing.T) {
	client := &fakeClient{}
	source := &fakeSource{catalogue: []Match{interstellar}}
	svc, store := newService(t, client, source, Options{})
	ctx := context.Background()
	client.add(hashA, "Interstellar.2014.1080p", 1, "Interstellar.2014.1080p/movie.mkv")

	old := interstellar
	old.EnglishTitle = ""
	store.SaveMatch(ctx, hashA, old, false, true, "") //nolint:errcheck

	_, target, err := svc.RenamePreview(ctx, hashA)
	if err != nil || target != "Interstellar_(2014)" {
		t.Errorf("RenamePreview() target = %q, %v", target, err)
	}
	if rec, _, _ := store.Get(ctx, hashA); rec.EnglishTitle != "Interstellar" {
		t.Errorf("EnglishTitle = %q, want it saved", rec.EnglishTitle)
	}
}
