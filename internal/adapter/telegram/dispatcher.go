package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Update aliases models.Update for brevity.
type Update = models.Update

type ctxUpdate struct {
	ctx context.Context
	upd *models.Update
}

// HandlerFunc processes a single update.
type HandlerFunc func(ctx context.Context, b *bot.Bot, upd *models.Update)

// Dispatcher routes updates to worker goroutines keeping chat order.
type Dispatcher struct {
	bot     *bot.Bot
	handler HandlerFunc
	workers int
	chans   []chan ctxUpdate
}

// NewDispatcher creates dispatcher with given worker count.
func NewDispatcher(b *bot.Bot, workers int, h HandlerFunc) *Dispatcher {
	d := &Dispatcher{bot: b, handler: h, workers: workers, chans: make([]chan ctxUpdate, workers)}
	for i := 0; i < workers; i++ {
		d.chans[i] = make(chan ctxUpdate, 100)
		go d.worker(d.chans[i])
	}
	return d
}

// Dispatch sends update to appropriate worker based on chat ID.
func (d *Dispatcher) Dispatch(ctx context.Context, upd *models.Update) {
	chatID := extractChatID(upd)
	idx := 0
	if chatID != 0 {
		idx = int(abs(chatID)) % d.workers
	}
	d.chans[idx] <- ctxUpdate{ctx: ctx, upd: upd}
}

func (d *Dispatcher) worker(in <-chan ctxUpdate) {
	for item := range in {
		d.handler(item.ctx, d.bot, item.upd)
	}
}

func extractChatID(u *models.Update) int64 {
	if u.Message != nil {
		return u.Message.Chat.ID
	}
	if u.CallbackQuery != nil && u.CallbackQuery.Message.Message != nil {
		return u.CallbackQuery.Message.Message.Chat.ID
	}
	return 0
}

func abs(i int64) int64 {
	if i < 0 {
		return -i
	}
	return i
}
