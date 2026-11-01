package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup writes private engine logs to stderr and an optional owner-only file.
func Setup(level, logFile string) (*slog.Logger, func(), error) {
	levels := map[string]slog.Level{
		"": slog.LevelInfo, "debug": slog.LevelDebug, "info": slog.LevelInfo,
		"warn": slog.LevelWarn, "error": slog.LevelError,
	}
	logLevel, valid := levels[level]
	if !valid {
		return nil, nil, fmt.Errorf("invalid log level (must be debug, info, warn, error)")
	}
	var output io.Writer = os.Stderr
	closeLog := func() {}
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return nil, nil, fmt.Errorf("restrict log file permissions: %w", err)
		}
		output = io.MultiWriter(os.Stderr, file)
		closeLog = func() {
			if err := file.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "close log file:", err)
			}
		}
	}
	logger := slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: logLevel, AddSource: logLevel <= slog.LevelDebug,
	}))
	slog.SetDefault(logger)
	return logger, closeLog, nil
}
