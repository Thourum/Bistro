package prsync

import (
	"context"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestFetchDiff_CollectsCommitStatsForRange(t *testing.T) {
	dir, baseSHA, firstSHA := makeExpertiseHookRepo(t)
	repo, err := gitlib.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	writeExpertiseHookFile(t, dir, "second.txt", "second\n")
	secondSHA := commitExpertiseHookRepo(t, worktree, "second PR commit")
	// The redundant ancestor parent makes a merge that must not get list stats.
	head, err := worktree.Commit("merge base", &gitlib.CommitOptions{
		AllowEmptyCommits: true,
		Parents:           []plumbing.Hash{plumbing.NewHash(secondSHA), plumbing.NewHash(baseSHA)},
		Author:            &object.Signature{Name: "test", Email: "test@example.com", When: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("merge commit: %v", err)
	}
	gitClient := git.NewClient(nil)
	gitClient.SetLocalResolver(&expertiseHookLocalResolver{repoURL: "https://github.com/org/repo.git", path: dir})
	const prNumber = 7
	pr := &provider.PR{ID: "github:org/repo:7", Owner: "org", Repo: "repo", Number: prNumber, BaseSHA: baseSHA, HeadSHA: head.String()}
	for _, cacheState := range []string{"new PR", "unavailable cache"} {
		t.Run(cacheState, func(t *testing.T) {
			db := storetest.Open(t)
			if cacheState == "unavailable cache" {
				if _, err := db.Exec("DROP TABLE commit_stats"); err != nil {
					t.Fatalf("drop commit_stats: %v", err)
				}
			}
			orchestrator := NewOrchestrator(silentLog(), nil, db, gitClient)
			result := orchestrator.fetchPR(context.Background(), pr, prFetchOptions{diff: true})
			if result.Error != nil || !result.DiffComputed || result.MergeBaseSHA != baseSHA {
				t.Fatalf("expected successful PR diff against merge base: %+v", result)
			}
			if cacheState == "unavailable cache" {
				if result.CommitStatsComputed {
					t.Fatal("stats must remain uncomputed after cache read failure")
				}
				return
			}
			if !result.CommitStatsComputed || len(result.CommitStats) != 2 {
				t.Fatalf("expected two non-merge commit stats: %+v", result)
			}
			for i, sha := range []string{firstSHA, secondSHA} {
				stats := result.CommitStats[i]
				if stats.SHA != sha || stats.PatchID == "" {
					t.Errorf("stats[%d] = %+v; want SHA %s and patch fingerprint, oldest first", i, stats, sha)
				}
			}
			first, second := result.CommitStats[0], result.CommitStats[1]
			if first.Additions != 2 || first.Deletions != 1 || first.FileCount != 2 || second.Additions != 1 || second.Deletions != 0 || second.FileCount != 1 {
				t.Errorf("unexpected stats: %+v", result.CommitStats)
			}
		})
	}
}
