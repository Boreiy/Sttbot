package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lmittmann/tint"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Options defines parameters for logger creation.
type Options struct {
	Env          string
	ConsoleLevel string // Level for console output (default: info)
	FileLevel    string // Level for file output (default: debug)
	File         string
	App          string
}

var closers sync.Map

// New creates configured slog.Logger instance.
func New(o Options) *slog.Logger {
	consoleLevel := o.ConsoleLevel
	if consoleLevel == "" {
		consoleLevel = "info"
	}
	fileLevel := o.FileLevel
	if fileLevel == "" {
		fileLevel = "debug"
	}

	consoleLvl := levelFromString(consoleLevel)
	fileLvl := levelFromString(fileLevel)

	var handlers []slog.Handler

	// Console handler
	var consoleHandler slog.Handler
	if o.Env == "dev" {
		consoleHandler = tint.NewHandler(os.Stdout, &tint.Options{Level: consoleLvl, TimeFormat: time.Kitchen})
	} else {
		consoleHandler = tint.NewHandler(
			os.Stdout,
			&tint.Options{
				Level:      consoleLvl,
				TimeFormat: time.RFC3339,
				NoColor:    false,
			},
		)
	}
	consoleHandler = NewRedactingHandler(consoleHandler, []string{"token", "secret", "api_key"})
	handlers = append(handlers, consoleHandler)

	var closer func() error

	// File handler (if file path is specified)
	if o.File != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   o.File,
			MaxSize:    5,
			MaxBackups: 3,
			MaxAge:     28,
			Compress:   true,
		}
		closer = fileWriter.Close
		var fileHandler slog.Handler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{Level: fileLvl})
		fileHandler = NewRedactingHandler(fileHandler, []string{"token", "secret", "api_key"})
		handlers = append(handlers, fileHandler)
	}

	var h slog.Handler
	if len(handlers) == 1 {
		h = handlers[0]
	} else {
		h = NewMultiHandler(handlers...)
	}

	l := slog.New(h).With(
		slog.String("app", o.App),
		slog.String("env", o.Env),
	)

	if closer != nil {
		closers.Store(l, closer)
	}

	return l
}

// Close closes all file handlers to release resources.
// Should be called when shutting down the application.
func Close(logger *slog.Logger) error {
	if c, ok := closers.Load(logger); ok {
		closers.Delete(logger)
		return c.(func() error)()
	}
	return nil
}

func levelFromString(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// RedactingHandler masks sensitive log attributes.
type RedactingHandler struct {
	inner slog.Handler
	keys  map[string]struct{}
}

// NewRedactingHandler wraps handler with redaction of sensitive fields.
func NewRedactingHandler(inner slog.Handler, sensitive []string) *RedactingHandler {
	m := make(map[string]struct{}, len(sensitive))
	for _, k := range sensitive {
		m[strings.ToLower(k)] = struct{}{}
	}
	return &RedactingHandler{inner: inner, keys: m}
}

// Enabled implements slog.Handler.
func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

// Handle implements slog.Handler.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool { attrs = append(attrs, a); return true })
	nr.AddAttrs(h.sanitize(attrs...)...)
	return h.inner.Handle(ctx, nr)
}

// WithAttrs implements slog.Handler.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithAttrs(h.sanitize(attrs...)), keys: h.keys}
}

// WithGroup implements slog.Handler.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithGroup(name), keys: h.keys}
}

func (h *RedactingHandler) sanitize(attrs ...slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		k := strings.ToLower(a.Key)
		if _, ok := h.keys[k]; ok {
			out = append(out, slog.String(a.Key, "[REDACTED]"))
			continue
		}
		if s, ok := a.Value.Any().(string); ok && looksSensitive(s) {
			out = append(out, slog.String(a.Key, "[REDACTED]"))
			continue
		}
		out = append(out, a)
	}
	return out
}

func looksSensitive(s string) bool {
	if len(s) > 12 && (strings.Contains(s, "sk-") || strings.Contains(strings.ToLower(s), "token")) {
		return true
	}
	return false
}

// MultiHandler combines multiple handlers into one.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to multiple handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Enabled implements slog.Handler.
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle implements slog.Handler.
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

// WithAttrs implements slog.Handler.
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

// WithGroup implements slog.Handler.
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}
