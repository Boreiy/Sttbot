package config

import "testing"

func TestSTTDefaults(t *testing.T) {
	for _, tc := range []struct{ mode, url, model string }{
		{"", "https://api.selectel.ru/aig/v1", "qwen/qwen3.8-omni-flash"},
		{"classic", "https://api.openai.com/v1", "gpt-4o-mini-transcribe"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("STT_MODE", tc.mode)
			t.Setenv("OPENAI_BASE_URL", "")
			t.Setenv("OPENAI_STT_MODEL", "")
			t.Setenv("OPENAI_API_KEY", "key")
			t.Setenv("TELEGRAM_BOT_TOKEN", "token")
			c, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if c.OpenAI.BaseURL != tc.url || c.OpenAI.STTModel != tc.model {
				t.Fatalf("unexpected config: %+v", c.OpenAI)
			}
			t.Setenv("OPENAI_BASE_URL", "https://custom.example/v1")
			t.Setenv("OPENAI_STT_MODEL", "custom")
			c, err = Load()
			if err != nil || c.OpenAI.BaseURL != "https://custom.example/v1" || c.OpenAI.STTModel != "custom" {
				t.Fatal("explicit overrides not applied")
			}
		})
	}
	t.Setenv("STT_MODE", "invalid")
	t.Setenv("OPENAI_API_KEY", "key")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	if _, err := Load(); err == nil {
		t.Fatal("invalid mode accepted")
	}
}
