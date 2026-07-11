package git

import (
	"context"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

const remoteMismatchCode = "remote_mismatch"

// LocalFetchError is a structured local-repo fetch error.
type LocalFetchError struct {
	Code string
	Err  error
}

func (e *LocalFetchError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *LocalFetchError) Unwrap() error {
	return e.Err
}

// FetchIntoLocal fetches the required commits into a user-owned local repo,
// writing ONLY refs/bistro/<safeID>/. Never checks out; tag following
// disabled. Validates first that some remote of the repo matches identity.
func (c *Client) FetchIntoLocal(ctx context.Context, localPath string, id Identity, baseSHA, headSHA string, opts *CloneOptions) error {
	if localPath == "" {
		return fmt.Errorf("local path cannot be empty")
	}
	if baseSHA == "" {
		return fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return fmt.Errorf("headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}

	if err := validateLocalRepoRemote(repo, id); err != nil {
		return err
	}

	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing == "" {
		return nil
	}

	fetchOpts := &gitlib.FetchOptions{
		Auth:     c.auth(),
		RefSpecs: localFetchRefSpecs(id, opts),
		Tags:     gitlib.NoTags,
	}
	if opts != nil && (opts.PRNumber > 0 || opts.BaseBranch != "") {
		fetchOpts.Depth = targetedDepth(opts)
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch refs into local repo: %w", err)
	}

	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing != "" {
		return fmt.Errorf("required commit %s not available after fetch", missing)
	}

	return nil
}

func validateLocalRepoRemote(repo *gitlib.Repository, id Identity) error {
	remotes, err := repo.Remotes()
	if err != nil {
		return fmt.Errorf("failed to list remotes: %w", err)
	}
	for _, remote := range remotes {
		for _, remoteURL := range remote.Config().URLs {
			remoteID, err := ParseRemoteURL(remoteURL)
			if err != nil {
				continue
			}
			if remoteID.ID() == id.ID() {
				return nil
			}
		}
	}
	return &LocalFetchError{Code: remoteMismatchCode, Err: fmt.Errorf("no remote matches %s", id.ID())}
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
