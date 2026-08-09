package git

import (
	"context"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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
func (c *Client) CheckoutPRHead(ctx context.Context, localPath, headSHA string) error {
	if localPath == "" {
		return fmt.Errorf("local path cannot be empty")
	}
	if headSHA == "" {
		return fmt.Errorf("headSHA cannot be empty")
	}

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
		// Commit not present locally — fetch it before checking out.
		fetchOpts := &gitlib.FetchOptions{
			Auth: c.auth(),
			RefSpecs: []config.RefSpec{
				config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
				config.RefSpec("+refs/pull/*/head:refs/remotes/origin/pr/*"),
			},
		}
		if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
			return fmt.Errorf("failed to fetch refs before checkout: %w", err)
		}
		if _, err := repo.CommitObject(targetHash); err != nil {
			return fmt.Errorf("head commit %s not found even after fetch: %w", headSHA, err)
		}
	}

	if err := worktree.Checkout(&gitlib.CheckoutOptions{
		Hash: targetHash,
	}); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", headSHA, err)
	}

	return nil
}
