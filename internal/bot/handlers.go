package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"gopkg.in/telebot.v4"
)

const maxFileSize = 20 << 20 // 20 MB

func (b *Bot) handleStart(c telebot.Context) error {
	text := `<b>Transmitter Bot</b>

/add <code>&lt;magnet|url&gt;</code> — add torrent
/status — list active torrents
/status_all — list all torrents
/help — help

You can also send a <b>.torrent</b> file directly.`
	return c.Send(text, telebot.ModeHTML)
}

func (b *Bot) handleHelp(c telebot.Context) error {
	text := `<b>Help</b>

/add <code>&lt;magnet|url&gt;</code> — add torrent by magnet link or URL
/status — list active torrents (downloading, seeding)
/status_all — list all torrents including stopped

You can also send a <b>.torrent</b> file directly to the chat.`
	return c.Send(text, telebot.ModeHTML)
}

func (b *Bot) handleAdd(c telebot.Context) error {
	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Send("Usage: /add <code>&lt;magnet link or URL&gt;</code>", telebot.ModeHTML)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	added, err := b.client.AddMagnet(ctx, args)
	if err != nil {
		b.logger.Warn("failed to add torrent", "err", err)
		if strings.Contains(err.Error(), "duplicate") {
			return c.Send(fmt.Sprintf("⚠️ Already exists: <b>%s</b>", html.EscapeString(added.Name)), telebot.ModeHTML)
		}
		return c.Send("Error: "+html.EscapeString(err.Error()), telebot.ModeHTML)
	}

	b.applyAutoPriority(ctx, added.ID)

	rm := &telebot.ReplyMarkup{}
	rm.Inline(rm.Row(rm.Data("📋 View Status", "vs", "s:0")))

	return c.Send(
		fmt.Sprintf("✅ Added: <b>%s</b>", html.EscapeString(added.Name)),
		telebot.ModeHTML, rm,
	)
}

func (b *Bot) handleStatus(c telebot.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	all, err := b.client.GetTorrents(ctx)
	if err != nil {
		b.logger.Warn("failed to get torrents", "err", err)
		return c.Send("Failed to get torrent list: " + err.Error())
	}

	torrents := filterActive(all)
	if len(torrents) == 0 {
		return c.Send("No active torrents.")
	}

	text := formatStatusPage(torrents, 0)
	kb := statusPageKeyboard(torrents, 0, false)

	return c.Send(text, telebot.ModeHTML, kb)
}

func (b *Bot) handleStatusAll(c telebot.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	torrents, err := b.client.GetTorrents(ctx)
	if err != nil {
		b.logger.Warn("failed to get torrents", "err", err)
		return c.Send("Failed to get torrent list: " + err.Error())
	}

	if len(torrents) == 0 {
		return c.Send("No torrents.")
	}

	text := formatStatusPage(torrents, 0)
	kb := statusPageKeyboard(torrents, 0, true)

	return c.Send(text, telebot.ModeHTML, kb)
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
		if strings.Contains(err.Error(), "duplicate") {
			return c.Send(fmt.Sprintf("⚠️ Already exists: <b>%s</b>", html.EscapeString(added.Name)), telebot.ModeHTML)
		}
		return c.Send("Error: "+html.EscapeString(err.Error()), telebot.ModeHTML)
	}

	b.applyAutoPriority(ctx, added.ID)

	rm := &telebot.ReplyMarkup{}
	rm.Inline(rm.Row(rm.Data("📋 View Status", "vs", "s:0")))

	return c.Send(
		fmt.Sprintf("✅ Added: <b>%s</b>", html.EscapeString(added.Name)),
		telebot.ModeHTML, rm,
	)
}

func (b *Bot) applyAutoPriority(ctx context.Context, torrentID int64) {
	if !b.autoPriorityEnabled {
		return
	}
	if err := b.client.SetHighPriorityFiles(ctx, torrentID, b.autoPriorityHighCount); err != nil {
		b.logger.Warn("auto-priority: failed to set file priorities", "torrent_id", torrentID, "err", err)
	}
}
