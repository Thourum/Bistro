package engine

import (
	"context"
	"reflect"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/services/servicestest"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/prsync"
)

// TestStartEventBridge_NilOrchestrator verifies that startEventBridge is a
// safe no-op when no orchestrator has been set.
func TestStartEventBridge_NilOrchestrator(t *testing.T) {
	app := newTestApp()
	// Must not panic.
	app.startEventBridge(context.Background())
}

// TestStartEventBridge_CancelsOldGoroutine verifies that calling
// startEventBridge a second time cancels the previous bridge goroutine so we
// don't accumulate stale goroutines.
func TestStartEventBridge_CancelsOldGoroutine(t *testing.T) {
	app := newTestApp()

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

// ── Oracle blocker: AUTH_DISCONNECT teardown ──────────────────────────────

// TestSetProvider_NilClearsOrchestratorAndApplier verifies that calling
// SetProvider(nil) — the AUTH_DISCONNECT path — clears the orchestrator so
// currentApplier() returns nil and SUBMIT_ACTION is rejected cleanly.
func TestSetProvider_NilClearsOrchestratorAndApplier(t *testing.T) {
	app := newTestApp()

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

func TestBridgeEvent(t *testing.T) {
	for _, test := range []struct {
		typeName string
		data     map[string]any
		name     string
		payload  any
	}{
		{"rate_limited", map[string]any{"reset_in": float64(2.5)}, services.EventRateLimited, services.RateLimited{ResetIn: 2.5}},
		{"PR_METADATA_CHANGED", map[string]any{"pr_id": "pr", "owner": "owner", "repo": "repo", "number": int64(7), "field": "draft", "old": true, "new": false}, services.EventPRMetadataChanged, services.PRMetadataChanged{PRID: "pr", Owner: "owner", Repo: "repo", Number: 7, Field: "draft", Old: "true", New: "false"}},
		{"CI_STATUS_CHANGED", map[string]any{"pr_id": "pr", "owner": "owner", "repo": "repo", "number": float64(7), "old_status": "pending", "new_status": "success"}, services.EventCIStatusChanged, services.CIStatusChanged{PRID: "pr", Owner: "owner", Repo: "repo", Number: 7, OldStatus: "pending", NewStatus: "success"}},
		{"THREAD_RESOLVED", map[string]any{"pr_id": "pr", "thread_id": "thread"}, services.EventThreadStateChanged, services.ThreadStateChanged{PRID: "pr", ThreadID: "thread", Resolved: true}},
		{"THREAD_REOPENED", map[string]any{"pr_id": "pr", "thread_id": "thread", "resolved": true}, services.EventThreadStateChanged, services.ThreadStateChanged{PRID: "pr", ThreadID: "thread"}},
		{"COMMENT_POSTED", map[string]any{"pr_id": "pr", "thread_id": "thread", "in_reply_to": "comment"}, services.EventCommentPosted, services.CommentPosted{PRID: "pr", ThreadID: "thread", InReplyTo: "comment"}},
		{"DRAFTS_CHANGED", map[string]any{"pr_id": "pr"}, services.EventDraftsChanged, services.DraftsChanged{PRID: "pr"}},
		{"action_complete", map[string]any{"pr": "pr", "action": actions.ApproveCommand, "success": true}, services.EventActionComplete, services.ActionComplete{PR: "pr", Action: "approve", Success: true}},
		{"pr_approved", map[string]any{"pr_id": "pr"}, services.EventPRApproved, services.PRApproved{PRID: "pr"}},
		{"sync_started", nil, "", nil},
	} {
		t.Run(test.typeName, func(t *testing.T) {
			name, payload, ok := bridgeEvent(events.Event{Type: test.typeName, Data: test.data})
			if ok != (test.name != "") || name != test.name || !reflect.DeepEqual(payload, test.payload) {
				t.Fatalf("bridgeEvent = (%q, %#v, %v), want (%q, %#v)", name, payload, ok, test.name, test.payload)
			}
		})
	}
}

func TestExpertiseEventsAreTyped(t *testing.T) {
	app := newTestApp()
	recorder := app.emitter.(*servicestest.Recorder)
	app.broadcastExpertiseEvent(expertise.ExpertiseIndexProgressEvent, expertise.IndexProgressPayload{RepoID: "repo", FilesDone: 2, FilesTotal: 3})
	app.broadcastExpertiseEvent(expertise.ExpertiseReadyEvent, expertise.IndexReadyPayload{RepoID: "repo", IndexedSHA: "sha", FilesTotal: 3, SkippedFiles: 1})
	app.broadcastExpertiseEvent(expertise.ExpertiseErrorEvent, expertise.ErrorPayload{RepoID: "repo", Code: "index_error", Message: "failed"})
	want := []servicestest.Event{
		{Name: services.EventExpertiseProgress, Data: services.ExpertiseProgress{RepoID: "repo", FilesDone: 2, FilesTotal: 3}},
		{Name: services.EventExpertiseReady, Data: services.ExpertiseReady{RepoID: "repo", IndexedSHA: "sha", FilesTotal: 3, SkippedFiles: 1}},
		{Name: services.EventExpertiseError, Data: services.ExpertiseError{RepoID: "repo", Code: "index_error", Message: "failed"}},
	}
	if got := recorder.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func TestEventDataNumericKinds(t *testing.T) {
	for _, value := range []any{int(7), int64(7), float64(7)} {
		d := eventData{"number": value}
		if d.int("number") != 7 || d.float("number") != 7 {
			t.Fatalf("numeric conversion failed for %T", value)
		}
	}
	if d := (eventData{}); d.str("missing") != "" || d.int("missing") != 0 || d.float("missing") != 0 {
		t.Fatal("missing values did not default")
	}
}

func TestClassificationEventsAreTyped(t *testing.T) {
	app := newTestApp()
	recorder := app.emitter.(*servicestest.Recorder)
	app.emitClassificationEvent("COMMENTS_READY", map[string]any{"pr_id": "pr"})
	app.emitClassificationEvent("BYOK_AUTH_FAILED", nil)
	app.emitClassificationEvent("BYOK_USAGE", map[string]any{"session_tokens": int64(100), "est_cost": 0.25})
	app.emitClassificationEvent("unused", nil)
	want := []servicestest.Event{
		{Name: services.EventCommentsClassified, Data: services.CommentsClassified{PRID: "pr"}},
		{Name: services.EventByokAuthFailed, Data: services.ByokAuthFailed{}},
		{Name: services.EventByokUsage, Data: services.ByokUsage{SessionTokens: 100, EstCost: 0.25}},
	}
	if got := recorder.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}
