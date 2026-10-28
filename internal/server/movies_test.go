package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lebe-dev/transmitter/internal/config"
	"github.com/lebe-dev/transmitter/internal/movie"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

// fakeMovies records calls and returns canned results.
type fakeMovies struct {
	err     error
	flags   movie.Flags
	link    string
	poster  string
	renamed string
	woken   chan struct{}
}

func (f *fakeMovies) All(context.Context) (map[string]movie.Record, error) {
	return map[string]movie.Record{testHash: {State: movie.StateMatched, Title: "Interstellar", Year: 2014}}, f.err
}

func (f *fakeMovies) Detect(context.Context, string) (movie.Record, error) {
	return movie.Record{State: movie.StateUnmatched, Query: "q"}, f.err
}

func (f *fakeMovies) Link(_ context.Context, _ string, link string) (movie.Record, error) {
	f.link = link
	return movie.Record{State: movie.StateMatched, Manual: true}, f.err
}

func (f *fakeMovies) Clear(context.Context, string) (movie.Record, error) {
	return movie.Record{State: movie.StateUnmatched}, f.err
}

func (f *fakeMovies) SetFlags(_ context.Context, _ string, fl movie.Flags) (movie.Record, error) {
	f.flags = fl
	return movie.Record{Watched: fl.Watched != nil && *fl.Watched}, f.err
}

func (f *fakeMovies) RenamePreview(context.Context, string) (string, string, error) {
	return "a.mkv", "A (2014).mkv", f.err
}

func (f *fakeMovies) Rename(context.Context, string) (string, error) {
	f.renamed = "A (2014).mkv"
	return f.renamed, f.err
}

func (f *fakeMovies) PosterFile(context.Context, string) (string, error) {
	return f.poster, f.err
}

func (f *fakeMovies) Wake() {
	if f.woken != nil {
		f.woken <- struct{}{}
	}
}

func movieRequest(t *testing.T, h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/movies/"+testHash, strings.NewReader(body))
	req.SetPathValue("hash", testHash)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMoviesHandler(t *testing.T) {
	rec := movieRequest(t, MoviesHandler(&fakeMovies{}), http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got map[string]movie.Record
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got[testHash].Title != "Interstellar" {
		t.Errorf("body = %s, err = %v", rec.Body.String(), err)
	}
}

func TestMovieLinkHandler(t *testing.T) {
	svc := &fakeMovies{}
	rec := movieRequest(t, MovieLinkHandler(svc), http.MethodPut, `{"url":"https://www.imdb.com/title/tt0816692/"}`)
	if rec.Code != http.StatusOK || svc.link != "https://www.imdb.com/title/tt0816692/" {
		t.Errorf("status = %d, link = %q", rec.Code, svc.link)
	}

	rec = movieRequest(t, MovieLinkHandler(svc), http.MethodPut, `{bad`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid json: status = %d, want 400", rec.Code)
	}
}

func TestMovieFlagsHandler(t *testing.T) {
	svc := &fakeMovies{}
	rec := movieRequest(t, MovieFlagsHandler(svc), http.MethodPut, `{"watched":true}`)
	if rec.Code != http.StatusOK || svc.flags.Watched == nil || !*svc.flags.Watched || svc.flags.MissingSubs != nil {
		t.Errorf("status = %d, flags = %+v", rec.Code, svc.flags)
	}

	rec = movieRequest(t, MovieFlagsHandler(svc), http.MethodPut, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty flags: status = %d, want 400", rec.Code)
	}
}

func TestMovieRenameHandlers(t *testing.T) {
	svc := &fakeMovies{}
	rec := movieRequest(t, MovieRenamePreviewHandler(svc), http.MethodGet, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"target":"A (2014).mkv"`) {
		t.Errorf("preview: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = movieRequest(t, MovieRenameHandler(svc), http.MethodPost, "")
	if rec.Code != http.StatusOK || svc.renamed == "" {
		t.Errorf("rename: status = %d", rec.Code)
	}
}

func TestMovieErrorStatuses(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{movie.ErrInvalidHash, http.StatusBadRequest},
		{movie.ErrUnsupportedLink, http.StatusUnprocessableEntity},
		{movie.ErrEmptyName, http.StatusUnprocessableEntity},
		{movie.ErrNotFound, http.StatusNotFound},
		{movie.ErrNoPoster, http.StatusNotFound},
		{transmission.ErrTorrentNotFound, http.StatusNotFound},
		{movie.ErrNotMatched, http.StatusConflict},
		{errors.New("torrent-rename-path failed: Invalid argument"), http.StatusBadGateway},
	}
	for _, tt := range tests {
		rec := movieRequest(t, MovieDetectHandler(&fakeMovies{err: tt.err}), http.MethodPost, "")
		if rec.Code != tt.want {
			t.Errorf("%v: status = %d, want %d", tt.err, rec.Code, tt.want)
		}
	}
}

func TestMoviePosterHandler(t *testing.T) {
	file := filepath.Join(t.TempDir(), "p.jpg")
	os.WriteFile(file, []byte("\xff\xd8\xffjpeg"), 0o600) //nolint:errcheck

	rec := movieRequest(t, MoviePosterHandler(&fakeMovies{poster: file}), http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Error("poster must be cacheable")
	}

	rec = movieRequest(t, MoviePosterHandler(&fakeMovies{err: movie.ErrNoPoster}), http.MethodGet, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("no poster: status = %d, want 404", rec.Code)
	}
}

func TestProxyWakesMoviePluginOnAdd(t *testing.T) {
	client := backend(t, func(method string, _ []byte) transmission.RPCResponse {
		if method == "torrent-add" {
			return transmission.RPCResponse{Result: "success"}
		}
		return transmission.RPCResponse{Result: "duplicate torrent"}
	})
	woken := 0
	h := ProxyHandler(client, AutoPriorityConfig{}, 1024, nil, func() { woken++ })

	for _, body := range []string{`{"method":"torrent-add"}`, `{"method":"torrent-get"}`} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(body)))
	}
	if woken != 1 {
		t.Errorf("woken = %d, want 1 (only after a successful torrent-add)", woken)
	}
}

func TestMovieRoutesExistOnlyWhenEnabled(t *testing.T) {
	cfg := &config.Config{ListenAddr: "127.0.0.1:0", CORSOrigin: "http://localhost", MaxRequestBodyBytes: 1024}
	staticFS := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tt := range []struct {
		name    string
		movies  MovieService
		enabled bool
	}{
		{name: "disabled", movies: nil, enabled: false},
		{name: "enabled", movies: &fakeMovies{}, enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := New(cfg, nil, noteStore(t, 200), nil, nil, nil, nil, tt.movies, "test", staticFS, logger)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			rec := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/movies", nil))
			isJSON := rec.Header().Get("Content-Type") == mimeJSON
			if isJSON != tt.enabled {
				t.Errorf("GET /api/movies served JSON = %v, want %v (body %q)", isJSON, tt.enabled, rec.Body.String())
			}

			rec = httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
			var settings UISettings
			json.Unmarshal(rec.Body.Bytes(), &settings) //nolint:errcheck
			if settings.MoviePluginEnabled != tt.enabled {
				t.Errorf("moviePluginEnabled = %v, want %v", settings.MoviePluginEnabled, tt.enabled)
			}
		})
	}
}
