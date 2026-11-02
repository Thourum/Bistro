package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git/repolock"
)

// CommitRangeLimit bounds step-by-step review; larger PRs use the combined diff.
const CommitRangeLimit = 250

// ErrCommitNotInRange includes commits beyond the oldest capped range.
var ErrCommitNotInRange = errors.New("commit is not in the PR range")

// CommitRangeRequest identifies the locally cached PR range.
type CommitRangeRequest struct {
	RepoURL      string // lock key, same URL sync clones with
	RepoPath     string
	HeadSHA      string
	MergeBaseSHA string
	WithDiffs    bool // compute Diff for every non-merge commit
}

// CommitDiff retains file metadata even when a patch exceeds the file budget.
type CommitDiff struct {
	Files     []*File
	Unified   string
	Additions int
	Deletions int
	PatchID   string // commit fingerprint: sha256 over file PatchIDs
}

// RangeCommit contains metadata and an optional single-parent diff.
type RangeCommit struct {
	SHA        string
	ParentSHAs []string
	Subject    string
	Body       string
	AuthorName string
	AuthoredAt time.Time
	IsMerge    bool
	Diff       *CommitDiff // nil for merges, or when diffs were not requested
}

// CommitRange keeps the oldest commits when the review limit is exceeded.
type CommitRange struct {
	Commits   []RangeCommit // oldest first, at most CommitRangeLimit
	Truncated bool
}

// ListCommitRange serializes range reads with repository fetches and maintenance.
func ListCommitRange(locks *repolock.Locks, req CommitRangeRequest) (CommitRange, error) {
	var out CommitRange
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		commits, truncated, err := cappedRange(req)
		if err != nil {
			return err
		}
		out.Truncated = truncated
		out.Commits = make([]RangeCommit, 0, len(commits))
		for _, c := range commits {
			rc := toRangeCommit(c)
			if req.WithDiffs && !rc.IsMerge {
				d, err := commitDiff(c)
				if err != nil {
					return err
				}
				rc.Diff = &d
			}
			out.Commits = append(out.Commits, rc)
		}
		return nil
	})
	return out, err
}

// GetRangeCommit returns one commit of the capped range with its Diff
// (nil for merges); ErrCommitNotInRange when sha is not in it.
func GetRangeCommit(locks *repolock.Locks, req CommitRangeRequest, sha string) (RangeCommit, error) {
	var out RangeCommit
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		commits, _, err := cappedRange(req)
		if err != nil {
			return err
		}
		for _, c := range commits {
			if c.Hash.String() != sha {
				continue
			}
			out = toRangeCommit(c)
			if !out.IsMerge {
				d, err := commitDiff(c)
				if err != nil {
					return err
				}
				out.Diff = &d
			}
			return nil
		}
		return ErrCommitNotInRange
	})
	return out, err
}

func cappedRange(req CommitRangeRequest) ([]*object.Commit, bool, error) {
	repo, err := gitlib.PlainOpen(req.RepoPath)
	if err != nil {
		return nil, false, fmt.Errorf("open repo %s: %w", req.RepoPath, err)
	}
	commits, err := rangeCommits(repo, req.HeadSHA, req.MergeBaseSHA)
	if err != nil {
		return nil, false, err
	}
	if len(commits) > CommitRangeLimit {
		return commits[:CommitRangeLimit], true, nil
	}
	return commits, false, nil
}

func toRangeCommit(c *object.Commit) RangeCommit {
	subject, body, _ := strings.Cut(strings.TrimRight(c.Message, "\n"), "\n")
	parents := make([]string, len(c.ParentHashes))
	for i, p := range c.ParentHashes {
		parents[i] = p.String()
	}
	return RangeCommit{
		SHA: c.Hash.String(), ParentSHAs: parents,
		Subject: strings.TrimSpace(subject), Body: strings.TrimSpace(body),
		AuthorName: c.Author.Name, AuthoredAt: c.Author.When,
		IsMerge: len(c.ParentHashes) > 1,
	}
}

func commitDiff(c *object.Commit) (CommitDiff, error) {
	tree, err := c.Tree()
	if err != nil {
		return CommitDiff{}, fmt.Errorf("load tree of %s: %w", c.Hash, err)
	}
	parentTree := &object.Tree{} // root commits have no parent entries
	if len(c.ParentHashes) > 0 {
		parent, err := c.Parent(0)
		if err != nil {
			return CommitDiff{}, fmt.Errorf("load parent of %s: %w", c.Hash, err)
		}
		if parentTree, err = parent.Tree(); err != nil {
			return CommitDiff{}, fmt.Errorf("load parent tree of %s: %w", c.Hash, err)
		}
	}
	changes, err := object.DiffTree(parentTree, tree)
	if err != nil {
		return CommitDiff{}, fmt.Errorf("diff %s: %w", c.Hash, err)
	}
	patch, err := changes.Patch()
	if err != nil {
		return CommitDiff{}, fmt.Errorf("patch %s: %w", c.Hash, err)
	}
	result, err := renderDiffFiles(patch, DefaultOptions())
	if err != nil {
		return CommitDiff{}, err
	}
	d := CommitDiff{Files: result.Files, Unified: unifiedPatch(result.Files), PatchID: commitPatchID(result.Files)}
	for _, f := range result.Files {
		d.Additions += f.AddedLines
		d.Deletions += f.RemovedLines
	}
	return d, nil
}

// Empty (binary/truncated) patches need a header to keep the file visible,
// matching prdiff.Store.GetUnified.
func unifiedPatch(files []*File) string {
	var b strings.Builder
	for _, f := range files {
		if !strings.HasPrefix(f.Patch, "diff --git ") {
			oldPath := f.Path
			if f.OldPath != "" {
				oldPath = f.OldPath
			}
			fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldPath, f.Path)
		}
		b.WriteString(f.Patch)
		if f.Patch != "" && !strings.HasSuffix(f.Patch, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func commitPatchID(files []*File) string {
	ids := make([]string, len(files))
	for i, f := range files {
		ids[i] = f.PatchID
	}
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])
}
