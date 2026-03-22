package bot

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"gopkg.in/telebot.v4"
)

func (b *Bot) callbackFileToggle(c telebot.Context, data string) error {
	torrentID, fileIndex, err := parseTwoInts(data, "ft:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid data", ShowAlert: true})
	}

	state := b.getFileSelectState(torrentID)
	if state == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Session expired", ShowAlert: true})
	}

	idx := int(fileIndex)
	if idx < 0 || idx >= len(state.Selected) {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid file index", ShowAlert: true})
	}

	state.Selected[idx] = !state.Selected[idx]

	_ = c.Respond()
	return c.Edit(formatFileSelectMessage(state), fileSelectKeyboard(state), telebot.ModeHTML)
}

func (b *Bot) callbackFileSelectAll(c telebot.Context, data string) error {
	torrentID, err := parseID(data, "fa:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid data", ShowAlert: true})
	}

	state := b.getFileSelectState(torrentID)
	if state == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Session expired", ShowAlert: true})
	}

	for i := range state.Selected {
		state.Selected[i] = true
	}

	_ = c.Respond()
	return c.Edit(formatFileSelectMessage(state), fileSelectKeyboard(state), telebot.ModeHTML)
}

func (b *Bot) callbackFileDeselectAll(c telebot.Context, data string) error {
	torrentID, err := parseID(data, "fn:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid data", ShowAlert: true})
	}

	state := b.getFileSelectState(torrentID)
	if state == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Session expired", ShowAlert: true})
	}

	for i := range state.Selected {
		state.Selected[i] = false
	}

	_ = c.Respond()
	return c.Edit(formatFileSelectMessage(state), fileSelectKeyboard(state), telebot.ModeHTML)
}

func (b *Bot) callbackFilePage(c telebot.Context, data string) error {
	torrentID, page, err := parseTwoInts(data, "fp:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid data", ShowAlert: true})
	}

	state := b.getFileSelectState(torrentID)
	if state == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Session expired", ShowAlert: true})
	}

	state.Page = int(page)

	_ = c.Respond()
	return c.Edit(formatFileSelectMessage(state), fileSelectKeyboard(state), telebot.ModeHTML)
}

func (b *Bot) callbackFileConfirm(c telebot.Context, data string) error {
	torrentID, err := parseID(data, "fc:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid data", ShowAlert: true})
	}

	state := b.getFileSelectState(torrentID)
	if state == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Session expired", ShowAlert: true})
	}

	selCount, _ := selectedCount(state)
	if selCount == 0 {
		return c.Respond(&telebot.CallbackResponse{Text: "Select at least one file", ShowAlert: true})
	}

	var wanted, unwanted []int
	for i, sel := range state.Selected {
		if sel {
			wanted = append(wanted, i)
		} else {
			unwanted = append(unwanted, i)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if len(unwanted) > 0 {
		if err := b.client.SetFilesWanted(ctx, torrentID, wanted, unwanted); err != nil {
			b.logger.Warn("file select: set files wanted failed", "err", err)
			return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
		}
	}

	if err := b.client.StartTorrents(ctx, []int64{torrentID}); err != nil {
		b.logger.Warn("file select: start torrent failed", "err", err)
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	b.deleteFileSelectState(torrentID)

	_ = c.Respond()
	text := fmt.Sprintf("📥 Started: <b>%s</b>\n📂 Files: %d/%d",
		html.EscapeString(truncate(state.TorrentName, maxTorrentName)), selCount, len(state.Files))
	return c.Edit(text, telebot.ModeHTML)
}

func (b *Bot) callbackFileSkip(c telebot.Context, data string) error {
	torrentID, err := parseID(data, "fk:")
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid data", ShowAlert: true})
	}

	state := b.getFileSelectState(torrentID)
	if state == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Session expired", ShowAlert: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := b.client.StartTorrents(ctx, []int64{torrentID}); err != nil {
		b.logger.Warn("file select: start torrent failed", "err", err)
		return c.Respond(&telebot.CallbackResponse{Text: "Error: " + err.Error(), ShowAlert: true})
	}

	b.applyAutoPriority(ctx, torrentID)
	b.deleteFileSelectState(torrentID)

	_ = c.Respond()
	text := fmt.Sprintf("📥 Started: <b>%s</b>\n📂 All files", html.EscapeString(truncate(state.TorrentName, maxTorrentName)))
	return c.Edit(text, telebot.ModeHTML)
}

func parseTwoInts(data, prefix string) (int64, int64, error) {
	s := strings.TrimPrefix(data, prefix)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected two parts")
	}
	a, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	b, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}
