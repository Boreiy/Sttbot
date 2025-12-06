package handlers

import (
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Start handles /start command.
func Start(ctx context.Context, b *bot.Bot, msg *models.Message) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "запущено",
	})
	if err != nil {
		log.Println("send start:", err)
	}
}
