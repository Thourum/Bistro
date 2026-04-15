package server

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func newTestApp() *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &App{
		log:          logger,
		syncTrigger:  make(chan string, 8),
		syncInterval: 5 * time.Minute,
	}
}

func TestSyncWatchedRepos_NilOrchestrator(t *testing.T) {
	app := newTestApp()
	// orchestrator is nil — must not panic
	app.syncWatchedRepos(context.Background())
}

func TestSyncWatchedRepos_NilProvider(t *testing.T) {
	app := newTestApp()
	// prov is nil (zero value) — must not panic
	app.syncWatchedRepos(context.Background())
}

func TestSyncSingleRepo_InvalidFormat_NoColon(t *testing.T) {
	app := newTestApp()
	// Should log an error and return without panic.
	app.syncSingleRepo(context.Background(), "nocolon")
}

func TestSyncSingleRepo_InvalidFormat_NoSlash(t *testing.T) {
	app := newTestApp()
	// Should log an error and return without panic.
	app.syncSingleRepo(context.Background(), "github:noslash")
}

func TestSyncSingleRepo_NilOrchestratorNoOp(t *testing.T) {
	app := newTestApp()
	// Valid format but orchestrator is nil — must not panic.
	app.syncSingleRepo(context.Background(), "github:owner/repo")
}
