package app

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"bot-go-template/internal/adapter/external/openai"
	"bot-go-template/internal/adapter/telegram"
	"bot-go-template/internal/adapter/telegram/handlers"
	"bot-go-template/internal/adapter/telegram/middleware"
	"bot-go-template/internal/config"
	"bot-go-template/internal/platform/httpclient"
	"bot-go-template/internal/platform/logger"
)

// App wires application components.
type App struct {
	cfg config.Config
	log *slog.Logger
}

// New creates a new App instance and loads configuration.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	log := logger.New(logger.Options{
		Env:          cfg.Env,
		ConsoleLevel: cfg.Log.ConsoleLevel,
		FileLevel:    cfg.Log.FileLevel,
		File:         cfg.Log.File,
		App:          "sttbot",
	})
	return &App{cfg: cfg, log: log}, nil
}

// Run starts the application.
func (a *App) Run() error {
	a.log.Info("starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rate := middleware.NewRateLimiter(time.Second)
	acl := middleware.NewACL(a.cfg.AllowedIDs)
	client := httpclient.New(httpclient.WithLogger(a.log))
	tr := openai.NewTranscriber(client, a.cfg.OpenAI.BaseURL, a.cfg.OpenAI.STTModel, a.cfg.OpenAI.APIKey)

	handlerFunc := func(ctx context.Context, b *bot.Bot, upd *models.Update) {
		if msg := upd.Message; msg != nil {
			if strings.HasPrefix(msg.Text, "/") {
				handlers.Handle(ctx, b, upd)
				return
			}
			if v := msg.Voice; v != nil {
				name, ct, data, err := telegram.DownloadFile(ctx, b, a.cfg.Telegram.Token, v.FileID, client)
				if err != nil {
					return
				}
				txt, err := tr.Transcribe(ctx, name, ct, data)
				if err != nil {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "ошибка распознавания"})
					return
				}
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: txt})
				return
			}
			if aud := msg.Audio; aud != nil {
				name, ct, data, err := telegram.DownloadFile(ctx, b, a.cfg.Telegram.Token, aud.FileID, client)
				if err != nil {
					return
				}
				txt, err := tr.Transcribe(ctx, name, ct, data)
				if err != nil {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "ошибка распознавания"})
					return
				}
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: txt})
				return
			}
			if doc := msg.Document; doc != nil {
				if !telegram.IsSupportedAudio(doc.MimeType, doc.FileName) {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "неподдерживаемый формат"})
					return
				}
				name, ct, data, err := telegram.DownloadFile(ctx, b, a.cfg.Telegram.Token, doc.FileID, client)
				if err != nil {
					return
				}
				txt, err := tr.Transcribe(ctx, name, ct, data)
				if err != nil {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "ошибка распознавания"})
					return
				}
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: txt})
				return
			}
		}
	}
	handler := middleware.Chain(handlerFunc, rate.Middleware, acl.Middleware)

	var disp *telegram.Dispatcher
	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, upd *models.Update) {
			disp.Dispatch(ctx, upd)
		}),
		bot.WithAllowedUpdates([]string{"message", "callback_query"}),
	}
	if a.cfg.Telegram.WebhookSecret != "" {
		opts = append(opts, bot.WithWebhookSecretToken(a.cfg.Telegram.WebhookSecret))
	}

	b, err := bot.New(a.cfg.Telegram.Token, opts...)
	if err != nil {
		return err
	}

	disp = telegram.NewDispatcher(b, 8, handler)

	if a.cfg.Telegram.WebhookURL != "" {
		_, err := b.SetWebhook(ctx, &bot.SetWebhookParams{
			URL:         a.cfg.Telegram.WebhookURL,
			SecretToken: a.cfg.Telegram.WebhookSecret,
		})
		if err != nil {
			return err
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.POST("/telegram/webhook", gin.WrapH(b.WebhookHandler()))

		srv := &http.Server{Addr: a.cfg.HTTP.Addr, Handler: r}
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				a.log.Error("server", slog.Any("err", err))
			}
		}()
		go b.StartWebhook(ctx)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}

	go b.Start(ctx)
	<-ctx.Done()
	return nil
}
