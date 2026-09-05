package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// CheckoutInspection reports how a local checkout relates to a PR's head SHA,
// without mutating the working tree.
type CheckoutInspection struct {
	// HeadMatches is true when the repo's current HEAD commit equals headSHA.
	HeadMatches bool
	// SHAPresent is true when headSHA's commit object exists locally,
	// regardless of what HEAD currently points at.
	SHAPresent bool
}

// InspectPRCheckout opens the repository at localPath and reports whether its
// HEAD already matches headSHA and whether the commit object is present
// locally at all. Read-only — never fetches, never mutates the working tree.
func InspectPRCheckout(localPath, headSHA string) (CheckoutInspection, error) {
	if localPath == "" {
		return CheckoutInspection{}, fmt.Errorf("local path cannot be empty")
	}
	if headSHA == "" {
		return CheckoutInspection{}, fmt.Errorf("headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return CheckoutInspection{}, fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}

	head, err := repo.Head()
	if err != nil {
		return CheckoutInspection{}, fmt.Errorf("failed to resolve HEAD: %w", err)
	}

	targetHash := plumbing.NewHash(headSHA)
	_, shaErr := repo.CommitObject(targetHash)

	return CheckoutInspection{
		HeadMatches: head.Hash() == targetHash,
		SHAPresent:  shaErr == nil,
	}, nil
}

// ErrWorktreeDirty is returned by CheckoutPRHead when the worktree has
// uncommitted changes. Checkout refuses to run rather than discard them.
var ErrWorktreeDirty = fmt.Errorf("worktree dirty")

// CheckoutPRHead moves the repository at localPath's HEAD to headSHA,
// detaching it. Fetches first only when the commit isn't already present
// locally. Refuses to run — returning ErrWorktreeDirty — when the worktree
// has uncommitted changes, so a reviewer's in-progress work is never
// silently discarded.
//
// repoID is the repos.id value ("<provider>:<owner>/<repo>") — the same key
// sync's fetch/clone paths lock on via repoLockKey. The whole
// status → fetch → checkout sequence runs under that lock, so a background
// sync tick writing refs and packfiles into this repository can never
// interleave with the checkout. The dirty-worktree check is only meaningful
// if nothing else can mutate the repo between the check and the checkout.
func (c *Client) CheckoutPRHead(ctx context.Context, repoID, localPath, headSHA string) error {
	if localPath == "" {
		return fmt.Errorf("local path cannot be empty")
	}
	if headSHA == "" {
		return fmt.Errorf("headSHA cannot be empty")
	}

	operation := func() error {
		return c.checkoutPRHeadLocked(ctx, repoID, localPath, headSHA)
	}
	if c.locks == nil || repoID == "" {
		return operation()
	}
	return c.locks.WithRepo(repoID, operation)
}

// checkoutPRHeadLocked is CheckoutPRHead's body. Callers must hold the repo
// lock for repoID (or have no locks configured).
func (c *Client) checkoutPRHeadLocked(ctx context.Context, repoID, localPath, headSHA string) error {
	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}
	if !status.IsClean() {
		return ErrWorktreeDirty
	}

	targetHash := plumbing.NewHash(headSHA)
	if _, err := repo.CommitObject(targetHash); err != nil {
		// Commit not present locally — fetch it before checking out. The
		// remote is resolved by URL (it may not be called "origin") and refs
		// land under a Bistro-owned namespace so the user's remote-tracking
		// refs are never touched.
		remoteName, err := matchingLocalRepoRemoteByID(repo, repoID)
		if err != nil {
			return err
		}
		fetchOpts := &gitlib.FetchOptions{
			RemoteName: remoteName,
			Auth:       c.auth(),
			RefSpecs: []config.RefSpec{
				config.RefSpec("+refs/heads/*:refs/bistro/checkout/heads/*"),
				config.RefSpec("+refs/pull/*/head:refs/bistro/checkout/pull/*/head"),
			},
			Tags: gitlib.NoTags,
		}
		if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
			return fmt.Errorf("failed to fetch refs before checkout: %w", err)
		}
		if _, err := repo.CommitObject(targetHash); err != nil {
			return fmt.Errorf("head commit %s not found even after fetch: %w", headSHA, err)
		}
	}

	if err := refuseIfCheckoutClobbersLocalFiles(repo, localPath, targetHash); err != nil {
		return err
	}

	if err := worktree.Checkout(&gitlib.CheckoutOptions{
		Hash: targetHash,
	}); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", headSHA, err)
	}

	return nil
}

// refuseIfCheckoutClobbersLocalFiles guards the gap Status().IsClean() leaves
// open: gitignored files are invisible to status, so a checkout that starts
// tracking one of those paths would silently overwrite the local copy. Every
// path the target commit adds relative to HEAD that already exists on disk is
// therefore a conflict (untracked files are already caught by the clean check).
func refuseIfCheckoutClobbersLocalFiles(repo *gitlib.Repository, localPath string, target plumbing.Hash) error {
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to resolve HEAD: %w", err)
	}
	headTree, err := commitTree(repo, head.Hash())
	if err != nil {
		return err
	}
	targetTree, err := commitTree(repo, target)
	if err != nil {
		return err
	}
	changes, err := object.DiffTree(headTree, targetTree)
	if err != nil {
		return fmt.Errorf("failed to diff trees: %w", err)
	}

	var clobbered []string
	for _, change := range changes {
		// From.Name == "" means the path is not tracked in HEAD (pure add).
		if change.From.Name != "" || change.To.Name == "" {
			continue
		}
		if _, statErr := os.Lstat(filepath.Join(localPath, filepath.FromSlash(change.To.Name))); statErr == nil {
			clobbered = append(clobbered, change.To.Name)
		}
	}
	if len(clobbered) == 0 {
		return nil
	}
	return fmt.Errorf("checkout would overwrite local untracked/ignored files: %s", strings.Join(clobbered, ", "))
}

func commitTree(repo *gitlib.Repository, hash plumbing.Hash) (*object.Tree, error) {
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to load commit %s: %w", hash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to load tree for %s: %w", hash, err)
	}
	return tree, nil
}
