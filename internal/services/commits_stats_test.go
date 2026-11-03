package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/store/commitstats"
)

func fakeCommitStats(patchIDs map[string]string) func(context.Context, diff.CommitRangeRequest, []string) ([]diff.CommitStats, error) {
	return func(_ context.Context, _ diff.CommitRangeRequest, shas []string) ([]diff.CommitStats, error) {
		stats := make([]diff.CommitStats, len(shas))
		for i, sha := range shas {
			stats[i] = diff.CommitStats{SHA: sha, PatchID: patchIDs[sha]}
		}
		return stats, nil
	}
}

func TestCommitsList_WarmCacheSkipsCompute(t *testing.T) {
	s := newCommitsTestService(t)
	sha := strings.Repeat("a", 40)
	cached := commitstats.Stats{SHA: sha, Additions: 12, Deletions: 4, FileCount: 3, PatchID: "p"}
	ctx := context.Background()
	if err := s.Stats.Upsert(ctx, commitsTestPRID, diff.CommitStatsVersion, []commitstats.Stats{cached}); err != nil {
		t.Fatal(err)
	}
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: []diff.RangeCommit{{SHA: sha}}, Truncated: true}, nil
	}
	resp, err := s.List(ctx, PRRef{PRID: commitsTestPRID})
	if err != nil || !resp.Available || !resp.Truncated || len(resp.Commits) != 1 {
		t.Fatalf("List = %+v, %v", resp, err)
	}
	got := resp.Commits[0]
	if got.Additions != cached.Additions || got.Deletions != cached.Deletions || got.FileCount != cached.FileCount {
		t.Fatalf("stats = %+v, want %+v", got, cached)
	}
}

func TestCommitsList_PartialCacheComputesOnlyMissingAndStores(t *testing.T) {
	s := newCommitsTestService(t)
	sha1, sha2 := strings.Repeat("a", 40), strings.Repeat("b", 40)
	cached := commitstats.Stats{SHA: sha1, Additions: 12, Deletions: 4, FileCount: 3, PatchID: "p"}
	missing := commitstats.Stats{SHA: sha2, Additions: 5, Deletions: 2, FileCount: 1, PatchID: "q"}
	ctx := context.Background()
	if err := s.Stats.Upsert(ctx, commitsTestPRID, diff.CommitStatsVersion, []commitstats.Stats{cached}); err != nil {
		t.Fatal(err)
	}
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: []diff.RangeCommit{{SHA: sha1}, {SHA: sha2}}}, nil
	}
	calls := 0
	s.ComputeStats = func(_ context.Context, req diff.CommitRangeRequest, shas []string) ([]diff.CommitStats, error) {
		calls++
		if !reflect.DeepEqual(shas, []string{sha2}) || req.HeadSHA != "head" || req.MergeBaseSHA != "merge-base" {
			t.Fatalf("ComputeStats = %+v, %v", req, shas)
		}
		return []diff.CommitStats{{SHA: sha2, Additions: missing.Additions, Deletions: missing.Deletions, FileCount: missing.FileCount, PatchID: missing.PatchID}}, nil
	}
	for i := 0; i < 2; i++ {
		resp, err := s.List(ctx, PRRef{PRID: commitsTestPRID})
		if err != nil || !resp.Available || len(resp.Commits) != 2 {
			t.Fatalf("List = %+v, %v", resp, err)
		}
		for j, want := range []commitstats.Stats{cached, missing} {
			got := resp.Commits[j]
			if got.Additions != want.Additions || got.Deletions != want.Deletions || got.FileCount != want.FileCount {
				t.Fatalf("commit stats = %+v, want %+v", got, want)
			}
		}
	}
	if calls != 1 {
		t.Fatalf("ComputeStats calls = %d, want 1", calls)
	}
	stored, err := s.Stats.List(ctx, commitsTestPRID, diff.CommitStatsVersion)
	if err != nil || !reflect.DeepEqual(stored, map[string]commitstats.Stats{sha1: cached, sha2: missing}) {
		t.Fatalf("cached stats = %+v, %v", stored, err)
	}
}

func TestCommitsList_MergeNeverComputed(t *testing.T) {
	s := newCommitsTestService(t)
	sha, mergeSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	if err := s.Stats.Upsert(context.Background(), commitsTestPRID, diff.CommitStatsVersion, []commitstats.Stats{{SHA: mergeSHA, Additions: 1, Deletions: 1, FileCount: 1, PatchID: "merge"}}); err != nil {
		t.Fatal(err)
	}
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: []diff.RangeCommit{{SHA: sha}, {SHA: mergeSHA, IsMerge: true}}}, nil
	}
	s.ComputeStats = func(_ context.Context, _ diff.CommitRangeRequest, shas []string) ([]diff.CommitStats, error) {
		if !reflect.DeepEqual(shas, []string{sha}) {
			t.Fatalf("ComputeStats SHAs = %v, want only non-merge", shas)
		}
		return []diff.CommitStats{{SHA: sha, Additions: 1, FileCount: 1, PatchID: "p"}}, nil
	}
	resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	if err != nil || !resp.Available || len(resp.Commits) != 2 {
		t.Fatalf("List = %+v, %v", resp, err)
	}
	merge := resp.Commits[1]
	if !merge.IsMerge || merge.Additions != 0 || merge.Deletions != 0 || merge.FileCount != 0 {
		t.Fatalf("merge = %+v", merge)
	}
}

