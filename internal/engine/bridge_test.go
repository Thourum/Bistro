package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/expertise"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/prsync"
	"github.com/itterno/bistro/internal/ws"
)

// TestStartEventBridge_NilOrchestrator verifies that startEventBridge is a
// safe no-op when no orchestrator has been set.
func TestStartEventBridge_NilOrchestrator(t *testing.T) {
	app := newTestApp()
	app.wsServer = ws.NewServerWithLogger(app.log)
	// Must not panic.
	app.startEventBridge(context.Background())
}

// TestStartEventBridge_CancelsOldGoroutine verifies that calling
// startEventBridge a second time cancels the previous bridge goroutine so we
// don't accumulate stale goroutines.
func TestStartEventBridge_CancelsOldGoroutine(t *testing.T) {
	app := newTestApp()
	app.wsServer = ws.NewServerWithLogger(app.log)

	emitter := events.NewEventEmitter()

	// Stub: an orchestrator-shaped value with a real emitter.
	sub := emitter.Subscribe()
	_ = sub // first bridge will drain this channel

	// Wire a first bridge using a tiny helper that bypasses orchestrator.
	ctx1, cancel1 := context.WithCancel(context.Background())
	app.bridgeCancel = cancel1
	_ = ctx1

	// A second startEventBridge call should cancel the first cancel func.
	// We verify by checking cancel1's context is done after the call.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	// Assign a real orchestrator-less bridge (nil orchestrator path).
	app.bridgeCancel = cancel1 // simulate a live bridge
	app.orchestrator = nil
	app.startEventBridge(ctx2)

	select {
	case <-ctx1.Done():
		// Good: old context was cancelled.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected old bridge context to be cancelled")
	}
}

// TestWsEventMsg_MarshalShape verifies that wsEventMsg serialises into the
// expected {"type":..., "data":...} envelope that WS clients expect.
func TestBroadcastExpertiseEventFlattensTypedPayload(t *testing.T) {
	data := expertisePayloadDataForTest(expertiseReadyPayloadForTest())
	if _, ok := data["payload"]; ok {
		t.Fatalf("typed expertise event data contains payload wrapper: %#v", data)
	}
	if data["repo_id"] != "github:owner/repo" {
		t.Fatalf("repo_id = %v, want github:owner/repo", data["repo_id"])
	}
	if data["indexed_sha"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("indexed_sha = %v, want sha", data["indexed_sha"])
	}
	if data["files_total"] != float64(3) {
		t.Fatalf("files_total = %v, want 3", data["files_total"])
	}
}

func expertiseReadyPayloadForTest() any {
	return struct {
		RepoID     string `json:"repo_id"`
		IndexedSHA string `json:"indexed_sha"`
		FilesTotal int    `json:"files_total"`
	}{
		RepoID:     "github:owner/repo",
		IndexedSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FilesTotal: 3,
	}
}

func expertisePayloadDataForTest(payload any) map[string]interface{} {
	return expertisePayloadToMap(payload)
}

var _ = expertise.ExpertiseStatusReady

func TestWsEventMsg_MarshalShape(t *testing.T) {
	msg := wsEventMsg{
		Type: "THREAD_RESOLVED",
		Data: map[string]interface{}{
			"thread_id": "abc123",
		},
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "THREAD_RESOLVED" {
		t.Errorf("type = %v, want THREAD_RESOLVED", got["type"])
	}
	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map: %T", got["data"])
	}
	if data["thread_id"] != "abc123" {
		t.Errorf("data.thread_id = %v, want abc123", data["thread_id"])
	}
}

// ── Oracle blocker: AUTH_DISCONNECT teardown ──────────────────────────────

// TestSetProvider_NilClearsOrchestratorAndApplier verifies that calling
// SetProvider(nil) — the AUTH_DISCONNECT path — clears the orchestrator so
// currentApplier() returns nil and SUBMIT_ACTION is rejected cleanly.
func TestSetProvider_NilClearsOrchestratorAndApplier(t *testing.T) {
	app := newTestApp()
	app.wsServer = ws.NewServerWithLogger(app.log)

	// Simulate a connected state: non-nil prov stub and orchestrator.
	app.provMu.Lock()
	app.prov = mock.NewMockProvider()
	app.orchestrator = prsync.NewOrchestrator(nil, nil, nil, nil)
	app.provMu.Unlock()

	// currentApplier must return non-nil while connected.
	if app.currentApplier() == nil {
		t.Fatal("expected non-nil applier before disconnect")
	}

	// Disconnect.
	app.SetProvider(nil)

	// Both must be nil after disconnect.
	app.provMu.RLock()
	provAfter := app.prov
	orchAfter := app.orchestrator
	app.provMu.RUnlock()

	if provAfter != nil {
		t.Error("prov should be nil after SetProvider(nil)")
	}
	if orchAfter != nil {
		t.Error("orchestrator should be nil after SetProvider(nil)")
	}

	// currentApplier must return nil — SUBMIT_ACTION will be rejected.
	if app.currentApplier() != nil {
		t.Error("currentApplier() must return nil after disconnect")
	}
}

// TestSetProvider_NilCancelsBridge verifies that AUTH_DISCONNECT cancels the
// active event bridge goroutine so it stops broadcasting stale events.
func TestSetProvider_NilCancelsBridge(t *testing.T) {
	app := newTestApp()
	app.wsServer = ws.NewServerWithLogger(app.log)

	// Install a live bridge cancel func.
	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	app.provMu.Lock()
	app.prov = mock.NewMockProvider()
	app.orchestrator = prsync.NewOrchestrator(nil, nil, nil, nil)
	app.bridgeCancel = bridgeCancel
	app.provMu.Unlock()

	// Disconnect.
	app.SetProvider(nil)

	// The bridge context must be done promptly.
	select {
	case <-bridgeCtx.Done():
		// Good.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected bridge to be cancelled after SetProvider(nil)")
	}

	// bridgeCancel must be cleared so re-connecting creates a fresh bridge.
	app.provMu.RLock()
	bc := app.bridgeCancel
	app.provMu.RUnlock()
	if bc != nil {
		t.Error("bridgeCancel should be nil after disconnect")
	}
}

// TestCurrentApplier_RequiresBothProvAndOrchestrator verifies the double-nil
// guard: currentApplier returns nil if prov is nil (even if orchestrator is
// somehow non-nil) and vice-versa.
func TestCurrentApplier_RequiresBothProvAndOrchestrator(t *testing.T) {
	app := newTestApp()

	// Only orchestrator set — prov nil.
	app.provMu.Lock()
	app.orchestrator = prsync.NewOrchestrator(nil, nil, nil, nil)
	app.provMu.Unlock()
	if app.currentApplier() != nil {
		t.Error("currentApplier must return nil when prov is nil")
	}

	// Only prov set — orchestrator nil.
	app.provMu.Lock()
	app.orchestrator = nil
	app.prov = mock.NewMockProvider()
	app.provMu.Unlock()
	if app.currentApplier() != nil {
		t.Error("currentApplier must return nil when orchestrator is nil")
	}

	// Both set — should return non-nil.
	app.provMu.Lock()
	app.orchestrator = prsync.NewOrchestrator(nil, nil, nil, nil)
	app.provMu.Unlock()
	if app.currentApplier() == nil {
		t.Error("currentApplier must return non-nil when both prov and orchestrator are set")
	}
}
