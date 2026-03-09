package bot

import (
	"strings"
	"testing"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		bps  int64
		want string
	}{
		{0, "0B/s"},
		{512, "512B/s"},
		{1024, "1.0KB/s"},
		{1536, "1.5KB/s"},
		{1048576, "1.0MB/s"},
		{5242880, "5.0MB/s"},
	}
	for _, tt := range tests {
		got := formatSpeed(tt.bps)
		if got != tt.want {
			t.Errorf("formatSpeed(%d) = %q, want %q", tt.bps, got, tt.want)
		}
	}
}

func TestFormatETA(t *testing.T) {
	tests := []struct {
		secs int64
		want string
	}{
		{-1, "∞"},
		{0, "0m"},
		{59, "0m"},
		{60, "1m"},
		{3600, "1h 00m"},
		{3660, "1h 01m"},
		{18120, "5h 02m"},
	}
	for _, tt := range tests {
		got := formatETA(tt.secs)
		if got != tt.want {
			t.Errorf("formatETA(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("a very long name here", 10); got != "a very lo…" {
		t.Errorf("truncate long = %q", got)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{3865470566, "3.6 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestGroupTorrents(t *testing.T) {
	torrents := []transmission.Torrent{
		{ID: 1, Status: 4, PercentDone: 0.5},  // downloading
		{ID: 2, Status: 6, RateUpload: 1000},  // seeding
		{ID: 3, Status: 0, PercentDone: 1.0},  // paused
		{ID: 4, Status: 4, PercentDone: 0.2},  // downloading
		{ID: 5, Status: 6, RateUpload: 5000},  // seeding
		{ID: 6, Status: 0, PercentDone: 0.45}, // paused
	}

	g := groupTorrents(torrents)

	if len(g.downloading) != 2 {
		t.Fatalf("downloading count = %d, want 2", len(g.downloading))
	}
	// Downloading sorted by % asc
	if g.downloading[0].ID != 4 {
		t.Errorf("downloading[0].ID = %d, want 4 (lowest %%)", g.downloading[0].ID)
	}

	if len(g.seeding) != 2 {
		t.Fatalf("seeding count = %d, want 2", len(g.seeding))
	}
	// Seeding sorted by upload speed desc
	if g.seeding[0].ID != 5 {
		t.Errorf("seeding[0].ID = %d, want 5 (highest upload)", g.seeding[0].ID)
	}

	if len(g.paused) != 2 {
		t.Fatalf("paused count = %d, want 2", len(g.paused))
	}
	// Paused sorted by % desc
	if g.paused[0].ID != 3 {
		t.Errorf("paused[0].ID = %d, want 3 (highest %%)", g.paused[0].ID)
	}
}

func TestFormatStatusPageEmpty(t *testing.T) {
	got := formatStatusPage(nil, 0)
	if got != "No active torrents." {
		t.Errorf("unexpected output for empty: %q", got)
	}
}

func TestFormatStatusPageContent(t *testing.T) {
	torrents := []transmission.Torrent{
		{ID: 1, Name: "Ubuntu.22.04", Status: 4, PercentDone: 0.32, RateDownload: 4300000, ETA: 18120},
		{ID: 2, Name: "Arch.Linux", Status: 6, RateUpload: 1100000},
		{ID: 3, Name: "Old.Movie", Status: 0, PercentDone: 1.0},
	}

	got := formatStatusPage(torrents, 0)

	// Check header
	if !strings.Contains(got, "<b>Torrents: 3</b>") {
		t.Error("missing torrent count header")
	}

	// Check group headers
	if !strings.Contains(got, "📥 <b>Downloading (1)</b>") {
		t.Error("missing downloading header")
	}
	if !strings.Contains(got, "🌱 <b>Seeding (1)</b>") {
		t.Error("missing seeding header")
	}
	if !strings.Contains(got, "⏸ <b>Paused (1)</b>") {
		t.Error("missing paused header")
	}

	// Check torrent names are HTML-escaped and bold
	if !strings.Contains(got, "<b>Ubuntu.22.04</b>") {
		t.Error("missing torrent name")
	}

	// Check progress info for downloading
	if !strings.Contains(got, "32%") {
		t.Error("missing percent")
	}
	if !strings.Contains(got, "━") {
		t.Error("missing progress bar")
	}
}

func TestFormatStatusPagePagination(t *testing.T) {
	var torrents []transmission.Torrent
	for i := 0; i < 20; i++ {
		torrents = append(torrents, transmission.Torrent{
			ID: int64(i + 1), Name: "Torrent", Status: 4, PercentDone: float64(i) / 20,
		})
	}

	page0 := formatStatusPage(torrents, 0)
	if !strings.Contains(page0, "Page 1/") {
		t.Error("missing page indicator on page 0")
	}

	page2 := formatStatusPage(torrents, 2)
	if !strings.Contains(page2, "Page 3/") {
		t.Error("missing page indicator on page 2")
	}
}

func TestTotalPages(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{0, 1},
		{1, 1},
		{8, 1},
		{9, 2},
		{16, 2},
		{17, 3},
	}
	for _, tt := range tests {
		got := totalPages(tt.count)
		if got != tt.want {
			t.Errorf("totalPages(%d) = %d, want %d", tt.count, got, tt.want)
		}
	}
}

func TestFormatTorrentDetail(t *testing.T) {
	torrent := transmission.Torrent{
		ID:           42,
		Name:         "Ubuntu.22.04.Desktop.amd64.iso",
		Status:       4,
		PercentDone:  0.75,
		TotalSize:    3865470566,
		RateDownload: 5452595,
		RateUpload:   314573, // ~307 KB/s
		ETA:          8100,
		AddedDate:    1705276800,
	}

	got := formatTorrentDetail(torrent)

	checks := []string{
		"<b>Ubuntu.22.04.Desktop.amd64.iso</b>",
		"📥 Downloading",
		"75%",
		"━",
		"3.6 GB",
		"↓5.2MB/s",
		"↑307.2KB/s",
		"2h 15m",
	}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Errorf("detail missing %q in:\n%s", check, got)
		}
	}
}

func TestRenderProgressBar(t *testing.T) {
	bar0 := renderProgressBar(0)
	if bar0 != "░░░░░░░░" {
		t.Errorf("0%% bar = %q", bar0)
	}

	bar100 := renderProgressBar(1.0)
	if bar100 != "━━━━━━━━" {
		t.Errorf("100%% bar = %q", bar100)
	}

	bar50 := renderProgressBar(0.5)
	if len([]rune(bar50)) != barWidth {
		t.Errorf("50%% bar wrong width: %q", bar50)
	}
}

func TestHTMLEscapeInName(t *testing.T) {
	torrent := transmission.Torrent{
		ID: 1, Name: "Test <script>alert('xss')</script>", Status: 4,
	}
	got := formatTorrentDetail(torrent)
	if strings.Contains(got, "<script>") {
		t.Error("HTML not escaped in torrent name")
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Error("expected escaped HTML in torrent name")
	}
}
