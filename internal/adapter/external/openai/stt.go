// Package openai содержит адаптер для распознавания речи через OpenAI
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"sttbot/internal/platform/httpclient"
)

// Transcriber выполняет транскрибацию аудио в текст через OpenAI API
type Transcriber struct {
	client  *httpclient.Client
	baseURL string
	model   string
	apiKey  string
}

// NewTranscriber создаёт клиент транскрибации
func NewTranscriber(c *httpclient.Client, baseURL, model, apiKey string) *Transcriber {
	return &Transcriber{client: c, baseURL: strings.TrimRight(baseURL, "/"), model: model, apiKey: apiKey}
}

// Transcribe отправляет аудио и возвращает распознанный текст
func (t *Transcriber) Transcribe(ctx context.Context, filename, contentType string, data []byte) (string, error) {
	name := filename
	body := data
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, bytes.NewReader(body)); err != nil {
		return "", err
	}
	if err := w.WriteField("model", t.model); err != nil {
		return "", err
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, t.baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := t.client.Do(cctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Text, nil
}

// Конвертация не используется: OpenAI поддерживает OGG; отправляем исходный файл без преобразования
