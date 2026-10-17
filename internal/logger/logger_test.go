package logger

import (
	"bytes"
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()

	logger := NewLogger(false)
	if logger == nil {
		t.Fatalf("expected logger, got nil")
	}
}

func TestNewLoggerWithWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.DebugLevel, &buf)
	if logger == nil {
		t.Fatalf("expected logger, got nil")
	}
}

func TestLoggerDebug(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.DebugLevel, &buf)

	logger.Debug("debug message")

	// Debug messages are written at debug level
	output := buf.String()
	if output == "" {
		t.Fatalf("expected debug output")
	}
}

func TestLoggerDebugAtWarnLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.WarnLevel, &buf)

	logger.Debug("debug message")

	// Debug messages should not be written at warn level
	output := buf.String()
	if output != "" {
		t.Fatalf("expected no debug output at warn level, got %s", output)
	}
}

func TestLoggerInfo(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.InfoLevel, &buf)

	logger.Info("info message")

	output := buf.String()
	if output == "" {
		t.Fatalf("expected info output")
	}
}

func TestLoggerWarning(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.InfoLevel, &buf)

	logger.Warning("warning message")

	output := buf.String()
	if output == "" {
		t.Fatalf("expected warning output")
	}
}

func TestLoggerError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.InfoLevel, &buf)

	logger.Error("error message", nil)

	output := buf.String()
	if output == "" {
		t.Fatalf("expected error output")
	}
}

func TestLoggerErrorWithError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.InfoLevel, &buf)

	err := os.ErrNotExist
	logger.Error("error message", err)

	output := buf.String()
	if output == "" {
		t.Fatalf("expected error output")
	}
}

func TestLoggerStderr(t *testing.T) {
	t.Parallel()

	// Test default logger writes to stderr
	logger := NewLogger(true)

	// This should not panic
	logger.Info("test message")
}

func TestLoggerVerboseFalse(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLoggerWithWriter(zerolog.WarnLevel, &buf)

	// These should not write anything at warn level except warnings and errors
	logger.Debug("debug")
	logger.Info("info")
	logger.Warning("warning")
	logger.Error("error", nil)

	// Only warnings and errors should be written at warn level
	output := buf.String()

	if output == "" {
		t.Fatalf("expected some output for warning/error")
	}
}
