package git

import (
	"context"
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ErrMergeBaseNotFound reports that no common ancestor is reachable locally,
// typically because a shallow clone stops short of it.
var ErrMergeBaseNotFound = errors.New("merge base not found")

// deepenFetchDepth reaches merge bases beyond defaultTargetedFetchDepth in one fetch.
const deepenFetchDepth = 1000

// MergeBase returns the best common ancestor of baseSHA and headSHA in the
// repository at localPath.
func MergeBase(localPath, baseSHA, headSHA string) (string, error) {
	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("open repository %s: %w", localPath, err)
	}
	base, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return "", fmt.Errorf("%w: base %s: %w", ErrMergeBaseNotFound, baseSHA, err)
	}
	head, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return "", fmt.Errorf("%w: head %s: %w", ErrMergeBaseNotFound, headSHA, err)
	}
	// Walking past a shallow boundary surfaces as a missing-object error.
	bases, err := head.MergeBase(base)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrMergeBaseNotFound, err)
	}
	if len(bases) == 0 {
		return "", ErrMergeBaseNotFound
	}
	return bases[0].Hash.String(), nil
}

// Deepen extends a shallow clone's history along the PR's refs so a merge base
// beyond the shallow boundary becomes reachable. Full clones are left alone.
// Callers hold the repo lock (it runs inside WithPRClone's callback).
func (c *Client) Deepen(ctx context.Context, localPath string, opts *CloneOptions) error {
	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("open repository %s: %w", localPath, err)
	}
	shallow, err := repo.Storer.Shallow()
	if err != nil {
		return fmt.Errorf("read shallow commits in %s: %w", localPath, err)
	}
	if len(shallow) == 0 {
		return nil
	}
	return c.fetchIntoRepo(ctx, localPath, opts, deepenFetchDepth)
}
