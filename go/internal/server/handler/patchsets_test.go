package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git"
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

type mockRangeDiffRunner struct {
	available      bool
	fallbackReason string
	result         git.RangeDiffResult
	err            error
}

func (m *mockRangeDiffRunner) Available() bool        { return m.available }
func (m *mockRangeDiffRunner) FallbackReason() string { return m.fallbackReason }
func (m *mockRangeDiffRunner) RunRangeDiff(_ context.Context, _, _, _, _, _ string) (git.RangeDiffResult, error) {
	return m.result, m.err
}

type mockLastReviewedReader struct {
	record store.LastReviewedRecord
	found  bool
	err    error
}

func (m *mockLastReviewedReader) GetLastReviewedPatchset(_ context.Context, _, _ string) (store.LastReviewedRecord, bool, error) {
	return m.record, m.found, m.err
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
	canonicalHash := PatchContentHash(patch)

	ds := &stubDiffReader{
		diffs: map[string]string{"pr-v": "unified"},
		files: map[string][]store.DiffFile{
			"pr-v": {{FilePath: "main.go", Patch: patch}},
		},
	}

	h := NewMarkFileViewedHandlerWithDiffStore(db, cu, ds, testRepoLogger())
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
	canonicalHash := PatchContentHash(patch)

	ds := &stubDiffReader{
		diffs: map[string]string{"pr-vm": "unified"},
		files: map[string][]store.DiffFile{
			"pr-vm": {{FilePath: "a.go", Patch: patch}},
		},
	}

	h := NewMarkFileViewedHandlerWithDiffStore(db, cu, ds, testRepoLogger())
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

// ---- REQUEST_RANGE_DIFF ----

func TestRangeDiffHandler_NoPatchsets_Degrades(t *testing.T) {
	deps := RangeDiffHandlerDeps{
		Runner:    &mockRangeDiffRunner{available: false, fallbackReason: "git not found"},
		Patchsets: &mockPatchsetLister{patchsets: nil},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-1"})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r5", Data: data})
	if err != nil {
		t.Fatalf("must not error in degraded path: %v", err)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "RANGE_DIFF_READY" {
		t.Errorf("type = %q", env["type"])
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("degraded = %v, want true", d["degraded"])
	}
}

func TestRangeDiffHandler_SinceLastReview_NoUser_Degrades(t *testing.T) {
	now := time.Now().UTC()
	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-1", Number: 1, HeadSHA: "aaaa", PushedAt: now},
		}},
		LastReviewed: &mockLastReviewedReader{},
		CurrentUser:  &mockCurrentUserReader{found: false},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{
		"pr_id": "pr-1", "since_last_review": true,
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r6", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("expected degraded=true when no user, got %v", d["degraded"])
	}
}

func TestRangeDiffHandler_SinceLastReview_ResolvesFromPatchset(t *testing.T) {
	now := time.Now().UTC()
	sha1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false, fallbackReason: "no clone"},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-1", Number: 1, HeadSHA: sha1, BaseSHA: "base", PushedAt: now},
			{PRID: "pr-1", Number: 2, HeadSHA: sha2, BaseSHA: "base", PushedAt: now},
		}},
		LastReviewed: &mockLastReviewedReader{
			record: store.LastReviewedRecord{SHA: sha1, PatchsetNum: 1},
			found:  true,
		},
		CurrentUser:  &mockCurrentUserReader{login: "alice", found: true},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{
		"pr_id": "pr-1", "since_last_review": true,
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r7", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("expected degraded=true (no clone), got %v", d["degraded"])
	}
	// from_patchset should be 1 (last reviewed was sha1 = P1).
	if d["from_patchset"] != 1 {
		t.Errorf("from_patchset = %v, want 1", d["from_patchset"])
	}
	// to_patchset should be 2 (latest).
	if d["to_patchset"] != 2 {
		t.Errorf("to_patchset = %v, want 2", d["to_patchset"])
	}
	if d["from_sha"] != sha1 {
		t.Errorf("from_sha = %v, want %s", d["from_sha"], sha1)
	}
	if d["to_sha"] != sha2 {
		t.Errorf("to_sha = %v, want %s", d["to_sha"], sha2)
	}
}

