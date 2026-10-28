package movie

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Media types as TMDB names them.
const (
	MediaMovie = "movie"
	MediaTV    = "tv"
)

// DefaultTMDBURL and DefaultTMDBImageURL are the public TMDB endpoints. Both
// can be overridden, e.g. to go through a mirror where TMDB is unreachable.
const (
	DefaultTMDBURL      = "https://api.themoviedb.org/3"
	DefaultTMDBImageURL = "https://image.tmdb.org/t/p"
	// posterSize is wide enough for the dialog on a retina phone and still small.
	posterSize = "w342"
	// maxPosterBytes guards the cache against an unexpectedly large download.
	maxPosterBytes = 5 << 20
)

var (
	// ErrNotFound is returned when TMDB has nothing for the query or link.
	ErrNotFound = errors.New("movie not found")
	// ErrUnsupportedLink is returned for a link that is not a TMDB or IMDb page.
	ErrUnsupportedLink = errors.New("unsupported link: expected a TMDB or IMDb page")
)

// Match is a title found on TMDB.
type Match struct {
	MediaType     string
	TMDBID        int64
	IMDbID        string
	Title         string
	OriginalTitle string
	Year          int
	Overview      string
	PosterPath    string
	Rating        float64
	// Season is the season of a series release ("S02"), taken from the torrent
	// name rather than TMDB; 0 for movies and whole-series torrents.
	Season int
}

// TMDB is a minimal client for The Movie Database API v3.
type TMDB struct {
	baseURL    string
	imageURL   string
	apiKey     string
	language   string
	httpClient *http.Client
}

// NewTMDB creates a client. apiKey may be a v3 API key or a v4 read access
// token (a JWT); the latter is sent as a bearer token. language is a TMDB
// locale such as "en-US" or "ru-RU".
func NewTMDB(baseURL, imageURL, apiKey, language string) *TMDB {
	if baseURL == "" {
		baseURL = DefaultTMDBURL
	}
	if imageURL == "" {
		imageURL = DefaultTMDBImageURL
	}
	return &TMDB{
		baseURL:  strings.TrimRight(baseURL, "/"),
		imageURL: strings.TrimRight(imageURL, "/"),
		apiKey:   apiKey,
		language: language,
		// The default transport honours HTTPS_PROXY, which is how TMDB is
		// reached from networks that block it.
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// tmdbResult is a search hit; movies fill Title/ReleaseDate, series Name/FirstAirDate.
type tmdbResult struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	Name          string  `json:"name"`
	OriginalName  string  `json:"original_name"`
	FirstAirDate  string  `json:"first_air_date"`
	Overview      string  `json:"overview"`
	PosterPath    string  `json:"poster_path"`
	VoteAverage   float64 `json:"vote_average"`
	IMDbID        string  `json:"imdb_id"`
	ExternalIDs   struct {
		IMDbID string `json:"imdb_id"`
	} `json:"external_ids"`
}

func (r tmdbResult) match(mediaType string) Match {
	m := Match{
		MediaType:  mediaType,
		TMDBID:     r.ID,
		IMDbID:     r.IMDbID,
		Overview:   r.Overview,
		PosterPath: r.PosterPath,
		Rating:     r.VoteAverage,
	}
	if m.IMDbID == "" {
		m.IMDbID = r.ExternalIDs.IMDbID
	}
	date := r.ReleaseDate
	if mediaType == MediaTV {
		m.Title, m.OriginalTitle, date = r.Name, r.OriginalName, r.FirstAirDate
	} else {
		m.Title, m.OriginalTitle = r.Title, r.OriginalTitle
	}
	if len(date) >= 4 {
		m.Year, _ = strconv.Atoi(date[:4])
	}
	return m
}

// Search looks a title up among movies or series, optionally narrowed by year.
// It returns TMDB's best hits in relevance order.
func (c *TMDB) Search(ctx context.Context, mediaType, query string, year int) ([]Match, error) {
	params := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		if mediaType == MediaTV {
			params.Set("first_air_date_year", strconv.Itoa(year))
		} else {
			params.Set("year", strconv.Itoa(year))
		}
	}

	var resp struct {
		Results []tmdbResult `json:"results"`
	}
	if err := c.get(ctx, "/search/"+mediaType, params, &resp); err != nil {
		return nil, err
	}

	matches := make([]Match, 0, len(resp.Results))
	for _, r := range resp.Results {
		matches = append(matches, r.match(mediaType))
	}
	return matches, nil
}

