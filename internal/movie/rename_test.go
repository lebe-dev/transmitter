package movie

import (
	"strings"
	"testing"
)

func TestRenderName(t *testing.T) {
	m := Match{MediaType: MediaMovie, TMDBID: 157336, IMDbID: "tt0816692", Title: "Интерстеллар", OriginalTitle: "Interstellar", Year: 2014}
	tests := []struct {
		template string
		match    Match
		want     string
	}{
		{template: "", match: m, want: "Интерстеллар (2014)"},
		{template: "{original_title} ({year})", match: m, want: "Interstellar (2014)"},
		{template: "{title} [{imdb_id}] {tmdb_id} {type}", match: m, want: "Интерстеллар [tt0816692] 157336 movie"},
		{template: "{title} ({year})", match: Match{Title: "Untitled"}, want: "Untitled"},
		{template: "{title} ({year})", match: Match{Title: "Mission: Impossible / Fallout?", Year: 2018}, want: "Mission Impossible Fallout (2018)"},
		{template: "{original_title}", match: Match{Title: "Only Title"}, want: "Only Title"},
		{template: "{title}...", match: Match{Title: "Ends"}, want: "Ends"},
		{template: "{year}", match: Match{Title: "X"}, want: ""},
		{template: "", match: Match{MediaType: MediaTV, Title: "Lost", Year: 2004, Season: 2}, want: "Lost (2004) S02"},
		{template: "{title} - {season}", match: Match{MediaType: MediaTV, Title: "Lost", Season: 1}, want: "Lost - S01"},
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
	if strings.HasSuffix(got, " ") {
		t.Errorf("name %q ends with a space", got)
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
