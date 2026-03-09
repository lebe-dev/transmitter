package bot

import (
	"context"
	"fmt"
	"html"
	"time"
)

// StartMonitor polls Transmission on the given interval and sends completion notifications.
// Blocks until ctx is cancelled.
func (b *Bot) StartMonitor(ctx context.Context, interval time.Duration) {
	b.logger.Info("torrent monitor starting", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			b.logger.Info("torrent monitor stopped")
			return
		case <-ticker.C:
			b.pollOnce(ctx)
		}
	}
}

func (b *Bot) pollOnce(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	torrents, err := b.getter.GetTorrents(pollCtx)
	if err != nil {
		b.logger.Warn("monitor: get torrents failed", "err", err)
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// First successful poll: snapshot state, no notifications.
	if b.progress == nil {
		b.progress = make(map[int64]float64, len(torrents))
		for _, t := range torrents {
			b.progress[t.ID] = t.PercentDone
		}
		b.logger.Debug("monitor: state initialized", "count", len(torrents))
		return
	}

	for _, t := range torrents {
		prev, known := b.progress[t.ID]
		b.progress[t.ID] = t.PercentDone
		if known && prev < 1.0 && t.PercentDone >= 1.0 {
			b.logger.Info("torrent completed", "id", t.ID, "name", t.Name)
			msg := formatCompletionMessage(t.Name, t.TotalSize)
			go b.notifyFn(msg)
		}
	}

	// Prune removed torrents.
	current := make(map[int64]struct{}, len(torrents))
	for _, t := range torrents {
		current[t.ID] = struct{}{}
	}
	for id := range b.progress {
		if _, ok := current[id]; !ok {
			delete(b.progress, id)
		}
	}
}

func formatCompletionMessage(name string, totalSize int64) string {
	return fmt.Sprintf("✅ <b>Download complete</b>\n\n📄 %s\n💾 %s",
		html.EscapeString(name), formatSize(totalSize))
}
