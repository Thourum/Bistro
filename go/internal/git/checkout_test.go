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
	if err := c.CheckoutPRHead(context.Background(), dir, shaA); err != nil {
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
	err := c.CheckoutPRHead(context.Background(), dir, shaA)
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
	if err := c.CheckoutPRHead(context.Background(), "", "aaa"); err == nil {
		t.Error("expected error for empty local path")
	}
}

func TestCheckoutPRHead_EmptySHA(t *testing.T) {
	dir := t.TempDir()
	buildCheckoutTestRepo(t, dir)
	c := NewClient(nil)
	if err := c.CheckoutPRHead(context.Background(), dir, ""); err == nil {
		t.Error("expected error for empty headSHA")
	}
}

func TestCheckoutPRHead_ShaAbsentNoRemoteFails(t *testing.T) {
	dir := t.TempDir()
	buildCheckoutTestRepo(t, dir)

	c := NewClient(nil)
	err := c.CheckoutPRHead(context.Background(), dir, "0000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error: no remote configured, commit cannot be fetched")
	}
}
