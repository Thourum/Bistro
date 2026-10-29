package handler

import (
	"log/slog"
	"os"
)

func testRepoLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