func TestCommitsGet_MergeReturnsIsMerge(t *testing.T) {
	s := newCommitsTestService(t)
	sha := strings.Repeat("a", 40)
	s.Commit = func(diff.CommitRangeRequest, string) (diff.RangeCommit, error) {
		return diff.RangeCommit{SHA: sha, IsMerge: true}, nil
	}
	resp, err := s.Get(context.Background(), CommitRequest{PRID: commitsTestPRID, SHA: sha})
	if err != nil || !resp.Available || !resp.IsMerge || len(resp.Files) != 0 || resp.Diff != "" {
		t.Fatalf("Get merge = %+v, %v", resp, err)
	}
}

type failingCommitStatsStore struct {
	commitStatsStore
	listErr, upsertErr error
}

func (s failingCommitStatsStore) List(ctx context.Context, prID string, version int) (map[string]commitstats.Stats, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.commitStatsStore.List(ctx, prID, version)
}

func (s failingCommitStatsStore) Upsert(context.Context, string, int, []commitstats.Stats) error {
	return s.upsertErr
}

func TestCommits_ComputeFailureUsesGitErrorMapping(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		code    Code
	}{
		{name: "missing objects", failure: diff.ErrHeadUnavailable, code: CodeInvalid},
		{name: "git failure", failure: errors.New("compute failed"), code: CodeInternal},
		{name: "cancelled", failure: context.Canceled, code: CodeCancelled},
		{name: "timeout", failure: context.DeadlineExceeded, code: CodeTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newCommitsTestService(t)
			sha := strings.Repeat("a", 40)
			s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
				return diff.CommitRange{Commits: []diff.RangeCommit{{SHA: sha}}}, nil
			}
			s.ComputeStats = func(context.Context, diff.CommitRangeRequest, []string) ([]diff.CommitStats, error) {
				return nil, test.failure
			}
			resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
			if expectedReason, expectedErr := rangeFailure(context.Background(), test.failure); expectedErr != nil {
				assertCommitError(t, err, test.code, "")
			} else if err != nil || resp.Available || resp.Reason != expectedReason {
				t.Fatalf("List = %+v, %v; want reason %q", resp, err, expectedReason)
			}
			_, err = s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha, Viewed: true})
			assertCommitError(t, err, test.code, "")
		})
	}
}

func TestCommits_CacheReadFailureReturnsError(t *testing.T) {
	s := newCommitsTestService(t)
	sha := strings.Repeat("a", 40)
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: []diff.RangeCommit{{SHA: sha}}}, nil
	}
	s.Stats = failingCommitStatsStore{listErr: errors.New("cache read failed")}
	resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	assertCommitError(t, err, CodeInternal, "")
	if resp.Available || resp.Reason != "" {
		t.Fatalf("cache failure was treated as unavailable git data: %+v", resp)
	}
	_, err = s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha, Viewed: true})
	assertCommitError(t, err, CodeInternal, "")
	stored, err := s.Viewed.List(context.Background(), commitsTestPRID, "me")
	if err != nil || len(stored) != 0 {
		t.Fatalf("cache failure persisted viewed marks: %v, %v", stored, err)
	}
}

func TestCommits_CacheWriteFailurePreservesStatsAndViewedKey(t *testing.T) {
	s := newCommitsTestService(t)
	sha := strings.Repeat("a", 40)
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: []diff.RangeCommit{{SHA: sha}}}, nil
	}
	s.Stats = failingCommitStatsStore{commitStatsStore: s.Stats, upsertErr: errors.New("cache write failed")}
	s.ComputeStats = func(context.Context, diff.CommitRangeRequest, []string) ([]diff.CommitStats, error) {
		return []diff.CommitStats{{SHA: sha, Additions: 1, FileCount: 1, PatchID: "p"}}, nil
	}
	ctx := context.Background()
	_, err := s.SetViewed(ctx, SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha, Viewed: true})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.List(ctx, PRRef{PRID: commitsTestPRID})
	if err != nil || !resp.Available || len(resp.Commits) != 1 {
		t.Fatalf("List = %+v, %v", resp, err)
	}
	got := resp.Commits[0]
	if !got.Viewed || got.Additions != 1 || got.FileCount != 1 {
		t.Fatalf("cache write failure lost computed stats or viewed key: %+v", got)
	}
	stored, err := s.Viewed.List(ctx, commitsTestPRID, "me")
	if err != nil || !reflect.DeepEqual(stored, map[string]int{"p:0": 1}) {
		t.Fatalf("viewed keys = %v, %v", stored, err)
	}
}
