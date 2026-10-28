package movie

import (
	"reflect"
	"testing"
)

func TestParseRelease(t *testing.T) {
	tests := []struct {
		name string
		want Release
	}{
		{
			name: "Interstellar.2014.1080p.BluRay.x264-SPARKS",
			want: Release{Titles: []string{"Interstellar"}, Year: 2014},
		},
		{
			name: "The.Matrix.1999.REMASTERED.2160p.UHD.BluRay.x265-TERMiNAL",
			want: Release{Titles: []string{"The Matrix"}, Year: 1999},
		},
		{
			name: "Blade.Runner.2049.2017.1080p.WEB-DL.mkv",
			want: Release{Titles: []string{"Blade Runner 2049"}, Year: 2017},
		},
		{
			name: "2001.A.Space.Odyssey.1968.1080p",
			want: Release{Titles: []string{"2001 A Space Odyssey"}, Year: 1968},
		},
		{
			name: "1917.1080p.BluRay",
			want: Release{Titles: []string{"1917"}},
		},
		{
			name: "Some_Movie_(2010)_720p.mkv",
			want: Release{Titles: []string{"Some Movie"}, Year: 2010},
		},
		{
			name: "[YTS.MX] Dune Part Two (2024) [1080p]",
			want: Release{Titles: []string{"Dune Part Two"}, Year: 2024},
		},
		{
			name: "Интерстеллар / Interstellar (2014) BDRip 1080p",
			want: Release{Titles: []string{"Интерстеллар", "Interstellar"}, Year: 2014},
		},
		{
			name: "Интерстеллар / Interstellar (Кристофер Нолан / Christopher Nolan) [2014, США, фантастика, BDRip 1080p] Dub + Original",
			want: Release{Titles: []string{"Интерстеллар", "Interstellar"}, Year: 2014},
		},
		{
			name: "Mr. Robot S01 1080p",
			want: Release{Titles: []string{"Mr. Robot"}, TV: true, Season: 1},
		},
		{
			name: "Game.of.Thrones.S08E03.720p.WEB.H264-MEMENTO",
			want: Release{Titles: []string{"Game of Thrones"}, TV: true, Season: 8},
		},
		{
			name: "Breaking.Bad.S02E01-E03.1080p",
			want: Release{Titles: []string{"Breaking Bad"}, TV: true, Season: 2},
		},
		{
			name: "Остаться в живых / Lost / Сезон: 1 / Серии: 1-25 из 25 (Джей Джей Абрамс) [2004, США, драма, BDRip] MVO",
			want: Release{Titles: []string{"Остаться в живых", "Lost"}, Year: 2004, TV: true, Season: 1},
		},
		{
			name: "Ёлки 2010",
			want: Release{Titles: []string{"Ёлки"}, Year: 2010},
		},
		{
			name: "Samurai Jack",
			want: Release{Titles: []string{"Samurai Jack"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRelease(tt.name)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseRelease(%q) = %+v, want %+v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsVideoFile(t *testing.T) {
	for name, want := range map[string]bool{
		"movie.mkv":         true,
		"Dir/Movie.MP4":     true,
		"readme.txt":        false,
		"ubuntu-24.04.iso":  false,
		"Season 1/e01.m2ts": true,
	} {
		if got := IsVideoFile(name); got != want {
			t.Errorf("IsVideoFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNormalizeForCompare(t *testing.T) {
	if normalizeForCompare("Ёлки: Новые!") != normalizeForCompare("елки новые") {
		t.Error("ё and punctuation must not affect the comparison")
	}
	if normalizeForCompare("Fast & Furious") != normalizeForCompare("Fast and Furious") {
		t.Error("& must compare equal to 'and'")
	}
}
