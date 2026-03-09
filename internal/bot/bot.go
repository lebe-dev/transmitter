package bot

import (
	"log/slog"

	"github.com/lebe-dev/transmitter/internal/transmission"
	"gopkg.in/telebot.v4"
)

// Bot wraps the Telegram bot with authorization and Transmission client.
type Bot struct {
	tg     *telebot.Bot
	client *transmission.Client
	users  map[int64]bool
	logger *slog.Logger
}

// New creates a new Bot instance. Returns an error if the token is invalid.
func New(token string, allowedUsers []int64, client *transmission.Client, logger *slog.Logger) (*Bot, error) {
	tg, err := telebot.NewBot(telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10},
	})
	if err != nil {
		return nil, err
	}

	users := make(map[int64]bool, len(allowedUsers))
	for _, id := range allowedUsers {
		users[id] = true
	}

	b := &Bot{
		tg:     tg,
		client: client,
		users:  users,
		logger: logger,
	}

	b.registerHandlers()
	return b, nil
}

func (b *Bot) registerHandlers() {
	b.tg.Use(b.authMiddleware)

	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle("/add", b.handleAdd)
	b.tg.Handle("/status", b.handleStatus)
	b.tg.Handle(telebot.OnDocument, b.handleDocument)
}

// authMiddleware silently ignores messages from unauthorized users.
func (b *Bot) authMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if !b.users[c.Sender().ID] {
			b.logger.Warn("unauthorized telegram user", "id", c.Sender().ID, "username", c.Sender().Username)
			return nil
		}
		return next(c)
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