// Details fetches a movie or series by its TMDB ID, including the IMDb ID.
func (c *TMDB) Details(ctx context.Context, mediaType string, id int64) (Match, error) {
	var r tmdbResult
	params := url.Values{}
	if mediaType == MediaTV {
		params.Set("append_to_response", "external_ids")
	}
	if err := c.get(ctx, fmt.Sprintf("/%s/%d", mediaType, id), params, &r); err != nil {
		return Match{}, err
	}
	return r.match(mediaType), nil
}

// FindIMDb resolves an IMDb ID (tt1234567) to a movie or series.
func (c *TMDB) FindIMDb(ctx context.Context, imdbID string) (Match, error) {
	var resp struct {
		MovieResults []tmdbResult `json:"movie_results"`
		TVResults    []tmdbResult `json:"tv_results"`
	}
	params := url.Values{"external_source": {"imdb_id"}}
	if err := c.get(ctx, "/find/"+imdbID, params, &resp); err != nil {
		return Match{}, err
	}
	switch {
	case len(resp.MovieResults) > 0:
		return c.Details(ctx, MediaMovie, resp.MovieResults[0].ID)
	case len(resp.TVResults) > 0:
		return c.Details(ctx, MediaTV, resp.TVResults[0].ID)
	}
	return Match{}, ErrNotFound
}

// Poster downloads a poster image by its TMDB path ("/abc.jpg").
func (c *TMDB) Poster(ctx context.Context, posterPath string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.imageURL+"/"+posterSize+posterPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch poster: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch poster: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPosterBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read poster: %w", err)
	}
	if len(data) > maxPosterBytes {
		return nil, errors.New("poster too large")
	}
	return data, nil
}

func (c *TMDB) get(ctx context.Context, path string, params url.Values, out any) error {
	if c.language != "" {
		params.Set("language", c.language)
	}
	bearer := strings.HasPrefix(c.apiKey, "eyJ")
	if !bearer {
		params.Set("api_key", c.apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// The URL carries the API key; keep it out of logs.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("tmdb %s: %w", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("tmdb %s: HTTP %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("tmdb %s: decode: %w", path, err)
	}
	return nil
}

var (
	tmdbLinkRe = regexp.MustCompile(`themoviedb\.org/(?:[a-z]{2}(?:-[A-Z]{2})?/)?(movie|tv)/(\d+)`)
	imdbIDRe   = regexp.MustCompile(`\btt\d{6,10}\b`)
)

// Link is a parsed reference to a TMDB or IMDb page.
type Link struct {
	MediaType string
	TMDBID    int64
	IMDbID    string
}

// ParseLink understands TMDB pages (themoviedb.org/movie/603-the-matrix),
// IMDb pages (imdb.com/title/tt0133093) and bare IMDb IDs.
func ParseLink(raw string) (Link, error) {
	s := strings.TrimSpace(raw)
	if m := tmdbLinkRe.FindStringSubmatch(s); m != nil {
		id, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil || id <= 0 {
			return Link{}, ErrUnsupportedLink
		}
		return Link{MediaType: m[1], TMDBID: id}, nil
	}
	if id := imdbIDRe.FindString(s); id != "" && (strings.Contains(s, "imdb.com") || s == id) {
		return Link{IMDbID: id}, nil
	}
	return Link{}, ErrUnsupportedLink
}
