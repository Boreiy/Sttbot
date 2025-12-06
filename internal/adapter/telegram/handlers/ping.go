package handlers

import (
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Ping handles /ping command.
func Ping(ctx context.Context, b *bot.Bot, msg *models.Message) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "pong",
	})
	if err != nil {
		log.Println("send ping:", err)
	}
}
