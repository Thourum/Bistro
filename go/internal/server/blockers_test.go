package server

import (
	"context"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
	gosync "github.com/itterno/bistro/internal/sync"
	"github.com/itterno/bistro/internal/ws"
)

// newFullTestApp returns an App pre-loaded with an initialised provCtx
// (as NewWithLogger does) so blocker tests don't panic on nil context.
func newFullTestApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp()
	provCtx, provCtxCancel := context.WithCancel(context.Background())
	app.provMu.Lock()
	app.provCtx = provCtx
	app.provCtxCancel = provCtxCancel
	app.provMu.Unlock()
	app.wsServer = ws.NewServerWithLogger(app.log)
	return app
}

// openTestServerDB opens a migrated in-memory SQLite DB for server-package tests.
func openTestServerDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("openTestServerDB: %v", err)
	}
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("openTestServerDB migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ── Blocker 1: keychain startup + AUTH_DISCONNECT path ────────────────────

// TestTeardownProviderLocked_NameMatch verifies teardown clears state when name matches.
func TestTeardownProviderLocked_NameMatch(t *testing.T) {
	app := newFullTestApp(t)
	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	app.provMu.Lock()
	app.prov = mock.NewMockProvider()
	app.orchestrator = gosync.NewOrchestrator(nil, nil, nil, nil)
	app.activeProviderName = "github"
	app.bridgeCancel = bridgeCancel
	app.provMu.Unlock()

	app.provMu.Lock()
	app.teardownProviderLocked("github")
	app.provMu.Unlock()

	select {
	case <-bridgeCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("bridge should be cancelled after teardown")
	}
	app.provMu.RLock()
	prov := app.prov
	orch := app.orchestrator
	name := app.activeProviderName
	app.provMu.RUnlock()
	if prov != nil || orch != nil || name != "" {
		t.Errorf("prov/orch/name should be nil/empty; got prov=%v orch=%v name=%q", prov, orch, name)
	}
}

// TestTeardownProviderLocked_NameMismatch verifies wrong name is a no-op.
func TestTeardownProviderLocked_NameMismatch(t *testing.T) {
	app := newFullTestApp(t)
	app.provMu.Lock()
	app.prov = mock.NewMockProvider()
	app.orchestrator = gosync.NewOrchestrator(nil, nil, nil, nil)
	app.activeProviderName = "github"
	app.provMu.Unlock()

	app.provMu.Lock()
	app.teardownProviderLocked("github2")
	app.provMu.Unlock()

	app.provMu.RLock()
	prov := app.prov
	name := app.activeProviderName
	app.provMu.RUnlock()
	if prov == nil || name != "github" {
		t.Error("wrong-name teardown must not clear the active provider")
	}
}

// TestKeychainStartup_DisconnectByName is the blocker-1 regression test.
// External SetProvider(prov) before Run must record activeProviderName="github"
// so AUTH_DISCONNECT (name="github") can tear down the session.
func TestKeychainStartup_DisconnectByName(t *testing.T) {
	app := newFullTestApp(t)

	// Simulate main.go: SetProvider before Run (db is nil, no orch created).
	app.SetProvider(mock.NewMockProvider())

	app.provMu.RLock()
	name := app.activeProviderName
	app.provMu.RUnlock()
	if name != "github" {
		t.Errorf("activeProviderName = %q after SetProvider(prov), want \"github\"", name)
	}

	// Simulate what Run's startup block leaves in App.
	app.provMu.Lock()
	app.prov = mock.NewMockProvider()
	app.orchestrator = gosync.NewOrchestrator(nil, nil, nil, nil)
	app.provMu.Unlock()

	// AUTH_DISCONNECT fires with name="github" — must clear everything.
	app.provMu.Lock()
	app.teardownProviderLocked("github")
	app.provMu.Unlock()

	if app.currentApplier() != nil {
		t.Error("currentApplier must return nil after keychain-startup disconnect")
	}
}

// ── Blocker 2: action queue + liveness wired via attachOrchestratorLocked ──

// TestAttachOrchestratorLocked_LivenessWired verifies that orchestrators built
// via attachOrchestratorLocked have WithLiveness wired: when provCtx is
// cancelled, ApplyAction must reject rather than execute with stale credentials.
func TestAttachOrchestratorLocked_LivenessWired(t *testing.T) {
	db := openTestServerDB(t)
	app := newFullTestApp(t)
	app.db = db

	provCtx, provCtxCancel := context.WithCancel(context.Background())
	app.provMu.Lock()
	app.attachOrchestratorLocked(provCtx, mock.NewMockProvider(), "github")
	orch := app.orchestrator
	app.provMu.Unlock()

	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}

	// Cancel liveness — simulates provider disconnect.
	provCtxCancel()
	time.Sleep(5 * time.Millisecond)

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{
		PRID:   "github:owner/repo:1",
		Action: actions.ApproveCommand,
	})
	if result.Success {
		t.Error("expected failure after liveness cancel")
	}
	if result.Error != "provider disconnected: action rejected" {
		t.Errorf("unexpected error: %q", result.Error)
	}
}

