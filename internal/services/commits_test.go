package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/commitstats"
	"github.com/itterno/bistro/internal/store/commitviewed"
	"github.com/itterno/bistro/internal/store/storetest"
)

const commitsTestPRID = "github:o/r:1"

type commitsPatchsetStub struct {
	mockPatchsetLister
	current store.Patchset
	found   bool
}

func (s *commitsPatchsetStub) CurrentPatchset(context.Context, string) (store.Patchset, bool, error) {
	return s.current, s.found, s.err
}

func newCommitsTestService(t *testing.T) *Commits {
	t.Helper()
	db := storetest.Open(t).DB
	mustExecServicesDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github', 'o', 'r', 1, 'T', 'a', 'open',
		'2026-01-01', '2026-01-01', 'main', 'f', 'base', 'head')`, commitsTestPRID)
	return &Commits{
		Patchsets: &commitsPatchsetStub{current: store.Patchset{HeadSHA: "head", BaseSHA: "base", MergeBaseSHA: "merge-base"}, found: true},
		RepoForPR: func(context.Context, string) (string, string, bool, error) {
			return "repo-url", "repo-path", true, nil
		},
		CurrentUser: &mockCurrentUserReader{login: "me", found: true},
		Viewed:      commitviewed.New(db),
		Stats:       commitstats.New(db),
		ComputeStats: func(diff.CommitRangeRequest, []string) ([]diff.CommitStats, error) {
			t.Fatal("unexpected stats computation")
			return nil, nil
		},
		Log: testRepoLogger(),
	}
}

func TestCommitsGet_OutOfRangeSHA_ReturnsInvalidWithReason(t *testing.T) {
	s := newCommitsTestService(t)
	calls := 0
	s.Commit = func(diff.CommitRangeRequest, string) (diff.RangeCommit, error) {
		calls++
		return diff.RangeCommit{}, diff.ErrCommitNotInRange
	}
	_, err := s.Get(context.Background(), CommitRequest{PRID: commitsTestPRID, SHA: strings.Repeat("a", 40)})
	assertCommitError(t, err, CodeInvalid, ReasonCommitNotInPR)
	_, err = s.Get(context.Background(), CommitRequest{PRID: commitsTestPRID, SHA: "xyz"})
	assertCommitError(t, err, CodeInvalid, "")
	if calls != 1 {
		t.Fatalf("Commit called %d times; malformed SHA must not reach git", calls)
	}
}

func TestCommitsList_NoLocalCopy_Unavailable(t *testing.T) {
	s := newCommitsTestService(t)
	s.RepoForPR = func(context.Context, string) (string, string, bool, error) {
		return "", "", false, nil
	}
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		t.Fatal("Range called without a local repository")
		return diff.CommitRange{}, nil
	}
	resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	if err != nil || resp.Available || resp.Reason != "no local copy of this repository yet" || resp.Commits == nil {
		t.Fatalf("List = %+v, %v", resp, err)
	}
}

func TestCommitsViewed_SurvivesRebaseWithIdenticalContent(t *testing.T) {
	s := newCommitsTestService(t)
	sha1, sha2, sha3 := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	commits := []diff.RangeCommit{{SHA: sha1}}
	s.ComputeStats = fakeCommitStats(map[string]string{sha1: "p", sha2: "p", sha3: "q"})
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: commits}, nil
	}
	marked, err := s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha1, Viewed: true})
	if err != nil || !marked.Viewed || marked.SHA != sha1 {
		t.Fatalf("SetViewed = %+v, %v", marked, err)
	}
	commits[0] = diff.RangeCommit{SHA: sha2}
	assertCommitViewed(t, s, []bool{true})
	commits[0] = diff.RangeCommit{SHA: sha3}
	assertCommitViewed(t, s, []bool{false})
	commits[0] = diff.RangeCommit{SHA: sha2}
	if _, err := s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha2, Viewed: false}); err != nil {
		t.Fatal(err)
	}
	assertCommitViewed(t, s, []bool{false})
}

func TestCommitsViewed_DuplicatesStaySeparateAndResetOnRewrite(t *testing.T) {
	s := newCommitsTestService(t)
	sha1, sha2, rewritten := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	commits := []diff.RangeCommit{{SHA: sha1}, {SHA: sha2}}
	s.ComputeStats = fakeCommitStats(map[string]string{sha1: "p", sha2: "p", rewritten: "p"})
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: commits}, nil
	}
	if _, err := s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha1, Viewed: true}); err != nil {
		t.Fatal(err)
	}
	assertCommitViewed(t, s, []bool{true, false})
	commits = []diff.RangeCommit{{SHA: rewritten}}
	assertCommitViewed(t, s, []bool{false})
}

func TestCommitsViewed_MergeMarksResetWithSHAAndStayAccountScoped(t *testing.T) {
	s := newCommitsTestService(t)
	sha := strings.Repeat("a", 40)
	commits := []diff.RangeCommit{{SHA: sha, IsMerge: true}}
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{Commits: commits, Truncated: true}, nil
	}
	if _, err := s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha, Viewed: true}); err != nil {
		t.Fatal(err)
	}
	assertCommitViewed(t, s, []bool{true})
	resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	if err != nil || !resp.Truncated || !resp.Commits[0].IsMerge || resp.Commits[0].FileCount != 0 || resp.Commits[0].Additions != 0 || resp.Commits[0].Deletions != 0 {
		t.Fatalf("merge summary = %+v, %v", resp, err)
	}
	s.CurrentUser = &mockCurrentUserReader{login: "other", found: true}
	assertCommitViewed(t, s, []bool{false})
	s.CurrentUser = nil
	assertCommitViewed(t, s, []bool{false})
	s.CurrentUser = &mockCurrentUserReader{login: "me", found: true}
	commits[0].SHA = strings.Repeat("b", 40)
	assertCommitViewed(t, s, []bool{false})
}

func TestCommitsViewed_RequiresLoginAndCurrentRange(t *testing.T) {
	s := newCommitsTestService(t)
	sha := strings.Repeat("a", 40)
	s.CurrentUser = nil
	_, err := s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha, Viewed: true})
	assertCommitError(t, err, CodeUnauthenticated, "")
	s.CurrentUser = &mockCurrentUserReader{login: "me", found: true}
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) { return diff.CommitRange{}, nil }
	_, err = s.SetViewed(context.Background(), SetCommitViewedRequest{PRID: commitsTestPRID, SHA: sha, Viewed: true})
	assertCommitError(t, err, CodeInvalid, ReasonCommitNotInPR)
	stored, err := s.Viewed.List(context.Background(), commitsTestPRID, "me")
	if err != nil || len(stored) != 0 {
		t.Fatalf("invalid write persisted marks: %v, %v", stored, err)
	}
}

func TestCommitsGet_ResolvesFallbackAndPreservesDiff(t *testing.T) {
	s := newCommitsTestService(t)
	s.Patchsets = &commitsPatchsetStub{current: store.Patchset{BaseSHA: "base", HeadSHA: "head"}, found: true}
	s.MergeBase = func(path, base, head string) (string, error) {
		if path != "repo-path" || base != "base" || head != "head" {
			t.Fatalf("MergeBase arguments = %q, %q, %q", path, base, head)
		}
		return "fallback", nil
	}
	sha := strings.Repeat("a", 40)
	s.Commit = func(req diff.CommitRangeRequest, gotSHA string) (diff.RangeCommit, error) {
		if req.RepoURL != "repo-url" || req.RepoPath != "repo-path" || req.HeadSHA != "head" || req.MergeBaseSHA != "fallback" || gotSHA != sha {
			t.Fatalf("Commit arguments = %+v, %q", req, gotSHA)
		}
		return diff.RangeCommit{Diff: &diff.CommitDiff{Unified: "patch", Additions: 2, Deletions: 1, Files: []*diff.File{
			{Path: "image.png", Status: "binary", Binary: true, DiffStatus: diff.StatusOK},
			{Path: "new.txt", OldPath: "old.txt", Status: "renamed", Truncated: true, DiffStatus: diff.StatusTruncated, PatchBytes: 600 * 1024, HunkCount: 1, AddedLines: 2, RemovedLines: 1},
		}}}, nil
	}
	resp, err := s.Get(context.Background(), CommitRequest{PRID: "github:O/R:1", SHA: strings.ToUpper(sha)})
	if err != nil || !resp.Available || resp.PRID != commitsTestPRID || resp.SHA != sha || resp.Diff != "patch" || resp.Additions != 2 || resp.Deletions != 1 {
		t.Fatalf("Get = %+v, %v", resp, err)
	}
	wantFiles := []DiffFile{
		{Path: "image.png", Status: "binary", IsBinary: true, DiffStatus: "ok"},
		{Path: "new.txt", OldPath: "old.txt", Status: "renamed", IsTruncated: true, DiffStatus: "truncated", PatchBytes: 600 * 1024, HunkCount: 1, Additions: 2, Deletions: 1},
	}
	if !reflect.DeepEqual(resp.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", resp.Files, wantFiles)
	}
}

func TestCommitsList_MapsRangeFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		reason  string
		code    Code
	}{
		{name: "missing objects", failure: diff.ErrHeadUnavailable, reason: "the PR head or merge base is not in the local repository"},
		{name: "git failure", failure: errors.New("git failed"), reason: "could not list commits"},
		{name: "cancelled", failure: context.Canceled, code: CodeCancelled},
		{name: "timeout", failure: context.DeadlineExceeded, code: CodeTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newCommitsTestService(t)
			s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) { return diff.CommitRange{}, test.failure }
			resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
			if test.code != "" {
				assertCommitError(t, err, test.code, "")
				return
			}
			if err != nil || resp.Available || resp.Reason != test.reason {
				t.Fatalf("List = %+v, %v", resp, err)
			}
		})
	}
}

func TestCommitsSetViewed_MissingLocalObjects_ReturnsInvalid(t *testing.T) {
	s := newCommitsTestService(t)
	s.Range = func(diff.CommitRangeRequest) (diff.CommitRange, error) {
		return diff.CommitRange{}, fmt.Errorf("load range: %w", diff.ErrHeadUnavailable)
	}
	_, err := s.SetViewed(context.Background(), SetCommitViewedRequest{
		PRID: commitsTestPRID, SHA: strings.Repeat("a", 40), Viewed: true,
	})
	assertCommitError(t, err, CodeInvalid, "")
	resp, listErr := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	if listErr != nil || err.Error() != Errorf(CodeInvalid, "%s", resp.Reason).Error() || resp.Reason != "the PR head or merge base is not in the local repository" {
		t.Fatalf("SetViewed error = %v; List = %+v, %v", err, resp, listErr)
	}
	stored, storeErr := s.Viewed.List(context.Background(), commitsTestPRID, "me")
	if storeErr != nil || len(stored) != 0 {
		t.Fatalf("unavailable range persisted marks: %v, %v", stored, storeErr)
	}
}

func TestCommitsList_NotFound(t *testing.T) {
	s := newCommitsTestService(t)
	s.RepoForPR = func(context.Context, string) (string, string, bool, error) { return "", "", false, sql.ErrNoRows }
	_, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	assertCommitError(t, err, CodeNotFound, "")
}

func assertCommitError(t *testing.T, err error, code Code, reason string) {
	t.Helper()
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != code || serviceErr.Reason != reason {
		t.Fatalf("error = %v; want %s with reason %q", err, code, reason)
	}
}

func assertCommitViewed(t *testing.T, s *Commits, want []bool) {
	t.Helper()
	resp, err := s.List(context.Background(), PRRef{PRID: commitsTestPRID})
	if err != nil || !resp.Available || len(resp.Commits) != len(want) {
		t.Fatalf("List = %+v, %v; want %d commits", resp, err, len(want))
	}
	for i, viewed := range want {
		if resp.Commits[i].Viewed != viewed {
			t.Errorf("commit %d viewed = %v, want %v", i, resp.Commits[i].Viewed, viewed)
		}
	}
}
