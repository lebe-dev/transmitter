package bot

import (
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
)

const (
	torrentsPerPage = 8
	barWidth        = 8
	maxTorrentName  = 40
)

// truncate shortens a string to maxLen runes, appending ellipsis if needed.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// formatSpeed formats bytes/s into a human-readable speed string.
func formatSpeed(bps int64) string {
	switch {
	case bps >= 1<<20:
		return fmt.Sprintf("%.1fMB/s", float64(bps)/(1<<20))
	case bps >= 1<<10:
		return fmt.Sprintf("%.1fKB/s", float64(bps)/(1<<10))
	default:
		return fmt.Sprintf("%dB/s", bps)
	}
}

// formatETA formats seconds into a human-readable ETA string.
func formatETA(secs int64) string {
	if secs < 0 {
		return "∞"
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// statusGroup categorizes a torrent into downloading, seeding, or paused.
func statusGroup(t transmission.Torrent) string {
	switch t.Status {
	case 0:
		return "paused"
	case 1, 2, 3, 4:
		return "downloading"
	case 5, 6:
		return "seeding"
	default:
		return "downloading"
	}
}

type groupedTorrents struct {
	downloading []transmission.Torrent
	seeding     []transmission.Torrent
	paused      []transmission.Torrent
}

func groupTorrents(torrents []transmission.Torrent) groupedTorrents {
	var g groupedTorrents
	for _, t := range torrents {
		switch statusGroup(t) {
		case "downloading":
			g.downloading = append(g.downloading, t)
		case "seeding":
			g.seeding = append(g.seeding, t)
		case "paused":
			g.paused = append(g.paused, t)
		}
	}

	sort.Slice(g.downloading, func(i, j int) bool {
		return g.downloading[i].PercentDone < g.downloading[j].PercentDone
	})
	sort.Slice(g.seeding, func(i, j int) bool {
		return g.seeding[i].RateUpload > g.seeding[j].RateUpload
	})
	sort.Slice(g.paused, func(i, j int) bool {
		return g.paused[i].PercentDone > g.paused[j].PercentDone
	})

	return g
}

// allSorted returns all torrents in display order: downloading, seeding, paused.
func (g groupedTorrents) allSorted() []transmission.Torrent {
	result := make([]transmission.Torrent, 0, len(g.downloading)+len(g.seeding)+len(g.paused))
	result = append(result, g.downloading...)
	result = append(result, g.seeding...)
	result = append(result, g.paused...)
	return result
}

// totalPages returns the total number of pages for pagination.
func totalPages(count int) int {
	if count == 0 {
		return 1
	}
	return int(math.Ceil(float64(count) / float64(torrentsPerPage)))
}

// formatStatusPage formats the torrent list for a given page (0-indexed).
func formatStatusPage(torrents []transmission.Torrent, page int) string {
	if len(torrents) == 0 {
		return "No active torrents."
	}

	g := groupTorrents(torrents)
	all := g.allSorted()

	var totalDown, totalUp int64
	for _, t := range torrents {
		totalDown += t.RateDownload
		totalUp += t.RateUpload
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Torrents: %d</b>", len(torrents)))
	if totalDown > 0 || totalUp > 0 {
		sb.WriteString("  |")
		if totalDown > 0 {
			sb.WriteString(fmt.Sprintf("  ↓%s", formatSpeed(totalDown)))
		}
		if totalUp > 0 {
			sb.WriteString(fmt.Sprintf("  ↑%s", formatSpeed(totalUp)))
		}
	}
	sb.WriteString("\n")

	pages := totalPages(len(all))
	if page >= pages {
		page = pages - 1
	}
	if page < 0 {
		page = 0
	}

	start := page * torrentsPerPage
	end := start + torrentsPerPage
	if end > len(all) {
		end = len(all)
	}
	pageItems := all[start:end]

	// Determine which groups appear on this page
	type groupInfo struct {
		emoji string
		label string
		items []transmission.Torrent
	}

	var groups []groupInfo
	if len(g.downloading) > 0 {
		groups = append(groups, groupInfo{"📥", "Downloading", g.downloading})
	}
	if len(g.seeding) > 0 {
		groups = append(groups, groupInfo{"🌱", "Seeding", g.seeding})
	}
	if len(g.paused) > 0 {
		groups = append(groups, groupInfo{"⏸", "Paused", g.paused})
	}

	// Track position in the flat list
	num := start + 1
	for _, gi := range groups {
		// Check if any items from this group appear in the page
		var pageGroupItems []transmission.Torrent
		for _, item := range pageItems {
			if statusGroup(item) == statusGroup(gi.items[0]) {
				pageGroupItems = append(pageGroupItems, item)
			}
		}
		if len(pageGroupItems) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n%s <b>%s (%d)</b>\n", gi.emoji, gi.label, len(gi.items)))
		for _, t := range pageGroupItems {
			sb.WriteString(formatTorrentLine(t, num))
			num++
		}
	}

	if pages > 1 {
		sb.WriteString(fmt.Sprintf("\n📄 Page %d/%d", page+1, pages))
	}

	return sb.String()
}

func formatTorrentLine(t transmission.Torrent, num int) string {
	name := html.EscapeString(truncate(t.Name, maxTorrentName))
	group := statusGroup(t)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d. <b>%s</b>\n", num, name))

	switch group {
	case "downloading":
		pct := int(t.PercentDone * 100)
		bar := renderProgressBar(t.PercentDone)
		sb.WriteString(fmt.Sprintf("   %d%% %s", pct, bar))
		if t.RateDownload > 0 {
			sb.WriteString(fmt.Sprintf("  ↓%s", formatSpeed(t.RateDownload)))
		}
		if t.ETA > 0 {
			sb.WriteString(fmt.Sprintf("  ETA %s", formatETA(t.ETA)))
		}
		sb.WriteString("\n")
	case "seeding":
		if t.RateUpload > 0 {
			sb.WriteString(fmt.Sprintf("   ↑%s\n", formatSpeed(t.RateUpload)))
		}
	case "paused":
		pct := int(t.PercentDone * 100)
		sb.WriteString(fmt.Sprintf("   %d%%\n", pct))
	}

	return sb.String()
}

// renderProgressBar renders a progress bar using Unicode characters.
func renderProgressBar(pct float64) string {
	filled := int(math.Round(pct * barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	return strings.Repeat("━", filled) + strings.Repeat("░", barWidth-filled)
}

// formatTorrentDetail formats a single torrent for the detail view.
func formatTorrentDetail(t transmission.Torrent) string {
	name := html.EscapeString(t.Name)
	group := statusGroup(t)

	var statusEmoji, statusLabel string
	switch group {
	case "downloading":
		statusEmoji = "📥"
		statusLabel = "Downloading"
	case "seeding":
		statusEmoji = "🌱"
		statusLabel = "Seeding"
	case "paused":
		statusEmoji = "⏸"
		statusLabel = "Paused"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📄 <b>%s</b>\n\n", name))
	sb.WriteString(fmt.Sprintf("Status: %s %s\n", statusEmoji, statusLabel))

	pct := int(t.PercentDone * 100)
	bar := renderProgressBar(t.PercentDone)
	sb.WriteString(fmt.Sprintf("Progress: %d%% %s\n", pct, bar))
	sb.WriteString(fmt.Sprintf("Size: %s\n", formatSize(t.TotalSize)))

	if t.RateDownload > 0 || t.RateUpload > 0 {
		sb.WriteString("Speed:")
		if t.RateDownload > 0 {
			sb.WriteString(fmt.Sprintf(" ↓%s", formatSpeed(t.RateDownload)))
		}
		if t.RateUpload > 0 {
			sb.WriteString(fmt.Sprintf(" ↑%s", formatSpeed(t.RateUpload)))
		}
		sb.WriteString("\n")
	}

	if t.ETA > 0 {
		sb.WriteString(fmt.Sprintf("ETA: %s\n", formatETA(t.ETA)))
	}

	if t.AddedDate > 0 {
		sb.WriteString(fmt.Sprintf("Added: %s\n", formatDate(t.AddedDate)))
	}

	return sb.String()
}

// formatSize formats bytes into a human-readable size string.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// formatDate formats a Unix timestamp into YYYY-MM-DD.
func formatDate(ts int64) string {
	if ts <= 0 {
		return "unknown"
	}
	return time.Unix(ts, 0).Format("2006-01-02")
}
