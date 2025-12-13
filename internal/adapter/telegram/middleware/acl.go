// Package middleware содержит телеграм‑middleware, включая ACL по списку разрешённых пользователей
package middleware

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"bot-go-template/internal/adapter/telegram"
)

// ACL проверяет доступ по списку разрешённых Telegram user IDs
type ACL struct{ allowed map[int64]struct{} }

// NewACL создаёт ACL по списку ID
func NewACL(ids []int64) *ACL {
	m := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return &ACL{allowed: m}
}

// IsAllowed сообщает, имеет ли пользователь доступ
func (a *ACL) IsAllowed(id int64) bool { _, ok := a.allowed[id]; return ok }

// Middleware блокирует выполнение хендлера для неразрешённых пользователей
func (a *ACL) Middleware(next telegram.HandlerFunc) telegram.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, upd *models.Update) {
		var uid int64
		var chat int64
		if m := upd.Message; m != nil {
			chat = m.Chat.ID
			if m.From != nil {
				uid = m.From.ID
			}
		} else if cb := upd.CallbackQuery; cb != nil {
			chat = cb.Message.Message.Chat.ID
			uid = cb.From.ID
		}
		if uid == 0 || a.IsAllowed(uid) {
			next(ctx, b, upd)
			return
		}
		if chat != 0 && b != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chat, Text: "доступ запрещен"})
		}
	}
}

// ParseAllowedIDs парсит список ID из строки (разделители: запятая/переносы)
func ParseAllowedIDs(s string) []int64 {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\t' })
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var n int64
		for i := 0; i < len(p); i++ {
			n = n*10 + int64(p[i]-'0')
		}
		out = append(out, n)
	}
	return out
}
