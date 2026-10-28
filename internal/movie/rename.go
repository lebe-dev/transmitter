package movie

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// DefaultRenameTemplate gives names such as "Interstellar_(2014)": the English
// title whatever MOVIE_LANGUAGE is, with spaces turned into underscores.
const DefaultRenameTemplate = "{english_title} ({year})"

// maxNameBytes keeps a new name within the 255-byte limit of common file
// systems, leaving room for the extension of a single-file torrent.
const maxNameBytes = 200

var (
	// emptyGroupRe matches brackets left empty by a placeholder without a value.
	emptyGroupRe = regexp.MustCompile(`\(\s*\)|\[\s*\]|\{\s*\}`)
	// forbiddenRe matches characters not allowed in file names on Windows or
	// SMB shares; the downloads are often served from there.
	forbiddenRe = regexp.MustCompile(`[<>:"/\\|?*]`)
)

// RenderName fills the template with the matched title. Placeholders:
// {title}, {original_title}, {english_title}, {year}, {season}, {tmdb_id},
// {imdb_id}, {type}. {english_title} falls back to the original title, then to
// the localized one, when TMDB knows no English title.
// A season pack of a series gets "_S02" appended when the template has no
// {season}, so packs of one show do not collide on the same name.
// The result is safe to use as a single path component, with underscores in
// place of spaces; it is empty when the template yields nothing usable.
func RenderName(template string, m Match) string {
	if strings.TrimSpace(template) == "" {
		template = DefaultRenameTemplate
	}
	year, tmdbID, season := "", "", ""
	if m.Year > 0 {
		year = strconv.Itoa(m.Year)
	}
	if m.MediaType == MediaTV && m.Season > 0 {
		season = fmt.Sprintf("S%02d", m.Season)
		if !strings.Contains(template, "{season}") {
			template += " {season}"
		}
	}
	if m.TMDBID > 0 {
		tmdbID = strconv.FormatInt(m.TMDBID, 10)
	}
	original := m.OriginalTitle
	if original == "" {
		original = m.Title
	}
	english := m.EnglishTitle
	if english == "" {
		english = original
	}

	name := strings.NewReplacer(
		"{title}", m.Title,
		"{original_title}", original,
		"{english_title}", english,
		"{year}", year,
		"{season}", season,
		"{tmdb_id}", tmdbID,
		"{imdb_id}", m.IMDbID,
		"{type}", m.MediaType,
	).Replace(template)
	return sanitizeName(name)
}

// sanitizeName turns text into a single, portable path component. Spaces
// become underscores, so the name needs no quoting in a shell.
func sanitizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = forbiddenRe.ReplaceAllString(name, " ")
	for {
		next := emptyGroupRe.ReplaceAllString(name, "")
		if next == name {
			break
		}
		name = next
	}
	name = spacesRe.ReplaceAllString(name, " ")
	// Windows drops trailing dots and spaces, which would make the name differ
	// from what Transmission stored.
	name = strings.Trim(name, " .-")
	if name == "." || name == ".." {
		return ""
	}
	for len(name) > maxNameBytes {
		r := []rune(name)
		name = strings.TrimRight(string(r[:len(r)-1]), " .-")
	}
	return strings.ReplaceAll(name, " ", "_")
}

// TargetName is the new top-level name of a torrent: the rendered name for a
// folder, or the rendered name plus the original extension for a single file.
func TargetName(rendered, currentName string, singleFile bool) string {
	if rendered == "" {
		return ""
	}
	if singleFile {
		if ext := path.Ext(currentName); ext != "" && len(ext) <= 6 {
			return rendered + ext
		}
	}
	return rendered
}