func TestRangeDiffHandler_PlainDiffFallback_RawPopulated(t *testing.T) {
	// When range-diff degrades, raw must contain the stored plain diff (not empty).
	now := time.Now().UTC()
	sha1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	storedDiff := "diff --git a/main.go b/main.go\n+added line\n"

	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false, fallbackReason: "no clone"},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-fd", Number: 1, HeadSHA: sha1, BaseSHA: "base", PushedAt: now},
			{PRID: "pr-fd", Number: 2, HeadSHA: sha2, BaseSHA: "base", PushedAt: now},
		}},
		PlainDiff: &stubDiffReader{diffs: map[string]string{"pr-fd": storedDiff}},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{
		"pr_id": "pr-fd", "from_patchset": 1, "to_patchset": 2,
	})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r8", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("expected degraded=true, got %v", d["degraded"])
	}
	if d["raw"] != storedDiff {
		t.Errorf("raw should be the stored plain diff, got %q (want %q)", d["raw"], storedDiff)
	}
}

func TestRangeDiffHandler_MissingPRID(t *testing.T) {
	deps := RangeDiffHandlerDeps{Runner: &mockRangeDiffRunner{available: true}}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]string{})
	_, err := h(context.Background(), ws.InboundMessage{ID: "r9", Data: data})
	if err == nil {
		t.Error("expected error for missing pr_id")
	}
}

// ── Blocker 1: patchset identity tests ──────────────────────────────────────

func TestRangeDiffHandler_SinceLastReview_PatchsetNumber_Unambiguous(t *testing.T) {
	// PatchsetNum > 0 in the record → use patchset number directly, no SHA scan.
	now := time.Now().UTC()
	sha1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// Same head_sha as sha1 but different base — would be ambiguous in legacy path.
	sha1b := sha1

	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-1", Number: 1, HeadSHA: sha1b, BaseSHA: "base-A", PushedAt: now},
			{PRID: "pr-1", Number: 2, HeadSHA: sha1b, BaseSHA: "base-B", PushedAt: now},
			{PRID: "pr-1", Number: 3, HeadSHA: sha2, BaseSHA: "base-C", PushedAt: now},
		}},
		// PatchsetNum=2: authoritative, despite sha1 appearing in P1 and P2.
		LastReviewed: &mockLastReviewedReader{
			record: store.LastReviewedRecord{SHA: sha1, PatchsetNum: 2},
			found:  true,
		},
		CurrentUser: &mockCurrentUserReader{login: "alice", found: true},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-1", "since_last_review": true})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "ra", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	// from_patchset must be exactly 2 (not 1, not ambiguous).
	if d["from_patchset"] != 2 {
		t.Errorf("from_patchset = %v, want 2 (patchset_number authoritative)", d["from_patchset"])
	}
}

func TestRangeDiffHandler_SinceLastReview_LegacyAmbiguous_Degrades(t *testing.T) {
	// Legacy record (PatchsetNum=0) with same head_sha in multiple patchsets → degraded.
	now := time.Now().UTC()
	sameHead := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			// Two patchsets with same head but different base — ambiguous without patchset_num.
			{PRID: "pr-2", Number: 1, HeadSHA: sameHead, BaseSHA: "base-X", PushedAt: now},
			{PRID: "pr-2", Number: 2, HeadSHA: sameHead, BaseSHA: "base-Y", PushedAt: now},
			{PRID: "pr-2", Number: 3, HeadSHA: sha2, BaseSHA: "base-Z", PushedAt: now},
		}},
		LastReviewed: &mockLastReviewedReader{
			// PatchsetNum=0 (legacy), BaseSHA="" (unknown): head-only match is ambiguous.
			record: store.LastReviewedRecord{SHA: sameHead, PatchsetNum: 0, BaseSHA: ""},
			found:  true,
		},
		CurrentUser: &mockCurrentUserReader{login: "alice", found: true},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-2", "since_last_review": true})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "rb", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("ambiguous legacy SHA must degrade, got degraded=%v", d["degraded"])
	}
	// raw must be empty — we don't know which patchset was reviewed.
	if d["raw"] != "" {
		t.Errorf("raw must be empty for ambiguous legacy, got %q", d["raw"])
	}
	reason, _ := d["fallback_reason"].(string)
	if reason == "" {
		t.Error("fallback_reason must be non-empty for ambiguous case")
	}
}

// ── Blocker 3: pair-specific raw tests ──────────────────────────────────────

