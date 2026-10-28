package movie

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tmdbServer(t *testing.T, handler http.HandlerFunc) *TMDB {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewTMDB(srv.URL, srv.URL, "key123", "ru-RU")
}

func TestTMDBSearchMovie(t *testing.T) {
	c := tmdbServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/search/movie" || q.Get("query") != "Interstellar" || q.Get("year") != "2014" ||
			q.Get("api_key") != "key123" || q.Get("language") != "ru-RU" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"results":[{"id":157336,"title":"Интерстеллар","original_title":"Interstellar",` + //nolint:errcheck
			`"release_date":"2014-11-05","poster_path":"/p.jpg","vote_average":8.4}]}`))
	})

	got, err := c.Search(context.Background(), MediaMovie, "Interstellar", 2014)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	want := Match{MediaType: MediaMovie, TMDBID: 157336, Title: "Интерстеллар", OriginalTitle: "Interstellar", Year: 2014, PosterPath: "/p.jpg", Rating: 8.4}
	if len(got) != 1 || got[0] != want {
		t.Errorf("Search() = %+v, want %+v", got, want)
	}
}

func TestTMDBSearchTVUsesFirstAirDate(t *testing.T) {
	c := tmdbServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/tv" || r.URL.Query().Get("first_air_date_year") != "2004" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"results":[{"id":4607,"name":"Остаться в живых","original_name":"Lost","first_air_date":"2004-09-22"}]}`)) //nolint:errcheck
	})

	got, err := c.Search(context.Background(), MediaTV, "Lost", 2004)
	if err != nil || len(got) != 1 || got[0].Title != "Остаться в живых" || got[0].Year != 2004 || got[0].MediaType != MediaTV {
		t.Errorf("Search() = %+v, %v", got, err)
	}
}

func TestTMDBBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer eyJtoken" || r.URL.Query().Has("api_key") {
			t.Errorf("token must go in the header, got %s %v", r.URL, r.Header)
		}
		w.Write([]byte(`{"id":1,"title":"X","imdb_id":"tt0000001"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	m, err := NewTMDB(srv.URL, "", "eyJtoken", "").Details(context.Background(), MediaMovie, 1)
	if err != nil || m.IMDbID != "tt0000001" {
		t.Errorf("Details() = %+v, %v", m, err)
	}
}

func TestTMDBFindIMDb(t *testing.T) {
	c := tmdbServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/find/tt0903747":
			w.Write([]byte(`{"movie_results":[],"tv_results":[{"id":1396}]}`)) //nolint:errcheck
		case "/tv/1396":
			if r.URL.Query().Get("append_to_response") != "external_ids,translations" {
				t.Error("series details must ask for external ids and translations")
			}
			w.Write([]byte(`{"id":1396,"name":"Во все тяжкие","original_name":"Breaking Bad","first_air_date":"2008-01-20","external_ids":{"imdb_id":"tt0903747"},` + //nolint:errcheck
				`"translations":{"translations":[{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Breaking Bad"}}]}}`))
		default:
			http.NotFound(w, r)
		}
	})

	m, err := c.FindIMDb(context.Background(), "tt0903747")
	if err != nil || m.MediaType != MediaTV || m.IMDbID != "tt0903747" || m.Year != 2008 || m.EnglishTitle != "Breaking Bad" {
		t.Errorf("FindIMDb() = %+v, %v", m, err)
	}

	if _, err := c.FindIMDb(context.Background(), "tt9999999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindIMDb(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestTMDBDetailsEnglishTitle(t *testing.T) {
	tests := []struct {
		name     string
		language string
		body     string
		want     string
	}{
		{
			name:     "US translation wins over other English ones",
			language: "ru-RU",
			body: `{"id":1,"title":"Амели","original_title":"Le Fabuleux Destin d'Amélie Poulain","original_language":"fr",` +
				`"translations":{"translations":[{"iso_639_1":"en","iso_3166_1":"GB","data":{"title":"Amelie"}},` +
				`{"iso_639_1":"en","iso_3166_1":"US","data":{"title":"Amélie"}},{"iso_639_1":"de","iso_3166_1":"DE","data":{"title":"Die fabelhafte Welt der Amélie"}}]}}`,
			want: "Amélie",
		},
		{
			name:     "English original without a translation",
			language: "ru-RU",
			body:     `{"id":2,"title":"Интерстеллар","original_title":"Interstellar","original_language":"en","translations":{"translations":[]}}`,
			want:     "Interstellar",
		},
		{
			name:     "English client language",
			language: "en-US",
			body:     `{"id":3,"title":"Brother","original_title":"Брат","original_language":"ru"}`,
			want:     "Brother",
		},
		{
			name:     "no English title at all",
			language: "ru-RU",
			body:     `{"id":4,"title":"Брат","original_title":"Брат","original_language":"ru"}`,
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("append_to_response") != "translations" {
					t.Errorf("movie details must ask for translations, got %s", r.URL)
				}
				w.Write([]byte(tt.body)) //nolint:errcheck
			}))
			defer srv.Close()

			m, err := NewTMDB(srv.URL, "", "key", tt.language).Details(context.Background(), MediaMovie, 1)
			if err != nil || m.EnglishTitle != tt.want {
				t.Errorf("Details() EnglishTitle = %q, %v, want %q", m.EnglishTitle, err, tt.want)
			}
		})
	}
}

func TestTMDBErrorDoesNotLeakKey(t *testing.T) {
	c := NewTMDB("http://127.0.0.1:1", "", "secret-key", "")
	_, err := c.Search(context.Background(), MediaMovie, "x", 0)
	if err == nil {
		t.Fatal("Search() error = nil, want a connection error")
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Errorf("error %q leaks the API key", err)
	}
}

func TestParseLink(t *testing.T) {
	tests := []struct {
		in   string
		want Link
		err  bool
	}{
		{in: "https://www.themoviedb.org/movie/157336-interstellar", want: Link{MediaType: MediaMovie, TMDBID: 157336}},
		{in: "https://www.themoviedb.org/tv/1396-breaking-bad?language=ru", want: Link{MediaType: MediaTV, TMDBID: 1396}},
		{in: "themoviedb.org/ru-RU/movie/603", want: Link{MediaType: MediaMovie, TMDBID: 603}},
		{in: "https://www.imdb.com/title/tt0816692/?ref_=nv_sr", want: Link{IMDbID: "tt0816692"}},
		{in: "https://m.imdb.com/title/tt0903747", want: Link{IMDbID: "tt0903747"}},
		{in: " tt0816692 ", want: Link{IMDbID: "tt0816692"}},
		{in: "https://www.kinopoisk.ru/film/258687/", err: true},
		{in: "https://evil.example/tt0816692", err: true},
		{in: "", err: true},
	}
	for _, tt := range tests {
		got, err := ParseLink(tt.in)
		if tt.err {
			if !errors.Is(err, ErrUnsupportedLink) {
				t.Errorf("ParseLink(%q) error = %v, want ErrUnsupportedLink", tt.in, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseLink(%q) = %+v, %v; want %+v", tt.in, got, err, tt.want)
		}
	}
}
