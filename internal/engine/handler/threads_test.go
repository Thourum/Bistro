package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// ── mock helpers ─────────────────────────────────────────────────────────

type mockThreadLister struct {
	threads []store.ReviewThreadRow
	err     error
}

func (m *mockThreadLister) ListReviewThreads(_ context.Context, _ string) ([]store.ReviewThreadRow, error) {
	return m.threads, m.err
}

type mockActionApplier struct {
	result *actions.ActionResult
}

func (m *mockActionApplier) ApplyAction(_ context.Context, req *actions.ActionRequest) *actions.ActionResult {
	if m.result != nil {
		return m.result
	}
	return &actions.ActionResult{PRID: req.PRID, Action: req.Action, Success: true}
}

// staticGetter wraps a fixed ActionApplier into an ApplierGetter.
func staticGetter(a ActionApplier) ApplierGetter {
	return func() ActionApplier { return a }
}

// nilGetter always returns nil (simulates unauthenticated state).
func nilGetter() ApplierGetter {
	return func() ActionApplier { return nil }
}

// ── GET_REVIEW_THREADS ───────────────────────────────────────────────────

func TestGetReviewThreadsHandler_ReturnsList(t *testing.T) {
	lister := &mockThreadLister{threads: []store.ReviewThreadRow{
		{ID: "rt-1", PRID: "pr-1", FilePath: "main.go", Line: 5, Resolved: false, StateKnown: true},
		{ID: "rt-2", PRID: "pr-1", FilePath: "util.go", Line: 10, Resolved: true, StateKnown: true},
	}}
	h := NewGetReviewThreadsHandler(lister, testRepoLogger())
	data, _ := json.Marshal(map[string]string{"pr_id": "pr-1"})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r1", Type: "GET_REVIEW_THREADS", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "REVIEW_THREADS_READY" {
		t.Errorf("type = %q, want REVIEW_THREADS_READY", env["type"])
	}
	d := env["data"].(map[string]interface{})
	threads := d["threads"].([]store.ReviewThreadRow)
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	if threads[0].ID != "rt-1" || threads[1].ID != "rt-2" {
		t.Errorf("unexpected thread IDs: %v %v", threads[0].ID, threads[1].ID)
	}
}

func TestGetReviewThreadsHandler_EmptyList(t *testing.T) {
	h := NewGetReviewThreadsHandler(&mockThreadLister{}, testRepoLogger())
	data, _ := json.Marshal(map[string]string{"pr_id": "pr-empty"})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r2", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	d := resp.(map[string]interface{})["data"].(map[string]interface{})
	threads := d["threads"].([]store.ReviewThreadRow)
	if len(threads) != 0 {
		t.Errorf("expected empty list, got %d", len(threads))
	}
}

func TestGetReviewThreadsHandler_MissingPRID(t *testing.T) {
	h := NewGetReviewThreadsHandler(&mockThreadLister{}, testRepoLogger())
	data, _ := json.Marshal(map[string]string{})
	_, err := h(context.Background(), ws.InboundMessage{ID: "r3", Data: data})
	if err == nil {
		t.Error("expected error for missing pr_id")
	}
}

func TestGetReviewThreadsHandler_DBError(t *testing.T) {
	lister := &mockThreadLister{err: errors.New("db unavailable")}
	h := NewGetReviewThreadsHandler(lister, testRepoLogger())
	data, _ := json.Marshal(map[string]string{"pr_id": "pr-1"})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r4", Data: data})
	if err != nil {
		t.Fatalf("handler must not return error on DB failure: %v", err)
	}
	d := resp.(map[string]interface{})["data"].(map[string]interface{})
	if d["error"] == nil {
		t.Error("expected error field in response")
	}
}

// ── SUBMIT_ACTION (thread) ────────────────────────────────────────────────

func TestSubmitActionHandler_ResolveThread(t *testing.T) {
	applier := &mockActionApplier{}
	h := NewSubmitActionHandler(staticGetter(applier), testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"action":    "resolve_thread",
		"pr_id":     "pr-1",
		"thread_id": "PRRT_abc",
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r5", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "ACTION_RESULT" {
		t.Errorf("type = %q, want ACTION_RESULT", env["type"])
	}
	d := env["data"].(map[string]interface{})
	if d["success"] != true {
		t.Errorf("success = %v, want true", d["success"])
	}
}

func TestSubmitActionHandler_ThreadIDMappedToBody(t *testing.T) {
	// thread_id must be mapped to Body so the action handler receives it.
	var captured *actions.ActionRequest
	applier := &captureApplier{capture: &captured}
	h := NewSubmitActionHandler(staticGetter(applier), testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"action":    "resolve_thread",
		"pr_id":     "pr-1",
		"thread_id": "PRRT_node123",
	})
	h(context.Background(), ws.InboundMessage{ID: "r6", Data: data}) //nolint:errcheck
	if captured == nil || captured.Body != "PRRT_node123" {
		t.Errorf("expected Body=PRRT_node123, got: %v", captured)
	}
}

