package bot

import (
	"fmt"

	"github.com/lebe-dev/transmitter/internal/transmission"
	"gopkg.in/telebot.v4"
)

// statusPageKeyboard builds the inline keyboard for the status page.
func statusPageKeyboard(torrents []transmission.Torrent, page int) *telebot.ReplyMarkup {
	g := groupTorrents(torrents)
	all := g.allSorted()

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

	rm := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	for _, t := range all[start:end] {
		name := truncate(t.Name, 35)
		btn := rm.Data(name, fmt.Sprintf("d_%d", t.ID), fmt.Sprintf("d:%d", t.ID))
		rows = append(rows, rm.Row(btn))
	}

	if pages > 1 {
		var navBtns telebot.Row
		if page > 0 {
			navBtns = append(navBtns, rm.Data("◀ Prev", "sp", fmt.Sprintf("s:%d", page-1)))
		}
		navBtns = append(navBtns, rm.Data(fmt.Sprintf("%d/%d", page+1, pages), "noop", "noop"))
		if page < pages-1 {
			navBtns = append(navBtns, rm.Data("Next ▶", "sn", fmt.Sprintf("s:%d", page+1)))
		}
		rows = append(rows, navBtns)
	}

	rm.Inline(rows...)
	return rm
}

// detailKeyboard builds the inline keyboard for the torrent detail view.
func detailKeyboard(t transmission.Torrent, page int) *telebot.ReplyMarkup {
	rm := &telebot.ReplyMarkup{}

	var actionBtns telebot.Row
	if t.Status == 0 {
		actionBtns = append(actionBtns, rm.Data("▶ Resume", "r", fmt.Sprintf("r:%d", t.ID)))
	} else {
		actionBtns = append(actionBtns, rm.Data("⏸ Pause", "p", fmt.Sprintf("p:%d", t.ID)))
	}
	actionBtns = append(actionBtns, rm.Data("🗑 Delete", "x", fmt.Sprintf("x:%d", t.ID)))

	backBtn := rm.Data("← Back to list", "b", fmt.Sprintf("b:%d", page))

	rm.Inline(actionBtns, rm.Row(backBtn))
	return rm
}

// deleteConfirmKeyboard builds the inline keyboard for delete confirmation.
func deleteConfirmKeyboard(id int64) *telebot.ReplyMarkup {
	rm := &telebot.ReplyMarkup{}

	rm.Inline(
		rm.Row(rm.Data("🗑 Delete torrent only", "xk", fmt.Sprintf("xk:%d", id))),
		rm.Row(rm.Data("🗑 Delete with files", "xd", fmt.Sprintf("xd:%d", id))),
		rm.Row(rm.Data("Cancel", "c", "c")),
	)
	return rm
}
