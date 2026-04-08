package logger

import (
	"log/slog"
	"os"
)

// Logger wraps slog.Logger to provide a consistent logging interface.
type Logger struct {
	*slog.Logger
}

// New creates a new Logger writing text-formatted logs to stderr at Info level.
func New() *Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return &Logger{slog.New(handler)}
}

// Info logs an info-level message with optional key-value pairs.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

// Error logs an error-level message with optional key-value pairs.
func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

// Debug logs a debug-level message with optional key-value pairs.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}