func TestSubmitActionHandler_ReplyCommentKeepsBodyAndMapsCommentID(t *testing.T) {
	var captured *actions.ActionRequest
	applier := &captureApplier{capture: &captured}
	h := NewSubmitActionHandler(staticGetter(applier), testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"action":     "reply_comment",
		"pr_id":      "pr-1",
		"body":       "Reply body",
		"comment_id": "github-review-comment-123",
	})
	h(context.Background(), ws.InboundMessage{ID: "reply-1", Data: data}) //nolint:errcheck
	if captured == nil {
		t.Fatal("expected captured action request")
	}
	if captured.Body != "Reply body" {
		t.Errorf("expected Body to remain reply text, got %q", captured.Body)
	}
	if captured.InReplyTo != "github-review-comment-123" {
		t.Errorf("expected InReplyTo mapped from comment_id, got %q", captured.InReplyTo)
	}
}

func TestSubmitActionHandler_ReplyCommentKeepsThreadIDSeparateFromBody(t *testing.T) {
	var captured *actions.ActionRequest
	applier := &captureApplier{capture: &captured}
	h := NewSubmitActionHandler(staticGetter(applier), testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"action":    "reply_comment",
		"pr_id":     "pr-1",
		"body":      "Reply body",
		"thread_id": "PRRT_abc",
	})
	h(context.Background(), ws.InboundMessage{ID: "reply-2", Data: data}) //nolint:errcheck
	if captured == nil {
		t.Fatal("expected captured action request")
	}
	if captured.Body != "Reply body" {
		t.Errorf("expected Body to remain reply text, got %q", captured.Body)
	}
	if captured.ThreadID != "PRRT_abc" {
		t.Errorf("expected ThreadID mapped, got %q", captured.ThreadID)
	}
}

func TestSubmitActionHandler_MissingAction(t *testing.T) {
	h := NewSubmitActionHandler(staticGetter(&mockActionApplier{}), testRepoLogger())
	data, _ := json.Marshal(map[string]string{"pr_id": "pr-1"})
	_, err := h(context.Background(), ws.InboundMessage{ID: "r7", Data: data})
	if err == nil {
		t.Error("expected error for missing action")
	}
}

func TestSubmitActionHandler_ValidationError(t *testing.T) {
	// resolve_thread without body/thread_id should fail validation.
	h := NewSubmitActionHandler(staticGetter(&mockActionApplier{}), testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"action": "resolve_thread",
		"pr_id":  "pr-1",
		// no body or thread_id
	})
	_, err := h(context.Background(), ws.InboundMessage{ID: "r8", Data: data})
	if err == nil {
		t.Error("expected validation error for missing thread_id")
	}
}

// ── Blocker 1: SUBMIT_ACTION dynamic getter (nil provider) ────────────────

// TestSubmitActionHandler_NilGetter verifies that when no provider is
// authenticated (getter returns nil), the handler returns a clean error
// instead of panicking.
func TestSubmitActionHandler_NilGetter(t *testing.T) {
	h := NewSubmitActionHandler(nilGetter(), testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"action":    "resolve_thread",
		"pr_id":     "pr-1",
		"thread_id": "PRRT_abc",
	})
	_, err := h(context.Background(), ws.InboundMessage{ID: "r9", Data: data})
	if err == nil {
		t.Fatal("expected error when getter returns nil")
	}
	if err.Error() == "" {
		t.Error("error message should be non-empty")
	}
}

// TestSubmitActionHandler_DynamicGetter verifies the getter is called per
// request, so a newly set orchestrator is used without re-registration.
func TestSubmitActionHandler_DynamicGetter(t *testing.T) {
	var current ActionApplier
	getter := func() ActionApplier { return current }
	h := NewSubmitActionHandler(getter, testRepoLogger())

	data, _ := json.Marshal(map[string]string{
		"action":    "resolve_thread",
		"pr_id":     "pr-1",
		"thread_id": "PRRT_abc",
	})
	msg := ws.InboundMessage{ID: "r10", Data: data}

	// First call: nil applier → error.
	_, err := h(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when getter returns nil initially")
	}

	// Second call after provider set: success.
	current = &mockActionApplier{}
	resp, err := h(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected success after provider set, got: %v", err)
	}
	d := resp.(map[string]interface{})["data"].(map[string]interface{})
	if d["success"] != true {
		t.Errorf("expected success=true, got %v", d["success"])
	}
}

type captureApplier struct {
	capture **actions.ActionRequest
}

func (c *captureApplier) ApplyAction(_ context.Context, req *actions.ActionRequest) *actions.ActionResult {
	*c.capture = req
	return &actions.ActionResult{PRID: req.PRID, Action: req.Action, Success: true}
}
