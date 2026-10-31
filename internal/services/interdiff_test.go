package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/store"
)

type interdiffGitStub struct {
	request diff.InterdiffRequest
	result  diff.InterdiffResult
	err     error
}

func (s *interdiffGitStub) Interdiff(req diff.InterdiffRequest) (diff.InterdiffResult, error) {
	s.request = req
	return s.result, s.err
}

func TestInterdiffHandlerContract(t *testing.T) {
	const prID = "github:o/r:1"
	const filePath = "a.go"
	const repoURL = "https://github.com/o/r.git"
	const repoPath = "/local/repo"
	patchsets := []store.Patchset{
		{Number: 3, HeadSHA: "h3", MergeBaseSHA: "b3"},
		{Number: 5, HeadSHA: "h5", MergeBaseSHA: "b5"},
	}
	failure := errors.New("backend failed")
	for _, tt := range []struct {
		name         string
		toPatchset   int
		patchsets    []store.Patchset
		patchsetsErr error
		repoErr      error
		hasRepo      bool
		gitErr       error
		gitResult    diff.InterdiffResult
		wantTo       int
		wantReason   string
		wantReady    bool
	}{
		{name: "latest patchset", patchsets: patchsets, hasRepo: true, gitResult: diff.InterdiffResult{Patch: "head-to-head patch", Rebased: true}, wantTo: 5, wantReady: true},
		{name: "explicit identical pair", toPatchset: 3, patchsets: patchsets, hasRepo: true, wantTo: 3, wantReady: true},
		{name: "load failure", patchsetsErr: failure, wantReason: "could not load patchsets"},
		{name: "no patchsets", wantReason: "patchset is not recorded"},
		{name: "missing baseline", patchsets: patchsets[1:], wantReason: "patchset is not recorded"},
		{name: "missing target", toPatchset: 4, patchsets: patchsets, wantTo: 4, wantReason: "patchset is not recorded"},
		{name: "repo lookup failed", patchsets: patchsets, repoErr: failure, wantTo: 5, wantReason: "could not locate the repository"},
		{name: "repo not cached", patchsets: patchsets, wantTo: 5, wantReason: "no local copy of this repository yet"},
		{name: "target head lost", patchsets: patchsets, hasRepo: true, gitErr: fmt.Errorf("%w: h5", diff.ErrHeadUnavailable), wantTo: 5, wantReason: "could not load a patchset head (P3 or P5): patchset head unavailable locally: h5"},
		{name: "diff failed", patchsets: patchsets, hasRepo: true, gitErr: failure, wantTo: 5, wantReason: "could not compute the interdiff"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gitClient := &interdiffGitStub{result: tt.gitResult, err: tt.gitErr}
			s := &Interdiff{
				Patchsets: &mockPatchsetLister{patchsets: tt.patchsets, err: tt.patchsetsErr},
				Interdiff: gitClient.Interdiff,
				RepoForPR: func(_ context.Context, gotPRID string) (string, string, bool, error) {
					if gotPRID != prID {
						t.Errorf("repo lookup pr_id = %q, want %q", gotPRID, prID)
					}
					return repoURL, repoPath, tt.hasRepo, tt.repoErr
				},
				Log: testRepoLogger(),
			}
			response, err := s.Get(context.Background(), InterdiffRequest{PRID: prID, Path: filePath, FromPatchset: 3, ToPatchset: tt.toPatchset})
			if err != nil {
				t.Fatal(err)
			}
			want := InterdiffResponse{PRID: prID, Path: filePath, FromPatchset: 3, ToPatchset: tt.wantTo, Available: tt.wantReady, Reason: tt.wantReason, Patch: tt.gitResult.Patch, Rebased: tt.gitResult.Rebased}
			if !reflect.DeepEqual(response, want) {
				t.Fatalf("response = %+v, want %+v", response, want)
			}
			if !tt.wantReady {
				return
			}
			toHead, toMergeBase := "h5", "b5"
			if tt.wantTo == 3 {
				toHead, toMergeBase = "h3", "b3"
			}
			wantRequest := diff.InterdiffRequest{
				RepoURL: repoURL, RepoPath: repoPath, Path: filePath,
				FromHead: "h3", ToHead: toHead, FromMergeBase: "b3", ToMergeBase: toMergeBase,
			}
			if gitClient.request != wantRequest {
				t.Errorf("git request = %+v, want %+v", gitClient.request, wantRequest)
			}
		})
	}
}

func TestInterdiffRejectsInvalidRequest(t *testing.T) {
	for _, req := range []InterdiffRequest{{}, {Path: "a.go", FromPatchset: 3}, {PRID: "p", FromPatchset: 3}, {PRID: "p", Path: "a.go"}, {PRID: "p", Path: "a.go", FromPatchset: -1}} {
		_, err := (&Interdiff{}).Get(context.Background(), req)
		assertServiceCode(t, err, CodeInvalid)
	}
}

func TestInterdiffUnknownPRIsNotFound(t *testing.T) {
	s := &Interdiff{Patchsets: &mockPatchsetLister{}, RepoForPR: func(context.Context, string) (string, string, bool, error) { return "", "", false, sql.ErrNoRows }}
	_, err := s.Get(context.Background(), InterdiffRequest{PRID: "github:O/R:1", Path: "a.go", FromPatchset: 1})
	assertServiceCode(t, err, CodeNotFound)
}
