package config

import (
	"errors"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

// Config holds application configuration values.
type Config struct {
	Env      string `validate:"required,oneof=dev prod"`
	Telegram struct {
		Token         string `validate:"required"`
		WebhookURL    string
		WebhookSecret string
	}
	HTTP struct {
		Addr string `validate:"required"`
	}
	Log struct {
		ConsoleLevel string `validate:"required,oneof=debug info warn error"`
		FileLevel    string `validate:"required,oneof=debug info warn error"`
		File         string
	}
}

var validate = validator.New()

// Load reads configuration from environment variables and optional .env file.
func Load() (Config, error) {
	_ = godotenv.Load()

	var c Config
	c.Env = getenv("ENV", "prod")
	c.Telegram.Token = os.Getenv("TELEGRAM_BOT_TOKEN")
	c.Telegram.WebhookURL = os.Getenv("TELEGRAM_WEBHOOK_URL")
	c.Telegram.WebhookSecret = os.Getenv("TELEGRAM_WEBHOOK_SECRET")
	c.HTTP.Addr = getenv("HTTP_ADDR", ":80")
	c.Log.ConsoleLevel = strings.ToLower(getenv("LOG_CONSOLE_LEVEL", "info"))
	c.Log.FileLevel = strings.ToLower(getenv("LOG_FILE_LEVEL", "debug"))
	c.Log.File = getenv("LOG_FILE", "data/logs/bot.log")

	if err := validate.Struct(c); err != nil {
		return Config{}, err
	}
	if c.Telegram.WebhookURL != "" && c.Telegram.WebhookSecret == "" {
		return Config{}, errors.New("TELEGRAM_WEBHOOK_SECRET required when TELEGRAM_WEBHOOK_URL is set")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
