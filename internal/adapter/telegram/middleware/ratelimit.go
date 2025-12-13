package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"sttbot/internal/adapter/telegram"
)

// RateLimiter restricts request frequency per user.
type RateLimiter struct {
	mu   sync.Mutex
	last map[int64]time.Time
	rate time.Duration
}

// NewRateLimiter creates limiter with given rate.
func NewRateLimiter(rate time.Duration) *RateLimiter {
	return &RateLimiter{last: make(map[int64]time.Time), rate: rate}
}

// Allow returns false if user hits the limit.
func (r *RateLimiter) Allow(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if t, ok := r.last[userID]; ok && now.Sub(t) < r.rate {
		return false
	}
	r.last[userID] = now
	return true
}

// Middleware checks rate limit before calling next handler.
func (r *RateLimiter) Middleware(next telegram.HandlerFunc) telegram.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, upd *models.Update) {
		var (
			uid  int64
			chat int64
		)
		if msg := upd.Message; msg != nil && msg.From != nil {
			uid = msg.From.ID
			chat = msg.Chat.ID
		} else if cq := upd.CallbackQuery; cq != nil && cq.From.ID != 0 {
			uid = cq.From.ID
			chat = cq.Message.Message.Chat.ID
		}
		if uid != 0 && !r.Allow(uid) {
			if chat != 0 {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chat,
					Text:   "слишком часто",
				})
			}
			return
		}
		next(ctx, b, upd)
	}
}
