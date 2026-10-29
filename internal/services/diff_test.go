package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/itterno/bistro/internal/store/prdiff"
)

type stubDiffReader struct {
	diffs map[string]string
	files map[string][]prdiff.File
}

func (s *stubDiffReader) GetUnified(prID string) (string, error) {
	if d, ok := s.diffs[prID]; ok {
		return d, nil
	}
	return "", fmt.Errorf("no diffs found for PR %s", prID)
}

func (s *stubDiffReader) Get(prID string) ([]prdiff.File, error) {
	return s.files[prID], nil
}

func TestRequestDiffHandler_Found(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{
		"pr-1": "diff --git a/main.go b/main.go\n+new line\n",
	}}
	handler := (&Diff{DiffStore: stub}).Get

	resp, err := handler(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.PRID != "pr-1" || resp.Diff == "" {
		t.Fatalf("diff = %+v", resp)
	}
}

func TestRequestDiffHandler_IncludesFileStats(t *testing.T) {
	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": "diff --git a/a.go b/a.go\n+x\n"},
		files: map[string][]prdiff.File{
			"pr-1": {
				{FilePath: "a.go", Additions: 10, Deletions: 2, Patch: "patch-a"},
				{FilePath: "b.go", Additions: 5, Deletions: 1, Patch: "patch-b"},
			},
		},
	}
	handler := (&Diff{DiffStore: stub}).Get

	resp, err := handler(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	files := resp.Files
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].Path != "a.go" || files[0].Additions != 10 || files[0].Deletions != 2 {
		t.Errorf("unexpected first file: %+v", files[0])
	}
	if resp.Additions != 15 {
		t.Errorf("additions = %v, want 15", resp.Additions)
	}
	if resp.Deletions != 3 {
		t.Errorf("deletions = %v, want 3", resp.Deletions)
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
		files: map[string][]prdiff.File{
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
	handler := (&Diff{DiffStore: stub}).Get

	resp, err := handler(context.Background(), PRRef{PRID: "pr-meta"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	files := resp.Files
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

func TestDiffNotFoundAndInvalid(t *testing.T) {
	s := &Diff{DiffStore: &stubDiffReader{}}
	_, err := s.Get(context.Background(), PRRef{PRID: "missing"})
	assertServiceCode(t, err, CodeNotFound)
	_, err = s.Get(context.Background(), PRRef{})
	assertServiceCode(t, err, CodeInvalid)
}

// ---- content_hash and viewed tests ----

type stubViewedChecker struct {
	err    error
	viewed map[string]bool // key: "prID|filePath|hash"
}

func (s *stubViewedChecker) IsFileViewed(_ context.Context, prID, filePath, _, contentHash string) (bool, error) {
	key := prID + "|" + filePath + "|" + contentHash
	return s.viewed[key], s.err
}

func TestRequestDiffHandler_UsesStoredPatchID(t *testing.T) {
	const patchID = "full-patch-id"
	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": "truncated patch"},
		files: map[string][]prdiff.File{
			"pr-1": {{FilePath: "a.go", Patch: "truncated patch", PatchID: patchID}},
		},
	}
	handler := (&Diff{DiffStore: stub}).Get
	response, err := handler(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	files := response.Files
	if len(files) != 1 || files[0].ContentHash != patchID {
		t.Fatalf("files = %+v, want content_hash %q", files, patchID)
	}
}

func TestRequestDiffHandler_ViewedSurvivesLineShift(t *testing.T) {
	const originalPatch = "@@ -1,2 +1,3 @@\n context\n+new line\n context\n"
	const shiftedPatch = "@@ -10,2 +10,3 @@\n context\n+new line\n context\n"
	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": originalPatch},
		files: map[string][]prdiff.File{
			"pr-1": {{FilePath: "a.go", Patch: originalPatch}},
		},
	}
	viewed := &stubViewedChecker{viewed: make(map[string]bool)}
	handler := (&Diff{
		DiffStore: stub, ViewedStore: viewed,
		CurrentUser: &mockCurrentUserReader{login: "alice", found: true},
	}).Get
	response, err := handler(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("original diff: %v", err)
	}
	originalHash := response.Files[0].ContentHash
	viewed.viewed["pr-1|a.go|"+originalHash] = true
	stub.files["pr-1"] = []prdiff.File{{FilePath: "a.go", Patch: shiftedPatch}}

	response, err = handler(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("shifted diff: %v", err)
	}
	file := response.Files[0]
	if !file.Viewed || file.ContentHash != originalHash {
		t.Fatalf("shifted file = %+v, want viewed with hash %q", file, originalHash)
	}
}

func TestRequestDiffHandler_ContentHashIsDeterministic(t *testing.T) {
	// The same patch must always produce the same content_hash.
	patch := "@@ -1,2 +1,3 @@\n context\n+new line\n context\n"
	hash1 := viewedHash(prdiff.File{Patch: patch})
	hash2 := viewedHash(prdiff.File{Patch: patch})
	if hash1 != hash2 {
		t.Error("viewedHash is not deterministic")
	}
	if len(hash1) != 64 { // 32 bytes hex-encoded
		t.Errorf("expected 64-char hex hash, got len %d: %q", len(hash1), hash1)
	}
}

func TestRequestDiffHandler_DifferentPatchesDifferentHash(t *testing.T) {
	if viewedHash(prdiff.File{Patch: "patch-a"}) == viewedHash(prdiff.File{Patch: "patch-b"}) {
		t.Error("different patches must produce different hashes")
	}
}

func TestRequestDiffHandler_ViewedFlagTrueWhenFileViewed(t *testing.T) {
	const patch = "--- a.go\n+++ a.go\n+added line\n"
	hash := viewedHash(prdiff.File{Patch: patch})

	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": patch},
		files: map[string][]prdiff.File{
			"pr-1": {{FilePath: "a.go", Patch: patch, Additions: 1}},
		},
	}
	viewed := &stubViewedChecker{viewed: map[string]bool{
		"pr-1|a.go|" + hash: true,
	}}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	h := (&Diff{
		DiffStore:   stub,
		ViewedStore: viewed,
		CurrentUser: cu,
	}).Get

	resp, err := h(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	files := resp.Files
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
	hash := viewedHash(prdiff.File{Patch: patch})

	stub := &stubDiffReader{
		diffs: map[string]string{"pr-2": patch},
		files: map[string][]prdiff.File{
			"pr-2": {{FilePath: "main.go", Patch: patch}},
		},
	}
	// Viewed with a STALE hash (different from current patch hash).
	staleHash := viewedHash(prdiff.File{Patch: "old patch content"})
	viewed := &stubViewedChecker{viewed: map[string]bool{
		"pr-2|main.go|" + staleHash: true,
		// Current hash is NOT in viewed store → viewed=false.
	}}
	cu := &mockCurrentUserReader{login: "alice", found: true}

	h := (&Diff{
		DiffStore:   stub,
		ViewedStore: viewed,
		CurrentUser: cu,
	}).Get

	resp, err := h(context.Background(), PRRef{PRID: "pr-2"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	files := resp.Files
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

func TestDiffEnrichmentFailuresAreInternal(t *testing.T) {
	reader := &stubDiffReader{diffs: map[string]string{"p": "diff"}, files: map[string][]prdiff.File{"p": {{FilePath: "a.go", PatchID: "hash"}}}}
	failure := errors.New("lookup unavailable")
	s := &Diff{DiffStore: reader, CurrentUser: &errorCurrentUserReader{err: failure}}
	_, err := s.Get(context.Background(), PRRef{PRID: "p"})
	assertServiceCode(t, err, CodeInternal)
	s.CurrentUser = &mockCurrentUserReader{login: "alice", found: true}
	s.ViewedStore = &stubViewedChecker{err: failure}
	_, err = s.Get(context.Background(), PRRef{PRID: "p"})
	assertServiceCode(t, err, CodeInternal)
}
