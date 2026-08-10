package git

import (
	"context"
	"os"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// testRepoID is the repos.id-shaped lock key used by checkout tests.
const testRepoID = "github:owner/repo"

// buildCheckoutTestRepo initialises an on-disk repo at dir with two commits
// on "foo.txt", HEAD left at the second commit. Returns (shaA, shaB).
func buildCheckoutTestRepo(t *testing.T, dir string) (string, string) {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	writeAndCommit := func(content, msg string) plumbing.Hash {
		if err := os.WriteFile(dir+"/foo.txt", []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := wt.Add("foo.txt"); err != nil {
			t.Fatalf("Add: %v", err)
		}
		h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig})
		if err != nil {
			t.Fatalf("Commit %q: %v", msg, err)
		}
		return h
	}

	shaA := writeAndCommit("content A\n", "commit A")
	shaB := writeAndCommit("content B\n", "commit B")
	return shaA.String(), shaB.String()
}

// ── InspectPRCheckout ───────────────────────────────────────────────────────

func TestInspectPRCheckout_HeadMatches(t *testing.T) {
	dir := t.TempDir()
	_, shaB := buildCheckoutTestRepo(t, dir)

	got, err := InspectPRCheckout(dir, shaB)
	if err != nil {
		t.Fatalf("InspectPRCheckout: %v", err)
	}
	if !got.HeadMatches {
		t.Error("expected HeadMatches=true")
	}
	if !got.SHAPresent {
		t.Error("expected SHAPresent=true")
	}
}

func TestInspectPRCheckout_HeadElsewhereShaPresent(t *testing.T) {
	dir := t.TempDir()
	shaA, _ := buildCheckoutTestRepo(t, dir)

	got, err := InspectPRCheckout(dir, shaA)
	if err != nil {
		t.Fatalf("InspectPRCheckout: %v", err)
	}
	if got.HeadMatches {
		t.Error("expected HeadMatches=false (HEAD is at commit B)")
	}
	if !got.SHAPresent {
		t.Error("expected SHAPresent=true (commit A exists locally)")
	}
}

func TestInspectPRCheckout_ShaAbsent(t *testing.T) {
	dir := t.TempDir()
	buildCheckoutTestRepo(t, dir)

	got, err := InspectPRCheckout(dir, "0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("InspectPRCheckout: %v", err)
	}
	if got.HeadMatches {
		t.Error("expected HeadMatches=false")
	}
	if got.SHAPresent {
		t.Error("expected SHAPresent=false")
	}
}

func TestInspectPRCheckout_EmptyPath(t *testing.T) {
	if _, err := InspectPRCheckout("", "aaa"); err == nil {
		t.Error("expected error for empty local path")
	}
}

func TestInspectPRCheckout_EmptySHA(t *testing.T) {
	dir := t.TempDir()
	buildCheckoutTestRepo(t, dir)
	if _, err := InspectPRCheckout(dir, ""); err == nil {
		t.Error("expected error for empty headSHA")
	}
}

// ── CheckoutPRHead ──────────────────────────────────────────────────────────

func TestCheckoutPRHead_CleanWorktreeMovesHead(t *testing.T) {
	dir := t.TempDir()
	shaA, shaB := buildCheckoutTestRepo(t, dir)
	_ = shaB

	c := NewClient(nil)
	if err := c.CheckoutPRHead(context.Background(), testRepoID, dir, shaA); err != nil {
		t.Fatalf("CheckoutPRHead: %v", err)
	}

	got, err := InspectPRCheckout(dir, shaA)
	if err != nil {
		t.Fatalf("InspectPRCheckout after checkout: %v", err)
	}
	if !got.HeadMatches {
		t.Error("expected HEAD to have moved to shaA")
	}
}

