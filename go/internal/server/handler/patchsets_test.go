package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// ---- mock implementations ----

type mockPatchsetLister struct {
	patchsets []store.Patchset
	err       error
}

func (m *mockPatchsetLister) ListPatchsets(_ context.Context, _ string) ([]store.Patchset, error) {
	return m.patchsets, m.err
}

func (m *mockPatchsetLister) CurrentPatchset(_ context.Context, _ string) (store.Patchset, bool, error) {
	return store.Patchset{}, false, nil
}

type mockFileViewedMarker struct {
	called bool
	err    error
}

func (m *mockFileViewedMarker) MarkFileViewed(_ context.Context, _, _, _, _ string) error {
	m.called = true
	return m.err
}

type mockCurrentUserReader struct {
	login string
	found bool
}

func (m *mockCurrentUserReader) GetCurrentUser(_ context.Context, _ string) (string, bool, error) {
	return m.login, m.found, nil
}

// ---- helper ----

func patchsetMsg(t *testing.T, prID string) ws.InboundMessage {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"pr_id": prID})
	return ws.InboundMessage{ID: "req-1", Type: "GET_PATCHSETS", Data: data}
}

func extractData(t *testing.T, resp interface{}) map[string]interface{} {
	t.Helper()
	env, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response is not a map, got %T", resp)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map, got %T", env["data"])
	}
	return data
}

// ---- GET_PATCHSETS ----

func TestListPatchsetsHandler_ReturnsPatchsets(t *testing.T) {
	now := time.Now().UTC()
	ps := &mockPatchsetLister{patchsets: []store.Patchset{
		{PRID: "pr-1", Number: 1, HeadSHA: "aaa", BaseSHA: "bbb", PushedAt: now},
		{PRID: "pr-1", Number: 2, HeadSHA: "ccc", BaseSHA: "bbb", PushedAt: now},
	}}

	h := NewListPatchsetsHandler(ps, testRepoLogger())
	resp, err := h(context.Background(), patchsetMsg(t, "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	env := resp.(map[string]interface{})
	if env["type"] != "PATCHSETS_READY" {
		t.Errorf("type = %q, want PATCHSETS_READY", env["type"])
	}
	data := extractData(t, resp)
	patchsets, ok := data["patchsets"].([]store.Patchset)
	if !ok {
		t.Fatalf("patchsets is %T, want []store.Patchset", data["patchsets"])
	}
	if len(patchsets) != 2 {
		t.Fatalf("expected 2 patchsets, got %d", len(patchsets))
	}
	if patchsets[0].Number != 1 || patchsets[1].Number != 2 {
		t.Errorf("numbers: %d %d", patchsets[0].Number, patchsets[1].Number)
	}
}

func TestListPatchsetsHandler_EmptySlice(t *testing.T) {
	h := NewListPatchsetsHandler(&mockPatchsetLister{}, testRepoLogger())
	resp, err := h(context.Background(), patchsetMsg(t, "pr-empty"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	patchsets, ok := data["patchsets"].([]store.Patchset)
	if !ok {
		t.Fatalf("patchsets is %T", data["patchsets"])
	}
	if len(patchsets) != 0 {
		t.Errorf("expected empty slice, got %d items", len(patchsets))
	}
}

func TestListPatchsetsHandler_MissingPRID(t *testing.T) {
	h := NewListPatchsetsHandler(&mockPatchsetLister{}, testRepoLogger())
	data, _ := json.Marshal(map[string]string{})
	_, err := h(context.Background(), ws.InboundMessage{ID: "x", Data: data})
	if err == nil {
		t.Error("expected error for missing pr_id")
	}
}

// ---- MARK_FILE_VIEWED ----

func TestMarkFileViewedHandler_Success(t *testing.T) {
	db := &mockFileViewedMarker{}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	h := NewMarkFileViewedHandler(db, cu, testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"pr_id":        "pr-1",
		"file_path":    "main.go",
		"content_hash": "abc123",
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r1", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !db.called {
		t.Error("MarkFileViewed was not called")
	}
	env := resp.(map[string]interface{})
	if env["type"] != "FILE_VIEWED_ACK" {
		t.Errorf("type = %q", env["type"])
	}
	d := extractData(t, resp)
	if d["ok"] != true {
		t.Errorf("ok = %v, want true", d["ok"])
	}
}

func TestMarkFileViewedHandler_NoUser(t *testing.T) {
	db := &mockFileViewedMarker{}
	cu := &mockCurrentUserReader{found: false}

	h := NewMarkFileViewedHandler(db, cu, testRepoLogger())
	data, _ := json.Marshal(map[string]string{
		"pr_id": "pr-1", "file_path": "x.go", "content_hash": "h",
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r2", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if db.called {
		t.Error("MarkFileViewed should not be called when no user")
	}
	d := extractData(t, resp)
	if d["ok"] == true {
		t.Error("ok should be false when no user")
	}
}

// ---- MARK_FILE_VIEWED hash validation ----

func TestMarkFileViewedHandler_HashValidation_Mismatch(t *testing.T) {
	db := &mockFileViewedMarker{}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	const patch = "real patch content for main.go"
	canonicalHash := viewedHash(store.DiffFile{Patch: patch})

	ds := &stubDiffReader{
		diffs: map[string]string{"pr-v": "unified"},
		files: map[string][]store.DiffFile{
			"pr-v": {{FilePath: "main.go", Patch: patch}},
		},
	}

	h := NewMarkFileViewedHandlerWithDeps(MarkFileViewedDeps{Viewed: db, CurrentUser: cu, DiffStore: ds, Log: testRepoLogger()})
	// Send a DIFFERENT hash than what the server computed.
	data, _ := json.Marshal(map[string]string{
		"pr_id": "pr-v", "file_path": "main.go",
		"content_hash": "deadbeef" + canonicalHash[8:], // corrupted
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r3", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if db.called {
		t.Error("MarkFileViewed must not be called when hash is invalid")
	}
	d := extractData(t, resp)
	if d["ok"] == true {
		t.Error("ok must be false for hash mismatch")
	}
	if d["reason"] == nil || d["reason"] == "" {
		t.Error("reason must be non-empty on hash mismatch")
	}
}

func TestMarkFileViewedHandler_HashValidation_Match(t *testing.T) {
	db := &mockFileViewedMarker{}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	const patch = "the canonical patch content"
	canonicalHash := viewedHash(store.DiffFile{Patch: patch})

	ds := &stubDiffReader{
		diffs: map[string]string{"pr-vm": "unified"},
		files: map[string][]store.DiffFile{
			"pr-vm": {{FilePath: "a.go", Patch: patch}},
		},
	}

	h := NewMarkFileViewedHandlerWithDeps(MarkFileViewedDeps{Viewed: db, CurrentUser: cu, DiffStore: ds, Log: testRepoLogger()})
	data, _ := json.Marshal(map[string]string{
		"pr_id": "pr-vm", "file_path": "a.go", "content_hash": canonicalHash,
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r4", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !db.called {
		t.Error("MarkFileViewed must be called when hash matches")
	}
	d := extractData(t, resp)
	if d["ok"] != true {
		t.Errorf("ok must be true for valid hash, got: %v (reason: %v)", d["ok"], d["reason"])
	}
}
