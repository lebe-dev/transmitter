// Package movie is the Movie Plugin: it recognizes films and series by the
// torrent name, pulls their details and posters from TMDB, renames the
// downloaded folder or file and keeps per-torrent marks (watched, missing
// subtitles) in the local SQLite database.
package movie

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Release is what a torrent name says about its content.
type Release struct {
	// Titles are the candidate titles, most specific first. Russian trackers
	// name releases "Русское / Original", so there are often two.
	Titles []string
	// Year is the release year, 0 when the name carries none.
	Year int
	// TV is set when the name has a season/episode marker.
	TV     bool
	Season int
}

// videoExtensions are the file types counted as video when deciding whether a
// torrent is worth looking up.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true,
	".wmv": true, ".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	".webm": true, ".vob": true, ".flv": true, ".3gp": true, ".ogv": true,
}

// IsVideoFile reports whether the file name has a video extension.
func IsVideoFile(name string) bool {
	return videoExtensions[strings.ToLower(path.Ext(name))]
}

var (
	// junkRe matches the first technical token of a release name (quality,
	// source, codec, audio, edition). Everything from it on is not the title.
	junkRe = regexp.MustCompile(`(?i)(?:^|[\s\[\(\-,])(` +
		`2160p|1080p|1080i|720p|576p|480p|4k|uhd|hdr10\+?|hdr|sdr|` +
		`blu-?ray|bdrip|brrip|bdremux|remux|web-?dl|webrip|hdtv|hdrip|dvdrip|dvd5|dvd9|dvdscr|camrip|tvrip|satrip|` +
		`x\.?264|x\.?265|h\.?264|h\.?265|hevc|avc|xvid|divx|10bit|8bit|` +
		`aac|ac3|e-?ac3|dts(?:-hd)?|truehd|atmos|ddp?5\.?1|` +
		`proper|repack|extended|unrated|remastered|imax|hybrid|open\s?matte|director'?s\s?cut|theatrical|` +
		`amzn|itunes|nf|hmax|dsnp|multi|dub|mvo|dvo|avo|sub|subs|` +
		`complete|limited|internal` +
		`)(?:$|[\s\]\)\-,.])`)

	// tvRe matches S01E02, S01, 1x02 markers and "Season 1" / "Сезон: 1".
	tvRe = regexp.MustCompile(`(?i)(?:^|[\s\[\(\-,/])(?:s(\d{1,2})(?:\s?e\d{1,3}(?:-e?\d{1,3})?)*(?:$|[\s\]\)\-,./])|(\d{1,2})x\d{2,3}(?:$|[\s\]\)\-,./])|(?:season|сезон)[\s:]*(\d{1,2})|(?:серии|серия|episodes?)[\s:]*\d)`)

	// yearRe matches a year candidate; isolatedAt checks its surroundings, since
	// adjacent years ("2049 2017") would share a delimiter in one regexp.
	yearRe = regexp.MustCompile(`(?:19|20)\d{2}`)

	// bracketRe matches a bracketed group: [..], (..) or {..}.
	bracketRe = regexp.MustCompile(`[\[\(\{][^\[\]\(\)\{\}]*[\]\)\}]`)

	spacesRe = regexp.MustCompile(`\s+`)
)

// ParseRelease extracts the title, year and season marker from a torrent
// name such as "Interstellar.2014.1080p.BluRay.x264-SPARKS" or
// "Интерстеллар / Interstellar (Кристофер Нолан) [2014, США, BDRip 1080p]".
func ParseRelease(name string) Release {
	s := strings.TrimSpace(name)
	if IsVideoFile(s) {
		s = strings.TrimSuffix(s, path.Ext(s))
	}
	s = strings.ReplaceAll(s, "_", " ")
	// Scene names use dots instead of spaces; a name that already has spaces
	// keeps its dots ("Mr. Robot", "S.W.A.T.").
	if !strings.Contains(s, " ") {
		s = strings.ReplaceAll(s, ".", " ")
	}
	s = stripLeadingTags(s)

	var rel Release
	end := len(s)

	if loc := junkRe.FindStringIndex(s); loc != nil && loc[0] > 0 {
		end = loc[0]
	}

	if m := tvRe.FindStringSubmatchIndex(s); m != nil && m[0] > 0 {
		rel.TV = true
		for i := 2; i < len(m); i += 2 {
			if m[i] >= 0 {
				rel.Season, _ = strconv.Atoi(s[m[i]:m[i+1]])
				break
			}
		}
		if m[0] < end {
			end = m[0]
		}
	}

	titleEnd := end
	maxYear := time.Now().Year() + 1
	var yearAfterTitle int
	for _, m := range yearRe.FindAllStringIndex(s, -1) {
		year, _ := strconv.Atoi(s[m[0]:m[1]])
		if year > maxYear || m[0] == 0 || !isolatedAt(s, m[0], m[1]) {
			// A leading number is part of the title: "1917", "2012".
			continue
		}
		if m[0] < end {
			// The last year before the technical tokens is the release year:
			// "Blade.Runner.2049.2017.1080p" is Blade Runner 2049 from 2017.
			rel.Year = year
			titleEnd = m[0]
			continue
		}
		if yearAfterTitle == 0 {
			yearAfterTitle = year
		}
	}
	if rel.Year == 0 {
		rel.Year = yearAfterTitle
	}

	rel.Titles = splitTitles(s[:titleEnd])
	if len(rel.Titles) == 0 {
		// Nothing left before the markers: fall back to the whole cleaned name.
		rel.Titles = splitTitles(s)
	}
	return rel
}

// isolatedAt reports whether s[start:end] stands on its own: bounded by the
// string edges, spaces, brackets, commas or dashes.
func isolatedAt(s string, start, end int) bool {
	const delims = " []()-,"
	before := start == 0 || strings.ContainsRune(delims, rune(s[start-1]))
	after := end == len(s) || strings.ContainsRune(delims, rune(s[end]))
	return before && after
}

// stripLeadingTags drops release-site or group tags in front of the title,
// e.g. "[YTS.MX] Movie (2020)" or "[rutracker.org] ...".
func stripLeadingTags(s string) string {
	for {
		s = strings.TrimSpace(s)
		if s == "" || !strings.ContainsRune("[{", rune(s[0])) {
			return s
		}
		loc := bracketRe.FindStringIndex(s)
		if loc == nil || loc[0] != 0 || loc[1] == len(s) {
			return s
		}
		s = s[loc[1]:]
	}
}

// splitTitles cleans the title part of a name and splits it into the
// alternative titles separated by "/" or "|".
func splitTitles(s string) []string {
	// Bracketed parts of the title are directors, countries or notes.
	for {
		next := bracketRe.ReplaceAllString(s, " ")
		if next == s {
			break
		}
		s = next
	}

	var titles []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '|' }) {
		t := cleanTitle(part)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		titles = append(titles, t)
	}
	return titles
}

// cleanTitle collapses whitespace and trims punctuation left over from the
// separators around the title.
func cleanTitle(s string) string {
	s = strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ", "{", " ", "}", " ").Replace(s)
	s = spacesRe.ReplaceAllString(s, " ")
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == ':' || r == ',' || r == '.' || r == '+' || r == '~'
	})
}

// normalizeForCompare folds a title for loose equality: case, punctuation,
// "ё" and spacing are ignored.
func normalizeForCompare(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == 'ё':
			b.WriteRune('е')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '&':
			b.WriteString("and")
		}
	}
	return b.String()
}
