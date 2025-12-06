package handlers

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Handle routes updates to command handlers.
func Handle(ctx context.Context, b *bot.Bot, upd *models.Update) {
	if msg := upd.Message; msg != nil && strings.HasPrefix(msg.Text, "/") {
		cmd := strings.TrimPrefix(strings.SplitN(msg.Text, " ", 2)[0], "/")
		switch cmd {
		case "start":
			Start(ctx, b, msg)
		case "ping":
			Ping(ctx, b, msg)
		}
	}
}
