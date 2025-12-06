# Telegram и UX (go-telegram/bot, polling, webhook, клавиатуры)

Документ — практический и самодостаточный. Описывает подключение `github.com/go-telegram/bot`, запуск бота в dev и prod, структуру обработчиков, клавиатуры и базовые рекомендации по идемпотентности.

## 1) Подключение `go-telegram/bot` и установка

```bash
go get github.com/go-telegram/bot
```

## 2) Базовый запуск в dev через long polling

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/go-telegram/bot"
    "github.com/go-telegram/bot/models"
)

func main() {
    ctx := context.Background()
    b, err := bot.New(os.Getenv("TELEGRAM_BOT_TOKEN"))
    if err != nil {
        log.Fatal(err) // bot init failed
    }

    b.RegisterHandler(bot.HandlerTypeMessageText, "/start", startHandler)
    b.Start(ctx)
}

func startHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
    b.SendMessage(ctx, &bot.SendMessageParams{
        ChatID: update.Message.Chat.ID,
        Text:   "Привет!", // simple greeting
    })
}
```

## 3) Запуск в prod через webhook

```go
r := gin.New()
r.Use(gin.Recovery())
r.POST("/telegram/webhook", webhookHandler)

b, _ := bot.New(token)
_ = b.SetWebhook(ctx, &bot.SetWebhookParams{URL: webhookURL, SecretToken: secret})

go b.StartWebhook(ctx)
_ = http.ListenAndServe(":80", r)
```

## 4) Клавиатуры и форматирование сообщений

```go
params := &bot.SendMessageParams{
    ChatID: update.Message.Chat.ID,
    Text:   "Выбери опцию", // message text
    ReplyMarkup: &models.ReplyKeyboardMarkup{
        Keyboard: [][]models.KeyboardButton{{
            {Text: "A"}, {Text: "B"},
        }},
    },
}
if _, err := b.SendMessage(ctx, params); err != nil {
    log.Println("send:", err) // handle send error
}
```

## 5) Идемпотентность и rate limiting

- Храните `update_id` в памяти/БД, чтобы игнорировать дубликаты.
- Используйте middleware `ratelimit` для ограничения частоты вызовов.
- Разделяйте обработку чатов по воркерам (см. `Dispatcher`) для сохранения порядка сообщений.

## 6) Безопасность

- Не логируйте токен бота и личные данные пользователей.
- Проверяйте заголовок `X-Telegram-Bot-Api-Secret-Token` при вебхуке.

