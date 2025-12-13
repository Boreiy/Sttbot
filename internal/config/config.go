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
	OpenAI struct {
		APIKey   string `validate:"required"`
		BaseURL  string `validate:"required"`
		STTModel string `validate:"required"`
	}
	AllowedIDs []int64
	Log        struct {
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
	c.HTTP.Addr = getenv("HTTP_ADDR", ":2010")
	c.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	c.OpenAI.BaseURL = getenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	c.OpenAI.STTModel = getenv("OPENAI_STT_MODEL", "gpt-4o-mini-transcribe")
	c.AllowedIDs = parseIDs(os.Getenv("ALLOWED_IDS"))
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

func parseIDs(s string) []int64 {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\t' || r == ' ' })
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		var v int64
		for i := 0; i < len(p); i++ {
			c := p[i]
			if c < '0' || c > '9' {
				v = -1
				break
			}
		}
		if v == -1 {
			continue
		}
		// fast parse
		var n int64
		for i := 0; i < len(p); i++ {
			n = n*10 + int64(p[i]-'0')
		}
		out = append(out, n)
	}
	return out
}