func TestRangeDiffHandler_IdenticalPair_EmptyRaw(t *testing.T) {
	now := time.Now().UTC()
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	storedDiff := "diff --git a/x.go b/x.go\n+something\n"
	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-id", Number: 1, HeadSHA: sha, BaseSHA: "b", PushedAt: now},
		}},
		PlainDiff: &stubDiffReader{diffs: map[string]string{"pr-id": storedDiff}},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	// from==to==1: identical pair.
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-id", "from_patchset": 1, "to_patchset": 1})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "rc", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("identical pair must be degraded, got %v", d["degraded"])
	}
	if d["raw"] != "" {
		t.Errorf("identical pair: raw must be empty, got %q", d["raw"])
	}
}

func TestRangeDiffHandler_NonCurrentPair_EmptyRaw(t *testing.T) {
	// Patchsets P1, P2, P3 (latest=3). Request P1→P2 (non-current). Must NOT return P1→P3 diff.
	now := time.Now().UTC()
	sha1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sha3 := "cccccccccccccccccccccccccccccccccccccccc"
	storedDiff := "diff covering P3"
	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-nc", Number: 1, HeadSHA: sha1, PushedAt: now},
			{PRID: "pr-nc", Number: 2, HeadSHA: sha2, PushedAt: now},
			{PRID: "pr-nc", Number: 3, HeadSHA: sha3, PushedAt: now},
		}},
		PlainDiff: &stubDiffReader{diffs: map[string]string{"pr-nc": storedDiff}},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-nc", "from_patchset": 1, "to_patchset": 2})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "rd", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("non-current pair must degrade, got %v", d["degraded"])
	}
	if raw, _ := d["raw"].(string); raw != "" {
		t.Errorf("non-current pair: raw must be empty (not misleading stored diff), got %q", raw)
	}
}

func TestRangeDiffHandler_CurrentSequentialPair_RawPopulated(t *testing.T) {
	// P1→P2 where P2 is latest: current sequential pair → serve labeled plain diff.
	now := time.Now().UTC()
	sha1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	storedDiff := "diff --git a/main.go b/main.go\n+current change\n"
	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-cur", Number: 1, HeadSHA: sha1, PushedAt: now},
			{PRID: "pr-cur", Number: 2, HeadSHA: sha2, PushedAt: now},
		}},
		PlainDiff: &stubDiffReader{diffs: map[string]string{"pr-cur": storedDiff}},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-cur", "from_patchset": 1, "to_patchset": 2})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "re", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if d["degraded"] != true {
		t.Errorf("current pair (no clone): must degrade, got %v", d["degraded"])
	}
	raw, _ := d["raw"].(string)
	if raw != storedDiff {
		t.Errorf("current pair: raw must be stored plain diff, got %q", raw)
	}
	reason, _ := d["fallback_reason"].(string)
	if reason == "" {
		t.Error("current pair: fallback_reason must be set")
	}
}

func TestRangeDiffHandler_SkipOnePatchset_EmptyRaw(t *testing.T) {
	// P1→P3 (skipping P2): not sequential → raw must be empty.
	now := time.Now().UTC()
	sha1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sha3 := "cccccccccccccccccccccccccccccccccccccccc"
	storedDiff := "diff for current P3"
	deps := RangeDiffHandlerDeps{
		Runner: &mockRangeDiffRunner{available: false},
		Patchsets: &mockPatchsetLister{patchsets: []store.Patchset{
			{PRID: "pr-skip", Number: 1, HeadSHA: sha1, PushedAt: now},
			{PRID: "pr-skip", Number: 2, HeadSHA: sha2, PushedAt: now},
			{PRID: "pr-skip", Number: 3, HeadSHA: sha3, PushedAt: now},
		}},
		PlainDiff: &stubDiffReader{diffs: map[string]string{"pr-skip": storedDiff}},
	}
	h := NewRangeDiffHandlerWithDeps(deps, testRepoLogger())
	data, _ := json.Marshal(map[string]interface{}{"pr_id": "pr-skip", "from_patchset": 1, "to_patchset": 3})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "rf", Data: data})
	if err != nil {
		t.Fatalf("must not error: %v", err)
	}
	d := extractData(t, resp)
	if raw, _ := d["raw"].(string); raw != "" {
		t.Errorf("P1→P3 (skip): raw must be empty, got %q", raw)
	}
}
