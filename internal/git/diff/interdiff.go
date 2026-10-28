package diff

import (
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
)

// ErrHeadUnavailable means a patchset head is not in the local repo: it was
// recorded before heads were pinned, or the cache clone was deleted.
var ErrHeadUnavailable = errors.New("patchset head unavailable locally")

// InterdiffRequest names one file at two patchset heads.
type InterdiffRequest struct {
	RepoURL       string // the sync's clone URL; serializes with syncs of the repo
	RepoPath      string
	Path          string
	FromHead      string
	ToHead        string
	FromMergeBase string // "" when unknown
	ToMergeBase   string // "" when unknown
}

// InterdiffResult is the file's change between the two heads.
type InterdiffResult struct {
	Patch   string // "" when the file is identical at both heads
	Rebased bool   // the file also differs between the two merge bases
}

// Interdiff diffs req.Path from FromHead to ToHead.
func Interdiff(locks *repolock.Locks, req InterdiffRequest) (InterdiffResult, error) {
	var result InterdiffResult
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		repo, err := gitlib.PlainOpen(req.RepoPath)
		if err != nil {
			return fmt.Errorf("open repo %s: %w", req.RepoPath, err)
		}
		fromTree, err := HeadTree(repo, req.FromHead)
		if err != nil {
			return err
		}
		toTree, err := HeadTree(repo, req.ToHead)
		if err != nil {
			return err
		}
		if result.Patch, err = treeFilePatch(fromTree, toTree, req.Path); err != nil {
			return err
		}
		result.Rebased = fileChangedBetween(repo, req.FromMergeBase, req.ToMergeBase, req.Path)
		return nil
	})
	return result, err
}

// HeadTree loads the tree at sha, returning ErrHeadUnavailable for a missing head.
func HeadTree(repo *gitlib.Repository, sha string) (*object.Tree, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrHeadUnavailable, logging.ShortSHA(sha))
	}
	if err != nil {
		return nil, fmt.Errorf("load commit %s: %w", logging.ShortSHA(sha), err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load tree of %s: %w", logging.ShortSHA(sha), err)
	}
	return tree, nil
}

func treeFilePatch(from, to *object.Tree, path string) (string, error) {
	changes, err := object.DiffTree(from, to)
	if err != nil {
		return "", fmt.Errorf("diff trees for %s: %w", path, err)
	}
	for _, change := range changes {
		if change.From.Name != path && change.To.Name != path {
			continue
		}
		patch, err := change.Patch()
		if err != nil {
			return "", fmt.Errorf("patch %s: %w", path, err)
		}
		for _, filePatch := range patch.FilePatches() {
			return filePatchToString(filePatch)
		}
	}
	return "", nil
}

// fileChangedBetween reports whether path differs between two merge bases.
// Unreadable merge bases count as changed: the label only warns.
func fileChangedBetween(repo *gitlib.Repository, fromMergeBase, toMergeBase, path string) bool {
	if fromMergeBase == toMergeBase {
		return false
	}
	if fromMergeBase == "" || toMergeBase == "" {
		return false // Pre-feature patchsets: no merge base recorded, nothing to compare.
	}
	fromBlob, fromErr := blobAt(repo, fromMergeBase, path)
	toBlob, toErr := blobAt(repo, toMergeBase, path)
	if fromErr != nil || toErr != nil {
		return true
	}
	return fromBlob != toBlob
}

// blobAt returns path's blob hash at commitSHA, or ZeroHash when absent.
func blobAt(repo *gitlib.Repository, commitSHA, path string) (plumbing.Hash, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(commitSHA))
	if err != nil {
		return plumbing.ZeroHash, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	entry, err := tree.FindEntry(path)
	if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
		return plumbing.ZeroHash, nil
	}
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return entry.Hash, nil
}
