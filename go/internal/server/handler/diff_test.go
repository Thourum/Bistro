package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type stubDiffReader struct {
	diffs map[string]string
	files map[string][]store.DiffFile
}

func (s *stubDiffReader) GetUnifiedDiff(prID string) (string, error) {
	if d, ok := s.diffs[prID]; ok {
		return d, nil
	}
	return "", fmt.Errorf("no diffs found for PR %s", prID)
}

func (s *stubDiffReader) GetDiffs(prID string) ([]store.DiffFile, error) {
	return s.files[prID], nil
}

func makeMsg(t *testing.T, id, prID string) ws.InboundMessage {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"pr_id": prID})
	return ws.InboundMessage{ID: id, Type: "REQUEST_DIFF", Data: data}
}

func TestRequestDiffHandler_Found(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{
		"pr-1": "diff --git a/main.go b/main.go\n+new line\n",
	}}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-1", "pr-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", resp)
	}
	if m["type"] != "DIFF_READY" {
		t.Errorf("expected type DIFF_READY, got %v", m["type"])
	}
	if m["id"] != "req-1" {
		t.Errorf("expected id req-1, got %v", m["id"])
	}

	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", m["data"])
	}
	if data["pr_id"] != "pr-1" {
		t.Errorf("expected pr_id pr-1, got %v", data["pr_id"])
	}
	if data["diff"] == "" {
		t.Error("expected non-empty diff")
	}
	if _, hasErr := data["error"]; hasErr {
		t.Errorf("expected no error field on success, got %v", data["error"])
	}
}

func TestRequestDiffHandler_IncludesFileStats(t *testing.T) {
	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": "diff --git a/a.go b/a.go\n+x\n"},
		files: map[string][]store.DiffFile{
			"pr-1": {
				{FilePath: "a.go", Additions: 10, Deletions: 2, Patch: "patch-a"},
				{FilePath: "b.go", Additions: 5, Deletions: 1, Patch: "patch-b"},
			},
		},
	}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-files", "pr-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})

	// Files are now []diffFileDetail (not []diffFileStat).
	files, ok := data["files"].([]diffFileDetail)
	if !ok {
		t.Fatalf("files is %T, want []diffFileDetail", data["files"])
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].Path != "a.go" || files[0].Additions != 10 || files[0].Deletions != 2 {
		t.Errorf("unexpected first file: %+v", files[0])
	}
	if data["additions"] != 15 {
		t.Errorf("additions = %v, want 15", data["additions"])
	}
	if data["deletions"] != 3 {
		t.Errorf("deletions = %v, want 3", data["deletions"])
	}

	// content_hash must be non-empty.
	if files[0].ContentHash == "" {
		t.Error("content_hash should be non-empty for a.go")
	}
	// viewed should default to false when no CurrentUser dep is provided.
	if files[0].Viewed {
		t.Error("viewed should be false when no CurrentUser dep")
	}
}

func TestRequestDiffHandler_IncludesFileMetadata(t *testing.T) {
	stub := &stubDiffReader{
		diffs: map[string]string{"pr-meta": "diff --git a/old.go b/new.go\n"},
		files: map[string][]store.DiffFile{
			"pr-meta": {{
				FilePath:    "new.go",
				Status:      "renamed",
				OldPath:     "old.go",
				IsBinary:    true,
				IsTruncated: true,
				DiffStatus:  "truncated",
				PatchBytes:  42,
				HunkCount:   0,
				Patch:       "binary patch",
				Additions:   1,
				Deletions:   2,
			}},
		},
	}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-meta", "pr-meta"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	files := data["files"].([]diffFileDetail)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	got := files[0]
	if got.Status != "renamed" || got.OldPath != "old.go" || !got.IsBinary || !got.IsTruncated || got.DiffStatus != "truncated" || got.PatchBytes != 42 || got.HunkCount != 0 {
		t.Fatalf("metadata not exposed: %+v", got)
	}
	if got.ContentHash == "" || got.Viewed {
		t.Fatalf("content_hash/viewed not preserved: %+v", got)
	}
}

func TestRequestDiffHandler_NotFound(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{}}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-2", "pr-missing"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", resp)
	}
	if m["type"] != "DIFF_READY" {
		t.Errorf("expected type DIFF_READY, got %v", m["type"])
	}

	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", m["data"])
	}
	if data["error"] == "" {
		t.Error("expected non-empty error field for missing PR")
	}
	if data["diff"] != "" {
		t.Errorf("expected empty diff on error, got %v", data["diff"])
	}
}

