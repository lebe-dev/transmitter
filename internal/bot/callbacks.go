package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/telebot.v4"
)

func (b *Bot) handleCallback(c telebot.Context) error {
	data := c.Data()

	// Strip any prefix added by telebot (e.g. "d|d:42" → "d:42")
	if idx := strings.LastIndex(data, "|"); idx >= 0 {
		data = strings.TrimSpace(data[idx+1:])
	}

	switch {
	case strings.HasPrefix(data, "d:"):
		return b.callbackDetail(c, data)
	case strings.HasPrefix(data, "p:"):
		return b.callbackPause(c, data)
	case strings.HasPrefix(data, "r:"):
		return b.callbackResume(c, data)
	case strings.HasPrefix(data, "x:"):
		return b.callbackDeletePrompt(c, data)
	case strings.HasPrefix(data, "xk:"):
		return b.callbackDelete(c, data, false)
	case strings.HasPrefix(data, "xd:"):
		return b.callbackDelete(c, data, true)
	case data == "c":
		return b.callbackCancel(c)
	case strings.HasPrefix(data, "s:"):
		return b.callbackStatusPage(c, data)
	case strings.HasPrefix(data, "b:"):
		return b.callbackBackToList(c, data)
	case data == "noop":
		return c.Respond()
	default:
		return c.Respond(&telebot.CallbackResponse{Text: "Unknown action"})
	}
}

func (b *Bot) callbackDetail(c telebot.Context, data string) error {
	id, err := parseID(data, "d:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid torrent ID", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	torrent, err := b.client.GetTorrent(ctx, id)
	if err != nil {
		_ = c.Respond(&telebot.CallbackResponse{Text: "Torrent not found", ShowAlert: true})
		return c.Delete()
	}

	text := formatTorrentDetail(torrent)
	kb := detailKeyboard(torrent, 0)

	_ = c.Respond()
	return c.Edit(text, kb, telebot.ModeHTML)
}

func (b *Bot) callbackPause(c telebot.Context, data string) error {
	id, err := parseID(data, "p:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid ID", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.client.StopTorrents(ctx, []int64{id}); err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: "⏸ Paused"})

	torrent, err := b.client.GetTorrent(ctx, id)
	if err != nil {
		return nil
	}
	text := formatTorrentDetail(torrent)
	kb := detailKeyboard(torrent, 0)
	return c.Edit(text, kb, telebot.ModeHTML)
}

func (b *Bot) callbackResume(c telebot.Context, data string) error {
	id, err := parseID(data, "r:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid ID", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.client.StartTorrents(ctx, []int64{id}); err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: "▶ Resumed"})

	torrent, err := b.client.GetTorrent(ctx, id)
	if err != nil {
		return nil
	}
	text := formatTorrentDetail(torrent)
	kb := detailKeyboard(torrent, 0)
	return c.Edit(text, kb, telebot.ModeHTML)
}

func (b *Bot) callbackDeletePrompt(c telebot.Context, data string) error {
	id, err := parseID(data, "x:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid ID", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	torrent, err := b.client.GetTorrent(ctx, id)
	if err != nil {
		_ = c.Respond(&telebot.CallbackResponse{Text: "Torrent not found", ShowAlert: true})
		return c.Delete()
	}

	text := fmt.Sprintf("🗑 Delete <b>%s</b>?", truncate(torrent.Name, maxTorrentName))
	kb := deleteConfirmKeyboard(id)

	_ = c.Respond()
	return c.Edit(text, kb, telebot.ModeHTML)
}

func (b *Bot) callbackDelete(c telebot.Context, data string, deleteFiles bool) error {
	prefix := "xk:"
	if deleteFiles {
		prefix = "xd:"
	}
	id, err := parseID(data, prefix)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid ID", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get name before deleting
	name := "torrent"
	if t, err := b.client.GetTorrent(ctx, id); err == nil {
		name = truncate(t.Name, maxTorrentName)
	}

	if err := b.client.RemoveTorrents(ctx, []int64{id}, deleteFiles); err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: "🗑 Deleted"})
	return c.Edit(fmt.Sprintf("🗑 Deleted: %s", name))
}

func (b *Bot) callbackCancel(c telebot.Context) error {
	_ = c.Respond()
	return c.Delete()
}

func (b *Bot) callbackStatusPage(c telebot.Context, data string) error {
	page, err := parseID(data, "s:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid page", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	torrents, err := b.client.GetTorrents(ctx)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	text := formatStatusPage(torrents, int(page))
	kb := statusPageKeyboard(torrents, int(page))

	_ = c.Respond()
	return c.Edit(text, kb, telebot.ModeHTML)
}

func (b *Bot) callbackBackToList(c telebot.Context, data string) error {
	page, err := parseID(data, "b:")
	if err != nil {
		page = 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	torrents, err := b.client.GetTorrents(ctx)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	text := formatStatusPage(torrents, int(page))
	kb := statusPageKeyboard(torrents, int(page))

	_ = c.Respond()
	return c.Edit(text, kb, telebot.ModeHTML)
}

func parseID(data, prefix string) (int64, error) {
	s := strings.TrimPrefix(data, prefix)
	return strconv.ParseInt(s, 10, 64)
}
