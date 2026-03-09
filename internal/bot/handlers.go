package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lebe-dev/transmitter/internal/transmission"
	"gopkg.in/telebot.v4"
)

const (
	maxMessageLen  = 4096
	maxTorrentName = 40
	maxFileSize    = 20 << 20 // 20 MB
)

func (b *Bot) handleStart(c telebot.Context) error {
	return c.Send("Transmitter Bot\n\nCommands:\n/add <magnet> — add torrent\n/status — list torrents\n/help — help")
}

func (b *Bot) handleHelp(c telebot.Context) error {
	return c.Send("Help\n\n/add <magnet|url> — add torrent by magnet link or URL\n/status — list active torrents\n\nYou can also send a .torrent file directly to the chat.")
}

func (b *Bot) handleAdd(c telebot.Context) error {
	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Send("Usage: /add <magnet link or URL>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	added, err := b.client.AddMagnet(ctx, args)
	if err != nil {
		b.logger.Warn("failed to add torrent", "err", err)
		return c.Send("Error: " + err.Error())
	}
	return c.Send(fmt.Sprintf("Added: %s", added.Name))
}

func (b *Bot) handleStatus(c telebot.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	torrents, err := b.client.GetTorrents(ctx)
	if err != nil {
		b.logger.Warn("failed to get torrents", "err", err)
		return c.Send("Failed to get torrent list: " + err.Error())
	}

	if len(torrents) == 0 {
		return c.Send("No active torrents.")
	}

	lines := make([]string, 0, len(torrents))
	for _, t := range torrents {
		lines = append(lines, formatTorrent(t))
	}

	chunks := splitIntoChunks(lines, maxMessageLen)
	for i, chunk := range chunks {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}
		if err := c.Send(strings.Join(chunk, "\n")); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bot) handleDocument(c telebot.Context) error {
	doc := c.Message().Document
	if doc == nil {
		return nil
	}

	if doc.MIME != "application/x-bittorrent" && !strings.HasSuffix(strings.ToLower(doc.FileName), ".torrent") {
		return c.Send("Expected a .torrent file")
	}

	file, err := b.tg.File(&doc.File)
	if err != nil {
		b.logger.Warn("failed to download file from telegram", "err", err)
		return c.Send("Failed to download file: " + err.Error())
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFileSize))
	if err != nil {
		return c.Send("Error reading file: " + err.Error())
	}

	metainfo := base64.StdEncoding.EncodeToString(data)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	added, err := b.client.AddTorrentFile(ctx, metainfo)
	if err != nil {
		b.logger.Warn("failed to add torrent file", "err", err)
		return c.Send("Error: " + err.Error())
	}
	return c.Send(fmt.Sprintf("Added: %s", added.Name))
}

// formatTorrent formats a single torrent as a text line for Telegram.
func formatTorrent(t transmission.Torrent) string {
	name := truncate(t.Name, maxTorrentName)
	bar := renderBar(t.PercentDone)
	pct := int(t.PercentDone * 100)
	label := statusLabel(t.Status)

	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] %s [%s] %d%%", label, name, bar, pct)

	if t.RateDownload > 0 {
		fmt.Fprintf(&sb, " ↓%s", formatSpeed(t.RateDownload))
	}
	if t.RateUpload > 0 {
		fmt.Fprintf(&sb, " ↑%s", formatSpeed(t.RateUpload))
	}
	if t.ETA > 0 {
		fmt.Fprintf(&sb, " ETA %s", formatETA(t.ETA))
	}
	return sb.String()
}

// splitIntoChunks groups lines into chunks that fit within maxLen characters.
func splitIntoChunks(lines []string, maxLen int) [][]string {
	var chunks [][]string
	var current []string
	currentLen := 0

	for _, line := range lines {
		lineLen := utf8.RuneCountInString(line) + 1 // +1 for newline
		if currentLen+lineLen > maxLen && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
			currentLen = 0
		}
		current = append(current, line)
		currentLen += lineLen
	}

	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// statusLabel returns a text label for the Transmission torrent status code.
func statusLabel(status int) string {
	switch status {
	case 0:
		return "paused"
	case 1, 2:
		return "checking"
	case 3, 4:
		return "downloading"
	case 5, 6:
		return "seeding"
	default:
		return "unknown"
	}
}

// renderBar renders a progress bar using block characters.
func renderBar(pct float64) string {
	const total = 8
	filled := int(math.Round(pct * total))
	if filled > total {
		filled = total
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", total-filled)
}

// formatETA formats seconds into a human-readable ETA string.
func formatETA(secs int64) string {
	if secs < 0 {
		return "∞"
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
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

// truncate shortens a string to maxLen runes, appending ellipsis if needed.
func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}
