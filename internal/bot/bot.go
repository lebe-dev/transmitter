package bot

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lebe-dev/transmitter/internal/transmission"
	"gopkg.in/telebot.v4"
)

// torrentGetter is satisfied by *transmission.Client and allows test injection.
type torrentGetter interface {
	GetTorrents(ctx context.Context) ([]transmission.Torrent, error)
}

// Bot wraps the Telegram bot with authorization and Transmission client.
type Bot struct {
	tg                    *telebot.Bot
	client                *transmission.Client
	getter                torrentGetter // used by monitor; equals client unless overridden in tests
	users                 map[string]bool
	logger                *slog.Logger
	mu                    sync.RWMutex
	chatIDs               map[string]int64  // username → Telegram chat/user ID
	progress              map[int64]float64 // torrent ID → last known PercentDone (nil = uninitialized)
	notifyFn              func(string)      // injectable for tests
	autoPriorityEnabled   bool
	autoPriorityHighCount int
}

// New creates a new Bot instance. Returns an error if the token is invalid.
func New(token string, allowedUsers []string, client *transmission.Client, logger *slog.Logger, autoPriorityEnabled bool, autoPriorityHighCount int) (*Bot, error) {
	tg, err := telebot.NewBot(telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10},
	})
	if err != nil {
		return nil, err
	}

	users := make(map[string]bool, len(allowedUsers))
	for _, username := range allowedUsers {
		users[username] = true
	}

	b := &Bot{
		tg:                    tg,
		client:                client,
		getter:                client,
		users:                 users,
		logger:                logger,
		chatIDs:               make(map[string]int64),
		autoPriorityEnabled:   autoPriorityEnabled,
		autoPriorityHighCount: autoPriorityHighCount,
	}
	b.notifyFn = b.broadcastNotification

	b.registerHandlers()
	return b, nil
}

func (b *Bot) registerHandlers() {
	b.tg.Use(b.authMiddleware)

	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle("/add", b.handleAdd)
	b.tg.Handle("/status", b.handleStatus)
	b.tg.Handle("/status_all", b.handleStatusAll)
	b.tg.Handle(telebot.OnDocument, b.handleDocument)
	b.tg.Handle(telebot.OnCallback, b.handleCallback)
}

// authMiddleware silently ignores messages from unauthorized users.
func (b *Bot) authMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if !b.users[c.Sender().Username] {
			b.logger.Warn("unauthorized telegram user", "id", c.Sender().ID, "username", c.Sender().Username)
			return nil
		}
		b.mu.Lock()
		b.chatIDs[c.Sender().Username] = int64(c.Sender().ID)
		b.mu.Unlock()
		return next(c)
	}
}

// broadcastNotification sends a message to all known authorized chat IDs.
func (b *Bot) broadcastNotification(text string) {
	b.mu.RLock()
	ids := make(map[string]int64, len(b.chatIDs))
	for k, v := range b.chatIDs {
		ids[k] = v
	}
	b.mu.RUnlock()

	for username, chatID := range ids {
		if _, err := b.tg.Send(telebot.ChatID(chatID), text, telebot.ModeHTML); err != nil {
			b.logger.Warn("notification failed", "username", username, "err", err)
		}
	}
}

// Start begins the long polling loop. Blocks until Stop is called.
func (b *Bot) Start() {
	b.logger.Info("telegram bot starting")
	b.tg.Start()
}

// Stop gracefully stops the long polling loop.
func (b *Bot) Stop() {
	b.logger.Info("telegram bot stopping")
	b.tg.Stop()
}