func TestRequestDiffHandler_MissingPRID(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{}}
	handler := NewRequestDiffHandler(stub, nil)

	data, _ := json.Marshal(map[string]string{})
	msg := ws.InboundMessage{ID: "req-3", Type: "REQUEST_DIFF", Data: data}

	_, err := handler(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing pr_id, got nil")
	}
}

func TestRequestDiffHandler_InvalidPayload(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{}}
	handler := NewRequestDiffHandler(stub, nil)

	msg := ws.InboundMessage{ID: "req-4", Type: "REQUEST_DIFF", Data: []byte("not-json")}

	_, err := handler(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---- content_hash and viewed tests ----

type stubViewedChecker struct {
	viewed map[string]bool // key: "prID|filePath|hash"
}

func (s *stubViewedChecker) IsFileViewed(_ context.Context, prID, filePath, _, contentHash string) (bool, error) {
	key := prID + "|" + filePath + "|" + contentHash
	return s.viewed[key], nil
}

func TestRequestDiffHandler_ContentHashIsDeterministic(t *testing.T) {
	// The same patch must always produce the same content_hash.
	patch := "@@ -1,2 +1,3 @@\n context\n+new line\n context\n"
	hash1 := PatchContentHash(patch)
	hash2 := PatchContentHash(patch)
	if hash1 != hash2 {
		t.Error("PatchContentHash is not deterministic")
	}
	if len(hash1) != 64 { // 32 bytes hex-encoded
		t.Errorf("expected 64-char hex hash, got len %d: %q", len(hash1), hash1)
	}
}

func TestRequestDiffHandler_DifferentPatchesDifferentHash(t *testing.T) {
	if PatchContentHash("patch-a") == PatchContentHash("patch-b") {
		t.Error("different patches must produce different hashes")
	}
}

func TestRequestDiffHandler_ViewedFlagTrueWhenFileViewed(t *testing.T) {
	const patch = "--- a.go\n+++ a.go\n+added line\n"
	hash := PatchContentHash(patch)

	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": patch},
		files: map[string][]store.DiffFile{
			"pr-1": {{FilePath: "a.go", Patch: patch, Additions: 1}},
		},
	}
	viewed := &stubViewedChecker{viewed: map[string]bool{
		"pr-1|a.go|" + hash: true,
	}}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	h := NewRequestDiffHandlerWithDeps(DiffHandlerDeps{
		DiffStore:   stub,
		ViewedStore: viewed,
		CurrentUser: cu,
	})

	resp, err := h(context.Background(), makeMsg(t, "r1", "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	files := data["files"].([]diffFileDetail)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if !files[0].Viewed {
		t.Error("viewed should be true for a file with matching hash")
	}
	if files[0].ContentHash != hash {
		t.Errorf("content_hash mismatch: got %q want %q", files[0].ContentHash, hash)
	}
}

func TestRequestDiffHandler_ViewedFalseWhenHashMismatch(t *testing.T) {
	const patch = "some patch content"
	hash := PatchContentHash(patch)

	stub := &stubDiffReader{
		diffs: map[string]string{"pr-2": patch},
		files: map[string][]store.DiffFile{
			"pr-2": {{FilePath: "main.go", Patch: patch}},
		},
	}
	// Viewed with a STALE hash (different from current patch hash).
	staleHash := PatchContentHash("old patch content")
	viewed := &stubViewedChecker{viewed: map[string]bool{
		"pr-2|main.go|" + staleHash: true,
		// Current hash is NOT in viewed store → viewed=false.
	}}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	h := NewRequestDiffHandlerWithDeps(DiffHandlerDeps{
		DiffStore:   stub,
		ViewedStore: viewed,
		CurrentUser: cu,
	})

	resp, err := h(context.Background(), makeMsg(t, "r2", "pr-2"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	files := data["files"].([]diffFileDetail)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Viewed {
		t.Error("viewed should be false when stored hash doesn't match current patch hash")
	}
	if files[0].ContentHash != hash {
		t.Errorf("content_hash should be current: got %q want %q", files[0].ContentHash, hash)
	}
}
