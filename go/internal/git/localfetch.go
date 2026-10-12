package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

const remoteMismatchCode = "remote_mismatch"

// LocalFetchError is a structured local-repo fetch error.
type LocalFetchError struct {
	Code string
	Err  error
}

// Error returns the failure code and underlying cause when available.
func (e *LocalFetchError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

// Unwrap returns the underlying fetch error for errors.Is and errors.As.
func (e *LocalFetchError) Unwrap() error {
	return e.Err
}

// FetchIntoLocal fetches the required commits into a user-owned local repo,
// writing ONLY refs/bistro/<safeID>/. Never checks out; tag following
// disabled. Validates first that some remote of the repo matches identity.
// Note: opts.Depth is intentionally ignored — local fetches are never shallow,
// so no .git/shallow metadata is ever written into a user-owned repo.
func (c *Client) FetchIntoLocal(ctx context.Context, localPath string, id Identity, baseSHA, headSHA string, opts *CloneOptions) error {
	if localPath == "" {
		return errors.New("local path cannot be empty")
	}
	if baseSHA == "" {
		return errors.New("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return errors.New("headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}

	remoteName, err := matchingLocalRepoRemote(repo, id)
	if err != nil {
		return err
	}

	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing == "" {
		return nil
	}

	fetchOpts := &gitlib.FetchOptions{
		RemoteName: remoteName,
		Auth:       c.auth(),
		RefSpecs:   localFetchRefSpecs(id, opts),
		Tags:       gitlib.NoTags,
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && !errors.Is(err, gitlib.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to fetch refs into local repo: %w", err)
	}

	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing != "" {
		return fmt.Errorf("required commit %s not available after fetch", missing)
	}

	return nil
}

func matchingLocalRepoRemote(repo *gitlib.Repository, id Identity) (string, error) {
	return matchingLocalRepoRemoteByID(repo, id.ID())
}

// matchingLocalRepoRemoteByID returns the name of the first remote whose URL
// parses to repoID (a repos.id value such as "github:owner/repo"). Never
// assumes "origin".
func matchingLocalRepoRemoteByID(repo *gitlib.Repository, repoID string) (string, error) {
	remotes, err := repo.Remotes()
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}
	for _, remote := range remotes {
		for _, remoteURL := range remote.Config().URLs {
			remoteID, err := ParseRemoteURL(remoteURL)
			if err != nil {
				continue
			}
			if remoteID.ID() == strings.ToLower(repoID) {
				return remote.Config().Name, nil
			}
		}
	}
	return "", &LocalFetchError{Code: remoteMismatchCode, Err: fmt.Errorf("no remote matches %s", repoID)}
}

func localFetchRefSpecs(id Identity, opts *CloneOptions) []config.RefSpec {
	safeID := SafeRepoID(id)
	refSpecs := make([]config.RefSpec, 0, 2)
	if opts != nil {
		if opts.PRNumber > 0 {
			refSpecs = append(refSpecs, config.RefSpec(fmt.Sprintf(
				"+refs/pull/%d/head:refs/bistro/%s/pull/%d/head", opts.PRNumber, safeID, opts.PRNumber)))
		}
		if opts.BaseBranch != "" {
			refSpecs = append(refSpecs, config.RefSpec(fmt.Sprintf(
				"+refs/heads/%s:refs/bistro/%s/base/%s", opts.BaseBranch, safeID, opts.BaseBranch)))
		}
	}
	if len(refSpecs) > 0 {
		return refSpecs
	}
	return []config.RefSpec{
		config.RefSpec(fmt.Sprintf("+refs/heads/*:refs/bistro/%s/heads/*", safeID)),
		config.RefSpec(fmt.Sprintf("+refs/pull/*/head:refs/bistro/%s/pull/*/head", safeID)),
	}
}
