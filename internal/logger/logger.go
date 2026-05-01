package logger

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warning(msg string)
	Error(msg string, err error)
}

type logger struct {
	zl zerolog.Logger
}

func NewLogger(verbose bool) Logger {
	level := zerolog.WarnLevel
	if verbose {
		level = zerolog.DebugLevel
	}
	return NewLoggerWithWriter(level, os.Stderr)
}

func NewLoggerWithWriter(level zerolog.Level, w io.Writer) Logger {
	zl := zerolog.New(w).Level(level).With().Timestamp().Logger()
	return &logger{zl: zl}
}

func (l *logger) Debug(msg string) {
	l.zl.Debug().Msg(msg)
}

func (l *logger) Info(msg string) {
	l.zl.Info().Msg(msg)
}

func (l *logger) Warning(msg string) {
	l.zl.Warn().Msg(msg)
}

func (l *logger) Error(msg string, err error) {
	l.zl.Error().Err(err).Msg(msg)
}
