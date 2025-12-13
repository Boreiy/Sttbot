package openai_test

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "strings"
    "testing"

    "bot-go-template/internal/adapter/external/openai"
    "bot-go-template/internal/platform/httpclient"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTranscribe_OK(t *testing.T) {
    var capturedBody string
    rt := rtFunc(func(r *http.Request) (*http.Response, error) {
        if r.Method != http.MethodPost { t.Fatalf("method=%s", r.Method) }
        if !strings.HasSuffix(r.URL.Path, "/audio/transcriptions") { t.Fatalf("path=%s", r.URL.Path) }
        b, _ := io.ReadAll(r.Body)
        capturedBody = string(b)
        out := map[string]any{"text": "Привет"}
        data, _ := json.Marshal(out)
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
    })

    client := httpclient.New(httpclient.WithTransport(rt))
    tr := openai.NewTranscriber(client, "https://api.openai.com/v1", "gpt-4o-mini-transcribe", "secret")

    got, err := tr.Transcribe(context.Background(), "sample.wav", "audio/wav", []byte("data"))
    if err != nil { t.Fatalf("err=%v", err) }
    if got != "Привет" { t.Fatalf("got=%q", got) }
    if !strings.Contains(capturedBody, "name=\"model\"") { t.Fatalf("model field missing") }
    if !strings.Contains(capturedBody, "name=\"file\"") { t.Fatalf("file field missing") }
}

func TestTranscribe_ErrorStatus(t *testing.T) {
    rt := rtFunc(func(r *http.Request) (*http.Response, error) {
        return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("{}"))}, nil
    })
    client := httpclient.New(httpclient.WithTransport(rt))
    tr := openai.NewTranscriber(client, "https://api.openai.com/v1", "gpt-4o-mini-transcribe", "secret")
    _, err := tr.Transcribe(context.Background(), "sample.wav", "audio/wav", []byte("data"))
    if err == nil { t.Fatalf("expected error") }
}

