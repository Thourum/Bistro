package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type interdiffGitStub struct {
	request git.InterdiffRequest
	result  git.InterdiffResult
	err     error
}

func (s *interdiffGitStub) Interdiff(req git.InterdiffRequest) (git.InterdiffResult, error) {
	s.request = req
	return s.result, s.err
}

func TestInterdiffHandlerContract(t *testing.T) {
	const prID = "github:o/r:1"
	const filePath = "a.go"
	const requestID = "interdiff-1"
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
		gitResult    git.InterdiffResult
		wantTo       int
		wantReason   string
		wantReady    bool
	}{
		{name: "latest patchset", patchsets: patchsets, hasRepo: true, gitResult: git.InterdiffResult{Patch: "head-to-head patch", Rebased: true}, wantTo: 5, wantReady: true},
		{name: "explicit identical pair", toPatchset: 3, patchsets: patchsets, hasRepo: true, wantTo: 3, wantReady: true},
		{name: "load failure", patchsetsErr: failure, wantReason: "could not load patchsets"},
		{name: "no patchsets", wantReason: "patchset is not recorded"},
		{name: "missing baseline", patchsets: patchsets[1:], wantReason: "patchset is not recorded"},
		{name: "missing target", toPatchset: 4, patchsets: patchsets, wantTo: 4, wantReason: "patchset is not recorded"},
		{name: "repo lookup failed", patchsets: patchsets, repoErr: failure, wantTo: 5, wantReason: "could not locate the repository"},
		{name: "repo not cached", patchsets: patchsets, wantTo: 5, wantReason: "no local copy of this repository yet"},
		{name: "target head lost", patchsets: patchsets, hasRepo: true, gitErr: fmt.Errorf("%w: h5", git.ErrHeadUnavailable), wantTo: 5, wantReason: "could not load a patchset head (P3 or P5): patchset head unavailable locally: h5"},
		{name: "diff failed", patchsets: patchsets, hasRepo: true, gitErr: failure, wantTo: 5, wantReason: "could not compute the interdiff"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gitClient := &interdiffGitStub{result: tt.gitResult, err: tt.gitErr}
			h := NewInterdiffHandler(InterdiffHandlerDeps{
				Patchsets: &mockPatchsetLister{patchsets: tt.patchsets, err: tt.patchsetsErr},
				Git:       gitClient,
				RepoForPR: func(_ context.Context, gotPRID string) (string, string, bool, error) {
					if gotPRID != prID {
						t.Errorf("repo lookup pr_id = %q, want %q", gotPRID, prID)
					}
					return repoURL, repoPath, tt.hasRepo, tt.repoErr
				},
				Log: testRepoLogger(),
			})
			payload, err := json.Marshal(map[string]interface{}{
				"pr_id": prID, "path": filePath, "from_patchset": 3, "to_patchset": tt.toPatchset,
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := h(context.Background(), ws.InboundMessage{ID: requestID, Type: "REQUEST_INTERDIFF", Data: payload})
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				ID   string                 `json:"id"`
				Type string                 `json:"type"`
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(encoded, &envelope); err != nil {
				t.Fatal(err)
			}
			want := map[string]interface{}{
				"pr_id": prID, "path": filePath, "from_patchset": float64(3), "to_patchset": float64(tt.wantTo),
				"available": tt.wantReady, "reason": tt.wantReason, "patch": tt.gitResult.Patch, "rebased": tt.gitResult.Rebased,
			}
			if envelope.ID != requestID || envelope.Type != "INTERDIFF_READY" || !reflect.DeepEqual(envelope.Data, want) {
				t.Fatalf("response = %s, want id=%q type=INTERDIFF_READY data=%v", encoded, requestID, want)
			}
			if !tt.wantReady {
				return
			}
			toHead, toMergeBase := "h5", "b5"
			if tt.wantTo == 3 {
				toHead, toMergeBase = "h3", "b3"
			}
			wantRequest := git.InterdiffRequest{
				RepoURL: repoURL, RepoPath: repoPath, Path: filePath,
				FromHead: "h3", ToHead: toHead, FromMergeBase: "b3", ToMergeBase: toMergeBase,
			}
			if gitClient.request != wantRequest {
				t.Errorf("git request = %+v, want %+v", gitClient.request, wantRequest)
			}
		})
	}
}

func TestInterdiffHandlerRejectsInvalidPayload(t *testing.T) {
	h := NewInterdiffHandler(InterdiffHandlerDeps{Log: testRepoLogger()})
	for _, payload := range []string{
		`{`,
		`{}`,
		`{"path":"a.go","from_patchset":3}`,
		`{"pr_id":"github:o/r:1","from_patchset":3}`,
		`{"pr_id":"github:o/r:1","path":"a.go"}`,
		`{"pr_id":"github:o/r:1","path":"a.go","from_patchset":-1}`,
		`{"pr_id":"github:o/r:1","path":"a.go","from_patchset":"3"}`,
	} {
		t.Run(payload, func(t *testing.T) {
			response, err := h(context.Background(), ws.InboundMessage{ID: "invalid-1", Type: "REQUEST_INTERDIFF", Data: json.RawMessage(payload)})
			if err == nil || response != nil {
				t.Errorf("response=%v err=%v, want payload error", response, err)
			}
		})
	}
}
