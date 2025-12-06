package logger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_DualOutput(t *testing.T) {
	// Create temporary file for testing
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	opts := Options{
		Env:          "prod",
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         logFile,
		App:          "test-app",
	}

	logger := New(opts)
	defer func() {
		err := Close(logger)
		if err != nil {
			t.Errorf("Error closing logger: %v", err)
		}
	}()

	// Test logging at different levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	// Give some time for file writes
	time.Sleep(100 * time.Millisecond)

	// Check that file was created and contains logs
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	fileContent := string(content)

	// File should contain all messages (debug level includes all)
	if !strings.Contains(fileContent, "debug message") {
		t.Error("File should contain debug message")
	}
	if !strings.Contains(fileContent, "info message") {
		t.Error("File should contain info message")
	}
	if !strings.Contains(fileContent, "warn message") {
		t.Error("File should contain warn message")
	}

	// Check JSON format
	if !strings.Contains(fileContent, `"level":"DEBUG"`) {
		t.Error("File should contain JSON formatted debug level")
	}
	if !strings.Contains(fileContent, `"app":"test-app"`) {
		t.Error("File should contain app field")
	}
}

func TestNew_DefaultLevels(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "default.log")

	opts := Options{
		Env:  "prod",
		File: logFile,
		App:  "test-app",
	}

	logger := New(opts)
	defer func() {
		err := Close(logger)
		if err != nil {
			t.Errorf("Error closing logger: %v", err)
		}
	}()

	if logger == nil {
		t.Fatal("Logger should not be nil")
	}

	logger.Debug("debug message")
	logger.Info("info message")

	time.Sleep(100 * time.Millisecond)

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	fileContent := string(content)

	if !strings.Contains(fileContent, "debug message") {
		t.Error("Default file level should include debug messages")
	}
	if !strings.Contains(fileContent, "info message") {
		t.Error("File should contain info message")
	}
}

func TestNew_ConsoleOnly(t *testing.T) {
	// Test console-only mode (no file specified)
	opts := Options{
		Env:          "dev",
		ConsoleLevel: "info",
		App:          "test-app",
	}

	logger := New(opts)
	defer func() {
		err := Close(logger)
		if err != nil {
			t.Errorf("Error closing logger: %v", err)
		}
	}()

	if logger == nil {
		t.Fatal("Logger should not be nil")
	}

	// Should not panic
	logger.Info("console only message")
}

func TestNew_DifferentLevels(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "levels.log")

	opts := Options{
		Env:          "prod",
		ConsoleLevel: "warn",  // Only warn and error to console
		FileLevel:    "debug", // All levels to file
		File:         logFile,
		App:          "test-app",
	}

	logger := New(opts)
	defer func() {
		err := Close(logger)
		if err != nil {
			t.Errorf("Error closing logger: %v", err)
		}
	}()

	// Log messages at different levels
	logger.Debug("debug only in file")
	logger.Info("info only in file")
	logger.Warn("warn in both")
	logger.Error("error in both")

	// Give time for file writes
	time.Sleep(100 * time.Millisecond)

	// Check file contains all messages
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	fileContent := string(content)

	if !strings.Contains(fileContent, "debug only in file") {
		t.Error("File should contain debug message")
	}
	if !strings.Contains(fileContent, "info only in file") {
		t.Error("File should contain info message")
	}
	if !strings.Contains(fileContent, "warn in both") {
		t.Error("File should contain warn message")
	}
	if !strings.Contains(fileContent, "error in both") {
		t.Error("File should contain error message")
	}
}

func TestRedactingHandler(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "redacted.log")

	opts := Options{
		Env:       "prod",
		FileLevel: "debug",
		File:      logFile,
		App:       "test-app",
	}

	logger := New(opts)
	defer func() {
		err := Close(logger)
		if err != nil {
			t.Errorf("Error closing logger: %v", err)
		}
	}()

	// Log sensitive data
	logger.Info("api call", slog.String("token", "sk-1234567890abcdef"), slog.String("user", "john"))

	time.Sleep(100 * time.Millisecond)

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	fileContent := string(content)

	if strings.Contains(fileContent, "sk-1234567890abcdef") {
		t.Error("Sensitive token should be redacted")
	}
	if !strings.Contains(fileContent, "[REDACTED]") {
		t.Error("Should contain redacted placeholder")
	}
	if !strings.Contains(fileContent, "john") {
		t.Error("Non-sensitive data should not be redacted")
	}
}

func TestMultiHandler(t *testing.T) {
	h1 := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	h2 := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn})

	multi := NewMultiHandler(h1, h2)

	ctx := context.Background()

	// Test Enabled
	if !multi.Enabled(ctx, slog.LevelInfo) {
		t.Error("Should be enabled for info level")
	}
	if !multi.Enabled(ctx, slog.LevelWarn) {
		t.Error("Should be enabled for warn level")
	}

	// Test Handle (should not panic)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := multi.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Test WithAttrs
	withAttrs := multi.WithAttrs([]slog.Attr{slog.String("key", "value")})
	if withAttrs == nil {
		t.Error("WithAttrs should not return nil")
	}

	// Test WithGroup
	withGroup := multi.WithGroup("group")
	if withGroup == nil {
		t.Error("WithGroup should not return nil")
	}
}
