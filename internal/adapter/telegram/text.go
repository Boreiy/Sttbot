package telegram

import (
	"context"

	"github.com/go-telegram/bot"
)

// SplitText preserves every character and limits chunks to 4096 UTF-16 units.
// Counting supplementary characters as two units also keeps emoji within the limit.
func SplitText(text string) []string {
	var chunks []string
	start, units := 0, 0
	for i, r := range text {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > 4096 {
			chunks = append(chunks, text[start:i])
			start, units = i, 0
		}
		units += width
	}
	if start < len(text) {
		chunks = append(chunks, text[start:])
	}
	return chunks
}

// SendLongText sends chunks sequentially and stops on the first delivery error.
func SendLongText(ctx context.Context, b *bot.Bot, chatID int64, text string) error {
	for _, chunk := range SplitText(text) {
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: chunk}); err != nil {
			return err
		}
	}
	return nil
}
