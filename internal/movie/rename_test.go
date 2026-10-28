package movie

import (
	"strings"
	"testing"
)

func TestRenderName(t *testing.T) {
	m := Match{MediaType: MediaMovie, TMDBID: 157336, IMDbID: "tt0816692", Title: "Интерстеллар", OriginalTitle: "Interstellar", EnglishTitle: "Interstellar", Year: 2014}
	tests := []struct {
		template string
		match    Match
		want     string
	}{
		{template: "", match: m, want: "Interstellar_(2014)"},
		{template: "", match: Match{Title: "Амели", OriginalTitle: "Le Fabuleux Destin d'Amélie Poulain", EnglishTitle: "Amélie", Year: 2001}, want: "Amélie_(2001)"},
		{template: "", match: Match{Title: "Брат", OriginalTitle: "Брат", Year: 1997}, want: "Брат_(1997)"},
		{template: "{english_title}", match: Match{Title: "Only Title"}, want: "Only_Title"},
		{template: "{title} ({year})", match: m, want: "Интерстеллар_(2014)"},
		{template: "{original_title} ({year})", match: m, want: "Interstellar_(2014)"},
		{template: "{title} [{imdb_id}] {tmdb_id} {type}", match: m, want: "Интерстеллар_[tt0816692]_157336_movie"},
		{template: "{title} ({year})", match: Match{Title: "Untitled"}, want: "Untitled"},
		{template: "{title} ({year})", match: Match{Title: "Mission: Impossible / Fallout?", Year: 2018}, want: "Mission_Impossible_Fallout_(2018)"},
		{template: "{original_title}", match: Match{Title: "Only Title"}, want: "Only_Title"},
		{template: "{title}...", match: Match{Title: "Ends"}, want: "Ends"},
		{template: "{year}", match: Match{Title: "X"}, want: ""},
		{template: "", match: Match{MediaType: MediaTV, Title: "Остаться в живых", EnglishTitle: "Lost", Year: 2004, Season: 2}, want: "Lost_(2004)_S02"},
		{template: "{title} - {season}", match: Match{MediaType: MediaTV, Title: "Lost", Season: 1}, want: "Lost_-_S01"},
		{template: "{title} {season}", match: Match{MediaType: MediaTV, Title: "Lost"}, want: "Lost"},
		{template: "{title} {season}", match: Match{MediaType: MediaMovie, Title: "Up", Season: 3}, want: "Up"},
	}
	for _, tt := range tests {
		if got := RenderName(tt.template, tt.match); got != tt.want {
			t.Errorf("RenderName(%q, %+v) = %q, want %q", tt.template, tt.match, got, tt.want)
		}
	}
}

func TestRenderNameIsBounded(t *testing.T) {
	got := RenderName("{title}", Match{Title: strings.Repeat("Очень длинное название ", 30)})
	if len(got) > maxNameBytes {
		t.Errorf("len = %d, want at most %d", len(got), maxNameBytes)
	}
	if strings.HasSuffix(got, "_") || strings.Contains(got, " ") {
		t.Errorf("name %q has a space or ends with an underscore", got)
	}
}

func TestTargetName(t *testing.T) {
	if got := TargetName("Interstellar (2014)", "interstellar.2014.mkv", true); got != "Interstellar (2014).mkv" {
		t.Errorf("single file: %q", got)
	}
	if got := TargetName("Interstellar (2014)", "Interstellar.2014.1080p", false); got != "Interstellar (2014)" {
		t.Errorf("folder: %q", got)
	}
	if got := TargetName("", "x.mkv", true); got != "" {
		t.Errorf("empty render must stay empty, got %q", got)
	}
}
