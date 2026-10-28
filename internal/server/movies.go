package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/lebe-dev/transmitter/internal/movie"
	"github.com/lebe-dev/transmitter/internal/transmission"
)

// movieBodyMaxBytes caps the small JSON bodies of the movie endpoints (a link
// or two flags).
const movieBodyMaxBytes = 2048

// MovieService is the Movie Plugin as seen by the HTTP layer; *movie.Service
// implements it.
type MovieService interface {
	All(ctx context.Context) (map[string]movie.Record, error)
	Detect(ctx context.Context, hash string) (movie.Record, error)
	Link(ctx context.Context, hash, link string) (movie.Record, error)
	Clear(ctx context.Context, hash string) (movie.Record, error)
	SetFlags(ctx context.Context, hash string, f movie.Flags) (movie.Record, error)
	RenamePreview(ctx context.Context, hash string) (current, target string, err error)
	Rename(ctx context.Context, hash string) (string, error)
	PosterFile(ctx context.Context, hash string) (string, error)
	Wake()
}

// registerMovieRoutes adds the Movie Plugin endpoints. They do not exist at all
// while the plugin is disabled.
func registerMovieRoutes(mux *http.ServeMux, svc MovieService) {
	mux.Handle("GET /api/movies", MoviesHandler(svc))
	mux.Handle("POST /api/movies/{hash}/detect", MovieDetectHandler(svc))
	mux.Handle("PUT /api/movies/{hash}/link", MovieLinkHandler(svc))
	mux.Handle("DELETE /api/movies/{hash}/match", MovieClearHandler(svc))
	mux.Handle("PUT /api/movies/{hash}/flags", MovieFlagsHandler(svc))
	mux.Handle("GET /api/movies/{hash}/rename", MovieRenamePreviewHandler(svc))
	mux.Handle("POST /api/movies/{hash}/rename", MovieRenameHandler(svc))
	mux.Handle("GET /api/movies/{hash}/poster", MoviePosterHandler(svc))
}

// MoviesHandler returns every record keyed by torrent hash.
func MoviesHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := svc.All(r.Context())
		if err != nil {
			slog.Error("failed to list movies", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to load movies")
			return
		}
		writeJSON(w, all)
	}
}

// MovieDetectHandler looks the torrent up by its name again.
func MovieDetectHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := svc.Detect(r.Context(), r.PathValue("hash"))
		if err != nil {
			writeMovieError(w, "detect", err)
			return
		}
		writeJSON(w, rec)
	}
}

// movieLinkRequest is the body of PUT /api/movies/{hash}/link.
type movieLinkRequest struct {
	URL string `json:"url"`
}

// MovieLinkHandler matches the torrent to the title behind a TMDB or IMDb link.
func MovieLinkHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req movieLinkRequest
		if !readMovieBody(w, r, &req) {
			return
		}
		rec, err := svc.Link(r.Context(), r.PathValue("hash"), req.URL)
		if err != nil {
			writeMovieError(w, "link", err)
			return
		}
		writeJSON(w, rec)
	}
}

// MovieClearHandler forgets the match of a torrent.
func MovieClearHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := svc.Clear(r.Context(), r.PathValue("hash"))
		if err != nil {
			writeMovieError(w, "clear", err)
			return
		}
		writeJSON(w, rec)
	}
}

// MovieFlagsHandler sets the watched / missing subtitles marks.
func MovieFlagsHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var f movie.Flags
		if !readMovieBody(w, r, &f) {
			return
		}
		if f.Watched == nil && f.MissingSubs == nil {
			writeJSONError(w, http.StatusBadRequest, "nothing to update")
			return
		}
		rec, err := svc.SetFlags(r.Context(), r.PathValue("hash"), f)
		if err != nil {
			writeMovieError(w, "flags", err)
			return
		}
		writeJSON(w, rec)
	}
}

// MovieRenamePreviewHandler returns the current and the would-be name.
func MovieRenamePreviewHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current, target, err := svc.RenamePreview(r.Context(), r.PathValue("hash"))
		if err != nil {
			writeMovieError(w, "rename preview", err)
			return
		}
		writeJSON(w, map[string]string{"current": current, "target": target})
	}
}

// MovieRenameHandler renames the torrent on disk after its matched title.
func MovieRenameHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, err := svc.Rename(r.Context(), r.PathValue("hash"))
		if err != nil {
			writeMovieError(w, "rename", err)
			return
		}
		slog.Info("movie plugin: torrent renamed by user", "name", name)
		writeJSON(w, map[string]string{"name": name})
	}
}

// MoviePosterHandler serves the cached poster of the torrent's title.
func MoviePosterHandler(svc MovieService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, err := svc.PosterFile(r.Context(), r.PathValue("hash"))
		if err != nil {
			writeMovieError(w, "poster", err)
			return
		}
		f, err := os.Open(file)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "poster not found")
			return
		}
		defer f.Close() //nolint:errcheck
		info, err := f.Stat()
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "poster not found")
			return
		}
		// The poster of a title never changes; a re-match changes the file name
		// and the UI busts the cache with the title ID.
		w.Header().Set("Cache-Control", "private, max-age=604800")
		http.ServeContent(w, r, filepath.Base(file), info.ModTime(), f)
	}
}

func readMovieBody(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, movieBodyMaxBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request too large")
		return false
	}
	if err := json.Unmarshal(body, out); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set(headerContentType, mimeJSON)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeMovieError maps plugin errors to HTTP statuses. Failures of TMDB or
// Transmission are reported as 502; their text never carries the API key.
func writeMovieError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, movie.ErrInvalidHash):
		writeJSONError(w, http.StatusBadRequest, "invalid torrent hash")
	case errors.Is(err, movie.ErrUnsupportedLink):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, movie.ErrEmptyName):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, movie.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "title not found")
	case errors.Is(err, movie.ErrNoPoster):
		writeJSONError(w, http.StatusNotFound, "no poster")
	case errors.Is(err, transmission.ErrTorrentNotFound):
		writeJSONError(w, http.StatusNotFound, "torrent not found")
	case errors.Is(err, movie.ErrNotMatched):
		writeJSONError(w, http.StatusConflict, err.Error())
	default:
		slog.Warn("movie plugin request failed", "action", action, "err", err)
		writeJSONError(w, http.StatusBadGateway, err.Error())
	}
}
