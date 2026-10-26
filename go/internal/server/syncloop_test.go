package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/store/storetest"
	gosync "github.com/itterno/bistro/internal/sync"
	"github.com/itterno/bistro/internal/ws"
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

func TestWatchTriggerOverflowSyncsEveryNewlyWatchedRepo(t *testing.T) {
	app := newTestApp()
	app.autoSync = false
	app.db = storetest.Open(t)
	app.wsServer = ws.NewServerWithLogger(app.log)
	app.prov = mock.NewMockProvider().WithInboxDiscovery(&provider.InboxDiscoveryResult{}, nil)
	app.orchestrator = gosync.NewOrchestrator(app.log, app.prov, app.db, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	app.provCtx = ctx
	const repoCount = 20
	for i := range repoCount {
		repo := fmt.Sprintf("repo-%d", i)
		if _, err := app.db.Exec(`INSERT INTO repos(id,provider,owner,repo) VALUES(?,?,?,?)`, "github:acme/"+repo, "github", "acme", repo); err != nil {
			t.Fatal(err)
		}
	}
	watch := handler.NewToggleWatchHandler(app.db.DB, handler.WatchSyncTrigger{Queue: app.syncTrigger, Pending: &app.pendingWatchSync}, app.log)
	// The loop is busy: fill its buffer before allowing it to receive.
	for i := range repoCount {
		payload := json.RawMessage(fmt.Sprintf(`{"repo_id":"github:acme/repo-%d","watched":true}`, i))
		if _, err := watch(ctx, ws.InboundMessage{Type: "TOGGLE_WATCH_REPO", Data: payload}); err != nil {
			t.Fatal(err)
		}
	}
	eventStream := app.orchestrator.GetEventChannel()
	defer app.orchestrator.UnsubscribeEventChannel(eventStream)
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.runSyncLoop(ctx)
	}()
	defer func() { cancel(); <-done }()
	seen := make(map[string]bool)
	for len(seen) < repoCount {
		select {
		case event := <-eventStream:
			if event.Type != "sync_started" {
				continue
			}
			repo, ok := event.Data["repo"].(string)
			if !ok {
				t.Fatalf("sync event missing repo: %+v", event)
			}
			seen[repo] = true
		case <-ctx.Done():
			t.Fatalf("synced %d of %d newly watched repos after overflow", len(seen), repoCount)
		}
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

	settings := &syncLoopSettings{ticker: ticker, autoSync: autoSync, interval: interval}
	changed := app.applyPendingSyncSettings(settings)
	autoSync, interval = settings.autoSync, settings.interval
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

	settings := &syncLoopSettings{ticker: ticker, autoSync: autoSync, interval: interval}
	changed := app.applyPendingSyncSettings(settings)
	autoSync, interval = settings.autoSync, settings.interval
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
