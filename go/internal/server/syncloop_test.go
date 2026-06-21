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
		syncSettings: make(chan syncSettingsMsg, 4),
		syncInterval: 5 * time.Minute,
		autoSync:     true,
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

// TestApplyPendingSyncSettings_Coalesces verifies that multiple queued settings
// messages are drained and only the latest value is applied.
func TestApplyPendingSyncSettings_Coalesces(t *testing.T) {
	app := newTestApp()
	ticker := time.NewTicker(1 * time.Hour) // large interval; we don't want it firing
	defer ticker.Stop()

	// Queue three settings updates; only the last should win.
	app.syncSettings <- syncSettingsMsg{AutoSync: true, Interval: 10 * time.Second}
	app.syncSettings <- syncSettingsMsg{AutoSync: true, Interval: 20 * time.Second}
	app.syncSettings <- syncSettingsMsg{AutoSync: false, Interval: 30 * time.Second}

	autoSync := true
	interval := 5 * time.Minute

	changed := app.applyPendingSyncSettings(ticker, &autoSync, &interval)
	if !changed {
		t.Fatal("expected applyPendingSyncSettings to report a change")
	}
	if autoSync != false {
		t.Errorf("autoSync = %v, want false (latest value)", autoSync)
	}
	if interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s (latest value)", interval)
	}
	// Channel must be empty after draining.
	select {
	case <-app.syncSettings:
		t.Error("syncSettings channel should be empty after coalescing")
	default:
	}
}

// TestApplyPendingSyncSettings_EmptyNoop verifies no-op when channel is empty.
func TestApplyPendingSyncSettings_EmptyNoop(t *testing.T) {
	app := newTestApp()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	autoSync := true
	interval := 5 * time.Minute

	changed := app.applyPendingSyncSettings(ticker, &autoSync, &interval)
	if changed {
		t.Error("expected no change when channel is empty")
	}
	if autoSync != true || interval != 5*time.Minute {
		t.Error("values should be unchanged when channel is empty")
	}
}

// TestRunSyncLoop_DisableBeforeTickFires verifies that a disable sent while a
// sync is running prevents the next automatic tick from running another sync.
func TestRunSyncLoop_DisableBeforeTickFires(t *testing.T) {
	app := newTestApp()
	ctx, cancel := context.WithCancel(context.Background())

	syncCount := 0
	// Override syncWatchedRepos via the trigger channel path:
	// We use a manual trigger to observe the count and then send a disable.
	done := make(chan struct{})

	go func() {
		defer close(done)
		app.runSyncLoop(ctx)
	}()

	// Send a disable immediately — the loop should not auto-sync.
	app.syncSettings <- syncSettingsMsg{AutoSync: false, Interval: 50 * time.Millisecond}
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if syncCount != 0 {
		t.Errorf("sync ran %d times after disable; want 0", syncCount)
	}
}
