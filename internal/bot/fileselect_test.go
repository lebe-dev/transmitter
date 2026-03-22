package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

func testFiles() []transmission.TorrentFile {
	return []transmission.TorrentFile{
		{Name: "dir/Movie.S01E01.720p.mkv", Length: 1288490188},
		{Name: "dir/Movie.S01E02.720p.mkv", Length: 1181116006},
		{Name: "dir/Sample.mkv", Length: 52428800},
		{Name: "dir/Movie.S01E03.720p.mkv", Length: 1395864371},
	}
}

func newTestState() *FileSelectState {
	files := testFiles()
	selected := make([]bool, len(files))
	for i := range selected {
		selected[i] = true
	}
	return &FileSelectState{
		TorrentID:   42,
		TorrentName: "Test Torrent Name",
		Files:       files,
		Selected:    selected,
		Page:        0,
		CreatedAt:   time.Now(),
	}
}

func TestFormatFileSelectMessage(t *testing.T) {
	state := newTestState()
	msg := formatFileSelectMessage(state)

	if !strings.Contains(msg, "Test Torrent Name") {
		t.Error("message should contain torrent name")
	}
	if !strings.Contains(msg, "✅") {
		t.Error("message should contain check marks for selected files")
	}
	if !strings.Contains(msg, "4/4") {
		t.Error("message should show 4/4 selected")
	}
	if !strings.Contains(msg, "Movie.S01E01.720p.mkv") {
		t.Error("message should contain file names")
	}
}

func TestFormatFileSelectMessageDeselected(t *testing.T) {
	state := newTestState()
	state.Selected[2] = false
	msg := formatFileSelectMessage(state)

	if !strings.Contains(msg, "⬜") {
		t.Error("message should contain unchecked mark for deselected file")
	}
	if !strings.Contains(msg, "3/4") {
		t.Error("message should show 3/4 selected")
	}
}

func TestFormatFileSelectMessagePagination(t *testing.T) {
	files := make([]transmission.TorrentFile, 20)
	selected := make([]bool, 20)
	for i := range files {
		files[i] = transmission.TorrentFile{Name: "dir/file" + string(rune('A'+i)) + ".mkv", Length: 100_000_000}
		selected[i] = true
	}
	state := &FileSelectState{
		TorrentID:   1,
		TorrentName: "Big Torrent",
		Files:       files,
		Selected:    selected,
		Page:        1,
		CreatedAt:   time.Now(),
	}

	msg := formatFileSelectMessage(state)
	// Page 1 (0-indexed) should show files 9-16
	if !strings.Contains(msg, "9.") {
		t.Error("page 1 should start with file 9")
	}
	// Files 1-8 should not appear on page 1 — check for "✅ 1." pattern
	if strings.Contains(msg, "✅ 1.") || strings.Contains(msg, "⬜ 1.") {
		t.Error("page 1 should not contain file 1")
	}
}

func TestFileSelectKeyboard(t *testing.T) {
	state := newTestState()
	kb := fileSelectKeyboard(state)

	if kb == nil {
		t.Fatal("keyboard should not be nil")
	}
	if len(kb.InlineKeyboard) == 0 {
		t.Fatal("keyboard should have rows")
	}

	// 4 file buttons + select all/none row + confirm/skip row = 6 rows
	expected := 6
	if len(kb.InlineKeyboard) != expected {
		t.Errorf("expected %d rows, got %d", expected, len(kb.InlineKeyboard))
	}
}

func TestFileSelectKeyboardPaginated(t *testing.T) {
	files := make([]transmission.TorrentFile, 20)
	selected := make([]bool, 20)
	for i := range files {
		files[i] = transmission.TorrentFile{Name: "dir/file.mkv", Length: 100}
		selected[i] = true
	}
	state := &FileSelectState{
		TorrentID:   1,
		TorrentName: "Big",
		Files:       files,
		Selected:    selected,
		Page:        0,
		CreatedAt:   time.Now(),
	}

	kb := fileSelectKeyboard(state)
	// 8 file buttons + nav row + select row + confirm row = 11 rows
	expected := 11
	if len(kb.InlineKeyboard) != expected {
		t.Errorf("expected %d rows, got %d", expected, len(kb.InlineKeyboard))
	}
}

func TestSelectedCount(t *testing.T) {
	state := newTestState()

	count, size := selectedCount(state)
	if count != 4 {
		t.Errorf("expected 4 selected, got %d", count)
	}
	if size != 1288490188+1181116006+52428800+1395864371 {
		t.Errorf("unexpected total size: %d", size)
	}

	state.Selected[2] = false
	count, size = selectedCount(state)
	if count != 3 {
		t.Errorf("expected 3 selected, got %d", count)
	}
	if size != 1288490188+1181116006+1395864371 {
		t.Errorf("unexpected total size after deselect: %d", size)
	}
}

func TestSelectedCountNone(t *testing.T) {
	state := newTestState()
	for i := range state.Selected {
		state.Selected[i] = false
	}

	count, _ := selectedCount(state)
	if count != 0 {
		t.Errorf("expected 0 selected, got %d", count)
	}
}

func TestFileSelectPages(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{0, 1},
		{1, 1},
		{8, 1},
		{9, 2},
		{16, 2},
		{17, 3},
	}
	for _, tt := range tests {
		got := fileSelectPages(tt.count)
		if got != tt.expected {
			t.Errorf("fileSelectPages(%d) = %d, want %d", tt.count, got, tt.expected)
		}
	}
}

func TestParseTwoInts(t *testing.T) {
	a, b, err := parseTwoInts("ft:42:7", "ft:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != 42 || b != 7 {
		t.Errorf("expected 42, 7; got %d, %d", a, b)
	}

	_, _, err = parseTwoInts("ft:bad", "ft:")
	if err == nil {
		t.Error("expected error for bad input")
	}

	_, _, err = parseTwoInts("ft:1:bad", "ft:")
	if err == nil {
		t.Error("expected error for bad second part")
	}
}

func TestTotalSize(t *testing.T) {
	files := testFiles()
	size := totalSize(files)
	expected := int64(1288490188 + 1181116006 + 52428800 + 1395864371)
	if size != expected {
		t.Errorf("totalSize = %d, want %d", size, expected)
	}
}