func TestCheckoutPRHead_DirtyWorktreeRefuses(t *testing.T) {
	dir := t.TempDir()
	shaA, _ := buildCheckoutTestRepo(t, dir)

	// Dirty the worktree: modify the tracked file without committing.
	if err := os.WriteFile(dir+"/foo.txt", []byte("uncommitted change\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := NewClient(nil)
	err := c.CheckoutPRHead(context.Background(), testRepoID, dir, shaA)
	if err == nil {
		t.Fatal("expected error for dirty worktree")
	}
	// Verify no state change: HEAD must still be at commit B.
	repo, openErr := gogit.PlainOpen(dir)
	if openErr != nil {
		t.Fatalf("PlainOpen: %v", openErr)
	}
	head, headErr := repo.Head()
	if headErr != nil {
		t.Fatalf("Head: %v", headErr)
	}
	if head.Hash().String() == shaA {
		t.Error("HEAD moved despite dirty-worktree refusal")
	}
}

func TestCheckoutPRHead_EmptyPath(t *testing.T) {
	c := NewClient(nil)
	if err := c.CheckoutPRHead(context.Background(), testRepoID, "", "aaa"); err == nil {
		t.Error("expected error for empty local path")
	}
}

func TestCheckoutPRHead_EmptySHA(t *testing.T) {
	dir := t.TempDir()
	buildCheckoutTestRepo(t, dir)
	c := NewClient(nil)
	if err := c.CheckoutPRHead(context.Background(), testRepoID, dir, ""); err == nil {
		t.Error("expected error for empty headSHA")
	}
}

func TestCheckoutPRHead_ShaAbsentNoRemoteFails(t *testing.T) {
	dir := t.TempDir()
	buildCheckoutTestRepo(t, dir)

	c := NewClient(nil)
	err := c.CheckoutPRHead(context.Background(), testRepoID, dir, "0000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error: no remote configured, commit cannot be fetched")
	}
}

// ── CheckoutPRHead repo locking ─────────────────────────────────────────────

// TestCheckoutPRHead_SerializesOnRepoLock proves CheckoutPRHead participates in
// the shared per-repo lock. Sync's local fetch path (FetchIntoLocal, via
// cloneWithOptions) already holds RepoLocks.WithRepo(identity.ID()) while it
// writes packfiles and refs into the very same on-disk repository that
// CheckoutPRHead fetches into and checks out. Without the lock, a background
// sync tick and an open-in-editor checkout mutate one .git concurrently.
func TestCheckoutPRHead_SerializesOnRepoLock(t *testing.T) {
	dir := t.TempDir()
	shaA, _ := buildCheckoutTestRepo(t, dir)

	locks := NewRepoLocks()
	c := NewClient(nil)
	c.SetLocks(locks)

	// Hold the repo lock exactly as a concurrent sync fetch would.
	held := make(chan struct{})
	release := make(chan struct{})
	lockDone := make(chan struct{})
	go func() {
		defer close(lockDone)
		_ = locks.WithRepo(testRepoID, func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	checkoutDone := make(chan error, 1)
	go func() {
		checkoutDone <- c.CheckoutPRHead(context.Background(), testRepoID, dir, shaA)
	}()

	select {
	case err := <-checkoutDone:
		t.Fatalf("CheckoutPRHead completed (err=%v) while the repo lock was held by a concurrent operation", err)
	case <-time.After(150 * time.Millisecond):
		// Correctly blocked.
	}

	close(release)
	<-lockDone

	select {
	case err := <-checkoutDone:
		if err != nil {
			t.Fatalf("CheckoutPRHead after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CheckoutPRHead did not complete after the repo lock was released")
	}

	got, err := InspectPRCheckout(dir, shaA)
	if err != nil {
		t.Fatalf("InspectPRCheckout: %v", err)
	}
	if !got.HeadMatches {
		t.Error("expected HEAD to have moved to shaA")
	}
}

// TestCheckoutPRHead_NoLocksConfigured keeps CheckoutPRHead usable when the
// client has no RepoLocks (unit tests, standalone use).
func TestCheckoutPRHead_NoLocksConfigured(t *testing.T) {
	dir := t.TempDir()
	shaA, _ := buildCheckoutTestRepo(t, dir)

	c := NewClient(nil)
	c.locks = nil
	if err := c.CheckoutPRHead(context.Background(), testRepoID, dir, shaA); err != nil {
		t.Fatalf("CheckoutPRHead: %v", err)
	}
}

// TestCheckoutPRHead_LockKeyMatchesSyncKey pins the lock key to the same value
// sync uses (identity.ID()), so the two paths actually exclude each other.
// A key derived from localPath would silently never collide.
func TestCheckoutPRHead_LockKeyMatchesSyncKey(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		repoID  string
	}{
		{name: "https remote", repoURL: "https://github.com/owner/repo.git", repoID: "github:owner/repo"},
		{name: "scp remote", repoURL: "git@github.com:owner/repo.git", repoID: "github:owner/repo"},
		{name: "mixed case remote", repoURL: "https://github.com/Owner/Repo", repoID: "github:owner/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repoLockKey(tt.repoURL); got != tt.repoID {
				t.Errorf("repoLockKey(%q) = %q, want %q (the repos.id CheckoutPRHead locks on)", tt.repoURL, got, tt.repoID)
			}
		})
	}
}
