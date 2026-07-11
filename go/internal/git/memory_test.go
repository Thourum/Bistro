package git

import (
	"context"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// TestGenerateDiffForPRInMemory_Success verifies the in-memory PR-specific
// fetch+diff resolves both commits from a local remote and produces a diff
// identical to the disk-backed GenerateDiff over the same commits.
func TestGenerateDiffForPRInMemory_Success(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 31
	baseBranch, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	client := NewClient(NewCache())
	opts := &CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 50}

	memDiff, err := client.GenerateDiffForPRInMemory(context.Background(), url, baseSHA, headSHA, opts)
	if err != nil {
		t.Fatalf("GenerateDiffForPRInMemory: %v", err)
	}
	if len(memDiff.Files) == 0 {
		t.Fatal("expected non-empty in-memory diff")
	}

	// Disk reference diff over the same commits (open the source repo directly).
	repo, err := gogit.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}

	diskDiff, err := generateDiffFromRepo(repo, baseSHA, headSHA, DefaultDiffOptions())
	if err != nil {
		t.Fatalf("generateDiffFromRepo (disk): %v", err)
	}

	if len(memDiff.Files) != len(diskDiff.Files) {
		t.Fatalf("file count mismatch: in-memory=%d disk=%d", len(memDiff.Files), len(diskDiff.Files))
	}
	if memDiff.Size != diskDiff.Size {
		t.Errorf("diff size mismatch: in-memory=%d disk=%d", memDiff.Size, diskDiff.Size)
	}
	for i := range memDiff.Files {
		m, d := memDiff.Files[i], diskDiff.Files[i]
		if m.Path != d.Path || m.Status != d.Status || m.Patch != d.Patch ||
			m.AddedLines != d.AddedLines || m.RemovedLines != d.RemovedLines {
			t.Errorf("file %d mismatch:\n in-memory=%+v\n disk=%+v", i, m, d)
		}
	}
}

// TestGenerateDiffForPRInMemory_RequiredSHAMissing verifies the in-memory path
// errors (no silent degrade) when a RequiredSHA is absent from the shallow
// window. This is the prototype's equivalent of the disk path's fallback gate;
// the prototype has no disk fallback, so it surfaces an error.
func TestGenerateDiffForPRInMemory_RequiredSHAMissing(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 32
	baseBranch, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	client := NewClient(NewCache())
	const bogus = "1111111111111111111111111111111111111111"
	opts := &CloneOptions{
		PRNumber:     prNumber,
		BaseBranch:   baseBranch,
		Depth:        50,
		RequiredSHAs: []string{bogus},
	}
	if _, err := client.GenerateDiffForPRInMemory(context.Background(), url, baseSHA, headSHA, opts); err == nil {
		t.Fatal("expected error when a RequiredSHA is absent")
	}
}

// TestGenerateDiffForPRInMemory_DeepBaseMissing verifies that when the base
// commit lies outside a Depth:1 window it is reported missing (the in-memory
// prototype does not fall back to a broad clone — that is a disk-path feature).
func TestGenerateDiffForPRInMemory_DeepBaseMissing(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 33
	baseBranch, baseSHA, headSHA := buildDeepRemote(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	client := NewClient(NewCache())
	opts := &CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 1}
	if _, err := client.GenerateDiffForPRInMemory(context.Background(), url, baseSHA, headSHA, opts); err == nil {
		t.Fatal("expected error: deep base commit outside Depth:1 window")
	}
}

// TestGenerateDiffForPRInMemory_Validation covers argument validation and the
// no-targeting-hints guard.
func TestGenerateDiffForPRInMemory_Validation(t *testing.T) {
	client := NewClient(NewCache())
	ctx := context.Background()

	if _, err := client.GenerateDiffForPRInMemory(ctx, "", "b", "h", benchOptsForValidation()); err == nil {
		t.Error("expected error for empty URL")
	}
	if _, err := client.GenerateDiffForPRInMemory(ctx, "file:///x", "", "h", benchOptsForValidation()); err == nil {
		t.Error("expected error for empty baseSHA")
	}
	if _, err := client.GenerateDiffForPRInMemory(ctx, "file:///x", "b", "", benchOptsForValidation()); err == nil {
		t.Error("expected error for empty headSHA")
	}
	// No PR number and no base branch → cannot build refspecs.
	if _, err := client.GenerateDiffForPRInMemory(ctx, "file:///x", "b", "h", &CloneOptions{}); err == nil {
		t.Error("expected error when opts supplies no targeting hints")
	}
	if _, err := client.GenerateDiffForPRInMemory(ctx, "file:///x", "b", "h", nil); err == nil {
		t.Error("expected error for nil opts (no targeting hints)")
	}
}

func benchOptsForValidation() *CloneOptions {
	return &CloneOptions{PRNumber: 1, BaseBranch: "main"}
}

// TestVerifyRequiredCommits exercises the shared verifier used by both the disk
// targeted path and the in-memory path.
func TestVerifyRequiredCommits(t *testing.T) {
	remoteDir := t.TempDir()
	baseBranch, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, 1)
	_ = baseBranch
	repo, err := gogit.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}

	// All present (base+head, plus base again and an empty entry to be tolerated).
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, &CloneOptions{RequiredSHAs: []string{baseSHA, ""}}); missing != "" {
		t.Errorf("expected all present, got missing=%s", missing)
	}
	// Missing required SHA reported.
	const bogus = "2222222222222222222222222222222222222222"
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, &CloneOptions{RequiredSHAs: []string{bogus}}); missing != bogus {
		t.Errorf("expected missing=%s, got %s", bogus, missing)
	}
	// Missing base reported.
	if missing := verifyRequiredCommits(repo, bogus, headSHA, nil); missing != bogus {
		t.Errorf("expected missing base=%s, got %s", bogus, missing)
	}
}

// TestFetchForPRInMemory_RawVsVerifySplit documents the layering: the raw fetch
// succeeds (returns a repo) even when the PR base commit lies outside a Depth:1
// shallow window, while the verifying wrapper reports the base as missing. This
// is the in-memory analogue of the disk targeted path's required-commit gate
// (with no broad fallback in the prototype).
func TestFetchForPRInMemory_RawVsVerifySplit(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 41
	baseBranch, baseSHA, headSHA := buildDeepRemote(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	client := NewClient(NewCache())
	opts := &CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 1}

	// Raw fetch: no verification — succeeds and returns a usable repo even
	// though the deep base commit is not in the Depth:1 window.
	repo, err := client.fetchForPRInMemoryRaw(context.Background(), url, opts)
	if err != nil {
		t.Fatalf("fetchForPRInMemoryRaw: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo from raw fetch")
	}
	// Head IS reachable at depth 1 (it is the PR ref tip); base is NOT.
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		t.Errorf("head should be present in raw depth=1 fetch: %v", err)
	}

	// Verifying wrapper: must reject because base is outside the window.
	if _, err := client.fetchForPRInMemory(context.Background(), url, baseSHA, headSHA, opts); err == nil {
		t.Fatal("expected fetchForPRInMemory to error: deep base outside Depth:1 window")
	}

	// With sufficient depth, verification passes.
	optsDeep := &CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 50}
	if _, err := client.fetchForPRInMemory(context.Background(), url, baseSHA, headSHA, optsDeep); err != nil {
		t.Fatalf("fetchForPRInMemory with adequate depth: %v", err)
	}
}
