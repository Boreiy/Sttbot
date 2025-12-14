package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"sttbot/internal/adapter/external/openai"
	"sttbot/internal/platform/httpclient"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTranscribe_OK(t *testing.T) {
	rt := rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		var sawFile, sawModel bool
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("read part: %v", err)
			}
			switch part.FormName() {
			case "file":
				sawFile = true
				if ct := part.Header.Get("Content-Type"); ct != "audio/wav" {
					t.Fatalf("content-type=%s", ct)
				}
				data, _ := io.ReadAll(part)
				if string(data) != "data" {
					t.Fatalf("file data mismatch: %q", string(data))
				}
			case "model":
				sawModel = true
				data, _ := io.ReadAll(part)
				if string(data) != "gpt-4o-mini-transcribe" {
					t.Fatalf("model=%s", string(data))
				}
			}
		}
		if !sawFile || !sawModel {
			t.Fatalf("multipart parts missing: file=%v model=%v", sawFile, sawModel)
		}
		out := map[string]any{"text": "Привет"}
		data, _ := json.Marshal(out)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})

	client := httpclient.New(httpclient.WithTransport(rt))
	tr := openai.NewTranscriber(client, "https://api.openai.com/v1", "gpt-4o-mini-transcribe", "secret")

	got, err := tr.Transcribe(context.Background(), "sample.wav", "audio/wav", []byte("data"))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != "Привет" {
		t.Fatalf("got=%q", got)
	}
}

func TestTranscribe_ErrorStatus(t *testing.T) {
	rt := rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	client := httpclient.New(httpclient.WithTransport(rt))
	tr := openai.NewTranscriber(client, "https://api.openai.com/v1", "gpt-4o-mini-transcribe", "secret")
	_, err := tr.Transcribe(context.Background(), "sample.wav", "audio/wav", []byte("data"))
	if err == nil {
		t.Fatalf("expected error")
	}
}
