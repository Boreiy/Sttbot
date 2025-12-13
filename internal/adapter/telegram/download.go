// Package telegram содержит вспомогательные функции для работы с файлами Telegram
package telegram

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot"

	"sttbot/internal/platform/httpclient"
)

// DownloadFile загружает файл по file_id и возвращает имя, content-type и содержимое
func DownloadFile(ctx context.Context, b *bot.Bot, token, fileID string, client *httpclient.Client) (string, string, []byte, error) {
	f, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return "", "", nil, err
	}
	u := "https://api.telegram.org/file/bot" + token + "/" + f.FilePath
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", "", nil, err
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.ReadAll(resp.Body)
		return "", "", nil, io.ErrUnexpectedEOF
	}
	name := filepath.Base(f.FilePath)
	ct := guessCT(name)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", nil, err
	}
	return name, ct, data, nil
}

func guessCT(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/m4a"
	case ".webm":
		return "audio/webm"
	default:
		return "application/octet-stream"
	}
}

// IsSupportedAudio проверяет, что документ — допустимое аудио
func IsSupportedAudio(mime, filename string) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	if strings.HasPrefix(m, "video/") { // явные видео запрещаем
		return false
	}
	if strings.HasPrefix(m, "audio/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".ogg", ".oga", ".mp3", ".m4a", ".wav", ".webm", ".mpga", ".mpeg":
		return true
	default:
		return false
	}
}
