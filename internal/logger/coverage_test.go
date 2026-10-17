package logger

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoggerContract(t *testing.T) {
	var output bytes.Buffer
	lg := NewLoggerWithWriter(zerolog.DebugLevel, &output)
	lg.Debug("debug")
	lg.Info("info")
	lg.Warning("warning")
	lg.Error("error", nil)
	if output.Len() == 0 {
		t.Fatal("logger produced no output")
	}
	if NewLogger(false) == nil || NewLogger(true) == nil {
		t.Fatal("NewLogger returned nil")
	}
}
