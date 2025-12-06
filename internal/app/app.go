package app

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"bot-go-template/internal/adapter/telegram"
	"bot-go-template/internal/adapter/telegram/handlers"
	"bot-go-template/internal/adapter/telegram/middleware"
	"bot-go-template/internal/config"
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
		App:          "bot-go-template",
	})
	return &App{cfg: cfg, log: log}, nil
}

// Run starts the application.
func (a *App) Run() error {
	a.log.Info("starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rate := middleware.NewRateLimiter(time.Second)
	handler := middleware.Chain(handlers.Handle, rate.Middleware)

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
