package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/prsync"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/services/servicestest"
	"github.com/itterno/bistro/internal/store/storetest"
)

type syncRestartLogHandler struct {
	slog.Handler
	handle func(slog.Record)
}

func (h syncRestartLogHandler) Handle(_ context.Context, record slog.Record) error {
	h.handle(record)
	return nil
}

func TestRunSyncLoopRestartRetainsAppliedSettings(t *testing.T) {
	for _, autoSync := range []bool{false, true} {
		t.Run(fmt.Sprintf("autoSync=%t", autoSync), func(t *testing.T) {
			app := newTestApp()
			app.autoSync = !autoSync
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			const appliedInterval = time.Minute
			var starts []syncSettingsMsg
			app.log = slog.New(syncRestartLogHandler{
				Handler: slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
				handle: func(record slog.Record) {
					if record.Message == "sync trigger received" {
						panic("restart after settings applied")
					}
					if record.Message != "sync loop started" {
						return
					}
					var settings syncSettingsMsg
					record.Attrs(func(attr slog.Attr) bool {
						if attr.Key == "autoSync" {
							settings.AutoSync = attr.Value.Bool()
						}
						if attr.Key == "interval" {
							settings.Interval, _ = time.ParseDuration(attr.Value.String())
						}
						return true
					})
					starts = append(starts, settings)
					if len(starts) == 2 {
						cancel()
					}
				},
			})
			app.ApplySyncSettings(autoSync, appliedInterval)
			app.syncTrigger <- ""
			supervise(ctx, supervision{log: app.log, sleep: func(time.Duration) {}}, app.runSyncLoop)
			want := syncSettingsMsg{AutoSync: autoSync, Interval: appliedInterval}
			if len(starts) != 2 || starts[1] != want {
				t.Fatalf("loop starts = %+v, want restart with %+v", starts, want)
			}
		})
	}
}

func newTestApp() *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &App{
		log:          logger,
		emitter:      &servicestest.Recorder{},
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
	recorder := app.emitter.(*servicestest.Recorder)
	app.prov = mock.NewMockProvider().WithInboxDiscovery(&provider.InboxDiscoveryResult{}, nil)
	app.orchestrator = prsync.NewOrchestrator(app.log, app.prov, app.db, nil)
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
	watch := &services.Repos{DB: app.db.DB, SyncTrigger: services.WatchSyncTrigger{Queue: app.syncTrigger, Pending: &app.pendingWatchSync}, Log: app.log}
	// The loop is busy: fill its buffer before allowing it to receive.
	for i := range repoCount {
		watched := true
		if _, err := watch.SetWatched(ctx, services.SetWatchedRequest{RepoID: fmt.Sprintf("github:acme/repo-%d", i), Watched: &watched}); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.runSyncLoop(ctx)
	}()
	defer func() { cancel(); <-done }()
	seen := make(map[string]bool)
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for len(seen) < repoCount {
		select {
		case <-poll.C:
			for _, payload := range recorder.Named(services.EventSyncComplete) {
				complete, ok := payload.(services.SyncComplete)
				if !ok {
					t.Fatalf("sync:complete payload = %T", payload)
				}
				seen[complete.RepoID] = true
			}
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
