package bot

import (
	"context"
	"fmt"
	"html"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
	"gopkg.in/telebot.v4"
)

const (
	filesPerPage = 8
)

// FileSelectState holds the state of a file selection dialog.
type FileSelectState struct {
	TorrentID   int64
	TorrentName string
	Files       []transmission.TorrentFile
	Selected    []bool
	Page        int
	CreatedAt   time.Time
}

func (b *Bot) getFileSelectState(torrentID int64) *FileSelectState {
	b.fileSelectMu.Lock()
	defer b.fileSelectMu.Unlock()
	return b.fileSelect[torrentID]
}

func (b *Bot) setFileSelectState(torrentID int64, state *FileSelectState) {
	b.fileSelectMu.Lock()
	defer b.fileSelectMu.Unlock()
	b.fileSelect[torrentID] = state
}

func (b *Bot) deleteFileSelectState(torrentID int64) {
	b.fileSelectMu.Lock()
	defer b.fileSelectMu.Unlock()
	delete(b.fileSelect, torrentID)
}

// cleanupStaleFileSelectStates removes expired file select states and deletes orphaned torrents.
func (b *Bot) cleanupStaleFileSelectStates() {
	b.fileSelectMu.Lock()
	var stale []int64
	cutoff := time.Now().Add(-b.fileSelectTimeout)
	for id, state := range b.fileSelect {
		if state.CreatedAt.Before(cutoff) {
			stale = append(stale, id)
		}
	}
	for _, id := range stale {
		delete(b.fileSelect, id)
	}
	b.fileSelectMu.Unlock()

	for _, id := range stale {
		b.logger.Warn("file select state expired, removing torrent", "torrent_id", id)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := b.client.RemoveTorrents(ctx, []int64{id}, true); err != nil {
			b.logger.Warn("failed to remove expired torrent", "torrent_id", id, "err", err)
		}
		cancel()
	}
}

// showFileSelectDialog sends the file selection dialog message.
func (b *Bot) showFileSelectDialog(c telebot.Context, torrentID int64, name string, files []transmission.TorrentFile) error {
	state := &FileSelectState{
		TorrentID:   torrentID,
		TorrentName: name,
		Files:       files,
		Selected:    make([]bool, len(files)),
		Page:        0,
		CreatedAt:   time.Now(),
	}
	for i := range state.Selected {
		state.Selected[i] = true
	}

	b.setFileSelectState(torrentID, state)

	text := formatFileSelectMessage(state)
	kb := fileSelectKeyboard(state)
	return c.Send(text, telebot.ModeHTML, kb)
}

// selectedCount returns the number of selected files and their total size.
func selectedCount(state *FileSelectState) (int, int64) {
	count := 0
	var size int64
	for i, sel := range state.Selected {
		if sel {
			count++
			size += state.Files[i].Length
		}
	}
	return count, size
}

func totalSize(files []transmission.TorrentFile) int64 {
	var size int64
	for _, f := range files {
		size += f.Length
	}
	return size
}

func formatFileSelectMessage(state *FileSelectState) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📂 <b>%s</b>\n\n", html.EscapeString(truncate(state.TorrentName, maxTorrentName))))

	pages := fileSelectPages(len(state.Files))
	page := state.Page
	if page >= pages {
		page = pages - 1
	}

	start := page * filesPerPage
	end := start + filesPerPage
	if end > len(state.Files) {
		end = len(state.Files)
	}

	for i := start; i < end; i++ {
		f := state.Files[i]
		check := "⬜"
		if state.Selected[i] {
			check = "✅"
		}
		name := filepath.Base(f.Name)
		sb.WriteString(fmt.Sprintf("%s %d. %s — %s\n\n", check, i+1, html.EscapeString(name), formatSize(f.Length)))
	}

	selCount, selSize := selectedCount(state)
	sb.WriteString(fmt.Sprintf("\nSelected: %d/%d (%s / %s)", selCount, len(state.Files), formatSize(selSize), formatSize(totalSize(state.Files))))

	return sb.String()
}

func fileSelectKeyboard(state *FileSelectState) *telebot.ReplyMarkup {
	rm := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	pages := fileSelectPages(len(state.Files))
	page := state.Page
	if page >= pages {
		page = pages - 1
	}

	start := page * filesPerPage
	end := start + filesPerPage
	if end > len(state.Files) {
		end = len(state.Files)
	}

	for i := start; i < end; i++ {
		f := state.Files[i]
		check := "⬜"
		if state.Selected[i] {
			check = "✅"
		}
		name := filepath.Base(f.Name)
		label := fmt.Sprintf("%s %s (%s)", check, truncate(name, 25), formatSize(f.Length))
		btn := rm.Data(label, fmt.Sprintf("ft_%d_%d", state.TorrentID, i), fmt.Sprintf("ft:%d:%d", state.TorrentID, i))
		rows = append(rows, rm.Row(btn))
	}

	if pages > 1 {
		var navBtns telebot.Row
		if page > 0 {
			navBtns = append(navBtns, rm.Data("◀", "fpp", fmt.Sprintf("fp:%d:%d", state.TorrentID, page-1)))
		}
		navBtns = append(navBtns, rm.Data(fmt.Sprintf("%d/%d", page+1, pages), "fnoop", "noop"))
		if page < pages-1 {
			navBtns = append(navBtns, rm.Data("▶", "fpn", fmt.Sprintf("fp:%d:%d", state.TorrentID, page+1)))
		}
		rows = append(rows, navBtns)
	}

	rows = append(rows, telebot.Row{
		rm.Data("✅ All", "fa", fmt.Sprintf("fa:%d", state.TorrentID)),
		rm.Data("⬜ None", "fn", fmt.Sprintf("fn:%d", state.TorrentID)),
	})

	selCount, _ := selectedCount(state)
	confirmLabel := fmt.Sprintf("📥 Download (%d)", selCount)
	rows = append(rows, telebot.Row{
		rm.Data(confirmLabel, "fc", fmt.Sprintf("fc:%d", state.TorrentID)),
		rm.Data("⏭ All files", "fk", fmt.Sprintf("fk:%d", state.TorrentID)),
	})

	rm.Inline(rows...)
	return rm
}

func fileSelectPages(count int) int {
	if count == 0 {
		return 1
	}
	return int(math.Ceil(float64(count) / float64(filesPerPage)))
}