// ── Blocker 4: liveness check rejects stale orchestrator ──────────────────

// TestApplyAction_LivenessRejected verifies that a handler holding a stale
// orchestrator reference (captured before disconnect) is rejected cleanly.
func TestApplyAction_LivenessRejected(t *testing.T) {
	liveCtx, cancelLiveness := context.WithCancel(context.Background())
	orch := gosync.NewOrchestrator(nil, nil, nil, nil).
		WithLiveness(liveCtx)

	cancelLiveness() // simulate disconnect after handler captured orch

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{
		PRID:   "github:owner/repo:1",
		Action: actions.ApproveCommand,
	})
	if result.Success {
		t.Error("expected failure when liveness is cancelled")
	}
	if result.Error != "provider disconnected: action rejected" {
		t.Errorf("unexpected error: %q", result.Error)
	}
}

// ── Blocker 5: stale sync suppressed after disconnect ─────────────────────

// TestProvCtx_CancelledOnDisconnect verifies that SetProvider(nil) cancels the
// per-provider context so any sync goroutine that captured it before disconnect
// will observe ctx.Err() != nil and suppress stale broadcasts.
func TestProvCtx_CancelledOnDisconnect(t *testing.T) {
	app := newFullTestApp(t)

	app.provMu.Lock()
	app.prov = mock.NewMockProvider()
	app.orchestrator = gosync.NewOrchestrator(nil, nil, nil, nil)
	app.activeProviderName = "github"
	capturedCtx := app.provCtx
	app.provMu.Unlock()

	if capturedCtx == nil {
		t.Fatal("provCtx should be non-nil before disconnect")
	}

	app.SetProvider(nil)

	select {
	case <-capturedCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("provCtx should be cancelled after SetProvider(nil)")
	}

	// After disconnect a new provCtx must be ready for the next connect.
	app.provMu.RLock()
	newCtx := app.provCtx
	app.provMu.RUnlock()
	if newCtx == nil {
		t.Error("provCtx should be re-initialised after disconnect")
	}
	if newCtx == capturedCtx {
		t.Error("provCtx should be a fresh context after disconnect")
	}
}

// ── Blocker 1: SetReplayOnStart defers replay into Run ────────────────────

// TestSetReplayOnStart_FlagStoredOnApp verifies that SetReplayOnStart stores
// the flag so Run() can act on it (the meaningful contract is that pre-Run
// calls no longer attempt early replay against a nil orchestrator).
func TestSetReplayOnStart_FlagStoredOnApp(t *testing.T) {
	app := newFullTestApp(t)
	if app.replayOnStart {
		t.Error("replayOnStart should default to false")
	}
	app.SetReplayOnStart(true)
	if !app.replayOnStart {
		t.Error("replayOnStart should be true after SetReplayOnStart(true)")
	}
	app.SetReplayOnStart(false)
	if app.replayOnStart {
		t.Error("replayOnStart should be false after SetReplayOnStart(false)")
	}
}

// TestReplayQueuedActions_NilOrchestratorIsNoop verifies the pre-Run no-op
// is still safe (not a panic) so callers that still call it before Run don't
// break. The documented advice is to use SetReplayOnStart instead.
func TestReplayQueuedActions_NilOrchestratorIsNoop(t *testing.T) {
	app := newFullTestApp(t)
	if err := app.ReplayQueuedActions(context.Background()); err != nil {
		t.Errorf("ReplayQueuedActions with nil orchestrator must return nil, got: %v", err)
	}
}
