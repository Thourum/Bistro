package diff

import (
	"fmt"
	"log/slog"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git/repolock"
)

// CommitStatsVersion invalidates cached stats when diff options or the
// fingerprint change; bump it with any such change.
const CommitStatsVersion = 1

// CommitStats is the cacheable list data of one non-merge commit. A SHA fixes
// its tree and first parent, so the stats never change for a given version.
type CommitStats struct {
	SHA       string
	Additions int
	Deletions int
	FileCount int
	PatchID   string
}

func statsOf(sha string, d CommitDiff) CommitStats {
	return CommitStats{SHA: sha, Additions: d.Additions, Deletions: d.Deletions, FileCount: len(d.Files), PatchID: d.PatchID}
}

// ComputeCommitStats diffs the given non-merge commits under the repo lock.
func ComputeCommitStats(locks *repolock.Locks, req CommitRangeRequest, shas []string) ([]CommitStats, error) {
	out := make([]CommitStats, 0, len(shas))
	started := time.Now()
	defer func() {
		slog.Debug("commit range timing", "op", "stats", "requested", len(shas),
			"computed", len(out), "duration_ms", time.Since(started).Milliseconds())
	}()
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		repo, closeRepo, err := openRangeRepo(req.RepoPath)
		if err != nil {
			return err
		}
		defer closeRepo()
		for _, sha := range shas {
			c, err := repo.CommitObject(plumbing.NewHash(sha))
			if err != nil {
				return fmt.Errorf("load commit %s: %w", sha, err)
			}
			d, err := commitDiff(c)
			if err != nil {
				return err
			}
			out = append(out, statsOf(sha, d))
		}
		return nil
	})
	return out, err
}

// CollectRangeStats is for callers that already hold the repo lock (sync's
// WithPRClone callback); it must not take the lock itself.
func CollectRangeStats(repoPath, headSHA, mergeBaseSHA string, cached map[string]CommitStats) ([]CommitStats, error) {
	repo, closeRepo, err := openRangeRepo(repoPath)
	if err != nil {
		return nil, err
	}
	defer closeRepo()
	return collectRangeStats(repo, headSHA, mergeBaseSHA, cached)
}

func collectRangeStats(repo *gitlib.Repository, headSHA, mergeBaseSHA string, cached map[string]CommitStats) ([]CommitStats, error) {
	commits, _, err := cappedRange(repo, headSHA, mergeBaseSHA)
	if err != nil {
		return nil, err
	}
	out := make([]CommitStats, 0, len(commits))
	for _, c := range commits {
		if len(c.ParentHashes) > 1 {
			continue
		}
		sha := c.Hash.String()
		if stats, ok := cached[sha]; ok {
			out = append(out, stats)
			continue
		}
		d, err := commitDiff(c)
		if err != nil {
			return nil, err
		}
		out = append(out, statsOf(sha, d))
	}
	return out, nil
}
