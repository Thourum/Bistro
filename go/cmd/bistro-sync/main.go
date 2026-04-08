package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/samuelasvanyi/bistro/internal/server"
)

func main() {
	wsPort := flag.Int("ws-port", 8080, "WebSocket server port")
	dbPath := flag.String("db-path", "./data/sync.db", "SQLite database path")
	replayQueue := flag.Bool("replay-queue", true, "Replay offline queue on startup")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logFile := flag.String("log-file", "", "Log file path (default: stderr only)")
	flag.Parse()

	// Setup logging
	if err := setupLogging(*logLevel, *logFile); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup logging: %v\n", err)
		os.Exit(1)
	}

	log := slog.Default()
	log.Info("bistro-sync starting",
		"ws-port", *wsPort,
		"db-path", *dbPath,
		"log-level", *logLevel,
		"log-file", *logFile,
	)

	app := server.NewWithLogger(*wsPort, *dbPath, log)

	if *replayQueue {
		log.Info("replaying offline queue")
		if err := app.ReplayQueuedActions(context.Background()); err != nil {
			log.Error("queue replay failed", "error", err)
		}
	}

	if err := app.Run(context.Background()); err != nil {
		log.Error("app error", "error", err)
		os.Exit(1)
	}
}

// setupLogging configures slog with the given level and optional file output.
func setupLogging(level, logFile string) error {
	// Parse log level
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, error)", level)
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		AddSource: logLevel == slog.LevelDebug, // Include source file:line for debug
	}

	var out io.Writer = os.Stderr

	// If log file specified, write to both stderr and file
	if logFile != "" {
		// Ensure log directory exists
		logDir := filepath.Dir(logFile)
		if logDir != "." && logDir != "" {
			if err := os.MkdirAll(logDir, 0755); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
		}

		// Open log file for appending
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		// Log to both stderr and file
		out = io.MultiWriter(os.Stderr, f)

		// Log file location to stderr immediately
		fmt.Fprintf(os.Stderr, "Logs written to: %s\n", logFile)
	}

	handler := slog.NewTextHandler(out, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
