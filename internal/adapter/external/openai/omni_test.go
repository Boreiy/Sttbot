package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sttbot/internal/adapter/external/openai"
	"sttbot/internal/platform/httpclient"
	"testing"
)

func TestOmniRequest(t *testing.T) {
	client := httpclient.New(httpclient.WithTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/aig/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("invalid request")
		}
		var body struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Content []struct {
					Type  string `json:"type"`
					Audio struct {
						Data   string `json:"data"`
						Format string `json:"format"`
					} `json:"input_audio"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "qwen/qwen3.8-omni-flash" || body.Stream || len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 {
			t.Fatal("invalid payload")
		}
		audio := body.Messages[0].Content[1].Audio
		if audio.Format != "ogg" || audio.Data != "data:audio/ogg;base64,ZGF0YQ==" {
			t.Fatalf("invalid audio: %+v", audio)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"Привет"},"finish_reason":"stop"}]}`))}, nil
	})))
	tr := openai.NewTranscriber(client, "https://api.selectel.ru/aig/v1/", "qwen/qwen3.8-omni-flash", "secret", "omni")
	text, err := tr.Transcribe(context.Background(), "voice.oga", "audio/ogg", []byte("data"))
	if err != nil || text != "Привет" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestOmniInvalidResponse(t *testing.T) {
	for _, body := range []string{`{}`, `{"choices":[{"message":{"content":""},"finish_reason":"stop"}]}`, `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`} {
		client := httpclient.New(httpclient.WithTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})))
		tr := openai.NewTranscriber(client, "https://example.com", "model", "key", "omni")
		if _, err := tr.Transcribe(context.Background(), "voice.ogg", "audio/ogg", []byte("data")); err == nil {
			t.Fatal("expected response error")
		}
	}
}
