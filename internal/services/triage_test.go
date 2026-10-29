package services

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestTriageFilesClassifiesEveryPath(t *testing.T) {
	baseline := map[string]string{"same.go": "p1", "edited.go": "p2", "gone.go": "p3"}
	target := map[string]string{"same.go": "p1", "edited.go": "p2b", "added.go": "p4"}

	got := triageFiles(baseline, target)

	want := []FileTriage{
		{Path: "added.go", Status: triageNew},
		{Path: "edited.go", Status: triageChanged},
		{Path: "gone.go", Status: triageRemoved},
		{Path: "same.go", Status: triageUnchanged},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("triageFiles = %+v, want %+v", got, want)
	}
}

func TestResolveBaselinePatchset(t *testing.T) {
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "h1"}, {Number: 2, HeadSHA: "h2"}, {Number: 3, HeadSHA: "h2"},
	}
	tests := []struct {
		name   string
		record store.LastReviewedRecord
		want   int
		found  bool
	}{
		{"stored number with matching head", store.LastReviewedRecord{SHA: "h2", PatchsetNum: 2}, 2, true},
		{"stale number falls back to latest at head", store.LastReviewedRecord{SHA: "h2", PatchsetNum: 1}, 3, true},
		{"unknown number matches by head", store.LastReviewedRecord{SHA: "h1"}, 1, true},
		{"head never recorded", store.LastReviewedRecord{SHA: "browser-only"}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := resolveBaselinePatchset(patchsets, tt.record)
			if found != tt.found || got.Number != tt.want {
				t.Errorf("got (P%d, %v), want (P%d, %v)", got.Number, found, tt.want, tt.found)
			}
		})
	}
}

type triageStoreStub struct {
	mockPatchsetLister
	files     map[int]map[string]string
	filesErr  map[int]error
	record    store.LastReviewedRecord
	hasReview bool
	reviewErr error
}

func (s *triageStoreStub) ListPatchsetFiles(_ context.Context, _ string, number int) (map[string]string, error) {
	return s.files[number], s.filesErr[number]
}

func (s *triageStoreStub) GetLastReviewed(_ context.Context, _, _ string) (store.LastReviewedRecord, bool, error) {
	return s.record, s.hasReview, s.reviewErr
}

func TestTriageHandlerReturnsPairAndReviewContracts(t *testing.T) {
	const prID = "github:o/r:1"
	baselineFiles := map[string]string{"same.go": "p1", "edited.go": "p2", "gone.go": "p3"}
	targetFiles := map[string]string{"same.go": "p1", "edited.go": "p2b", "added.go": "p4"}
	reviewedAt := time.Date(2026, time.October, 21, 11, 0, 0, 0, time.FixedZone("review", 2*60*60))
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "h1", HasFingerprints: true},
		{Number: 2, HeadSHA: "h2", HasFingerprints: true},
		{Number: 3, HeadSHA: "h3", HasFingerprints: true},
	}
	for _, tt := range []struct {
		name         string
		req          TriageRequest
		currentUser  currentUserReader
		wantTo       int
		wantBaseline *TriageBaseline
	}{
		{
			name: "explicit pair does not need a user", req: TriageRequest{PRID: prID, FromPatchset: 1, ToPatchset: 2}, wantTo: 2,
		},
		{
			name: "default pair uses last review and latest", req: TriageRequest{PRID: prID},
			currentUser: &mockCurrentUserReader{login: "alice", found: true}, wantTo: 3,
			wantBaseline: &TriageBaseline{Source: "github_review", ReviewedAt: "2026-10-21T09:00:00Z"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &triageStoreStub{
				mockPatchsetLister: mockPatchsetLister{patchsets: patchsets},
				files:              map[int]map[string]string{1: baselineFiles, 2: targetFiles, 3: targetFiles},
				record:             store.LastReviewedRecord{SHA: "h1", ReviewedAt: reviewedAt, Source: store.ReviewSourceGitHubReview}, hasReview: true,
			}
			s := &Triage{Store: db, CurrentUser: tt.currentUser, Log: testRepoLogger()}
			response, err := s.Get(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			assertTriageJSONShape(t, encoded, tt.wantBaseline)
			want := TriageResponse{
				PRID: prID, State: triageStateOK, FromPatchset: 1, ToPatchset: tt.wantTo, Baseline: tt.wantBaseline,
				Files: []FileTriage{
					{Path: "added.go", Status: triageNew}, {Path: "edited.go", Status: triageChanged},
					{Path: "gone.go", Status: triageRemoved}, {Path: "same.go", Status: triageUnchanged},
				},
				ChangedCount: 3, TotalCount: 4,
			}
			if !reflect.DeepEqual(response, want) {
				t.Errorf("response = %+v, want %+v", response, want)
			}
		})
	}
}

func TestDefaultTargetsUseCurrentPatchsetAfterForcePushBack(t *testing.T) {
	db := storetest.Open(t)
	ctx := context.Background()
	const prID = "github:o/r:1"
	mustExecServicesDB(t, db.DB, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state, created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github', 'o', 'r', 1, 'T', 'a', 'open', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'main', 'f', 'tip', 'h1')`, prID)
	mustExecServicesDB(t, db.DB, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, merge_base_sha, pushed_at, has_fingerprints)
		VALUES (?, 1, 'h1', 'tip', 'merge-1', CURRENT_TIMESTAMP, 1),
		       (?, 2, 'h2', 'tip', 'merge-2', CURRENT_TIMESTAMP, 1)`, prID, prID)
	mustExecServicesDB(t, db.DB, `INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id)
		VALUES (?, 1, 'a.go', 'patch-1'), (?, 2, 'a.go', 'patch-2')`, prID, prID)
	mustExecServicesDB(t, db.DB, `UPDATE prs SET head_sha = 'h2' WHERE id = ?`, prID)
	mustExecServicesDB(t, db.DB, `UPDATE prs SET head_sha = 'h1' WHERE id = ?`, prID)

	triage, err := (&Triage{Store: db}).Get(ctx, TriageRequest{PRID: prID, FromPatchset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if triage.State != triageStateOK || triage.ToPatchset != 1 || triage.ChangedCount != 0 ||
		!reflect.DeepEqual(triage.Files, []FileTriage{{Path: "a.go", Status: triageUnchanged}}) {
		t.Errorf("triage after force-push back = %+v, want unchanged P1", triage)
	}
	gitClient := &interdiffGitStub{}
	interdiff, err := (&Interdiff{
		Patchsets: db, Interdiff: gitClient.Interdiff,
		RepoForPR: func(context.Context, string) (string, string, bool, error) {
			return "https://github.com/o/r.git", "/local/repo", true, nil
		},
	}).Get(ctx, InterdiffRequest{PRID: prID, Path: "a.go", FromPatchset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !interdiff.Available || interdiff.ToPatchset != 1 || gitClient.request.ToHead != "h1" || gitClient.request.ToMergeBase != "merge-1" {
		t.Errorf("interdiff after force-push back = %+v, git request = %+v; want current P1", interdiff, gitClient.request)
	}
}

func TestTriageHandlerUnavailableStates(t *testing.T) {
	const prID = "github:o/r:1"
	lookupErr := errors.New("storage unavailable")
	withFingerprints := []store.Patchset{{Number: 1, HeadSHA: "h1", HasFingerprints: true}, {Number: 2, HeadSHA: "h2", HasFingerprints: true}}
	withoutBaselineFingerprints := []store.Patchset{{Number: 1, HeadSHA: "h1"}, {Number: 2, HeadSHA: "h2", HasFingerprints: true}}
	withoutTargetFingerprints := []store.Patchset{{Number: 1, HeadSHA: "h1", HasFingerprints: true}, {Number: 2, HeadSHA: "h2"}}
	for _, tt := range []struct {
		name        string
		db          triageStoreStub
		from        int
		to          int
		currentUser currentUserReader
		wantState   string
		wantReason  string
	}{
		{name: "no patchsets", wantState: triageStateNoPatchsets, wantReason: "no patchsets recorded yet for this PR"},
		{name: "patchset lookup failure", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{err: lookupErr}}, wantState: triageStateNoPatchsets, wantReason: "could not load patchsets"},
		{name: "target not recorded", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}}, from: 1, to: 3, wantState: triageStateNoPatchsets, wantReason: "P3 is not recorded"},
		{name: "baseline not recorded", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}}, from: 3, wantState: triageStateNoBaseline, wantReason: "P3 is not recorded"},
		{name: "no authenticated user", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}}, currentUser: &mockCurrentUserReader{}, wantState: triageStateNoBaseline, wantReason: "no authenticated user"},
		{name: "current user lookup failure", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}}, currentUser: &errorCurrentUserReader{err: lookupErr}, wantState: triageStateNoBaseline, wantReason: "no authenticated user"},
		{name: "review lookup failure", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}, reviewErr: lookupErr}, currentUser: &mockCurrentUserReader{login: "alice", found: true}, wantState: triageStateNoBaseline, wantReason: "could not load your last review"},
		{name: "no prior review", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}}, currentUser: &mockCurrentUserReader{login: "alice", found: true}, wantState: triageStateNoBaseline, wantReason: "no prior review recorded"},
		{name: "unrecorded review head", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}, record: store.LastReviewedRecord{SHA: "browser-only"}, hasReview: true}, currentUser: &mockCurrentUserReader{login: "alice", found: true}, wantState: triageStateNoBaseline, wantReason: "no comparable prior review: Bistro never recorded the head you reviewed"},
		{name: "baseline predates fingerprints", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withoutBaselineFingerprints}}, from: 1, wantState: triageStateNoFingerprints, wantReason: "P1 was recorded before file fingerprints; triage needs a newer patchset"},
		{name: "target predates fingerprints", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withoutTargetFingerprints}}, from: 1, wantState: triageStateNoFingerprints, wantReason: "P2 was recorded before file fingerprints; triage needs a newer patchset"},
		{name: "baseline files lookup failure", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}, filesErr: map[int]error{1: lookupErr}}, from: 1, wantState: triageStateNoFingerprints, wantReason: "could not load file fingerprints"},
		{name: "target files lookup failure", db: triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: withFingerprints}, filesErr: map[int]error{2: lookupErr}}, from: 1, wantState: triageStateNoFingerprints, wantReason: "could not load file fingerprints"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Triage{Store: &tt.db, CurrentUser: tt.currentUser, Log: testRepoLogger()}
			got, err := s.Get(context.Background(), TriageRequest{PRID: prID, FromPatchset: tt.from, ToPatchset: tt.to})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			assertTriageJSONShape(t, encoded, nil)
			if got.PRID != prID || got.State != tt.wantState || got.Reason != tt.wantReason {
				t.Errorf("response = %+v, want state %q and reason %q", got, tt.wantState, tt.wantReason)
			}
			if got.Files == nil || len(got.Files) != 0 || got.ChangedCount != 0 || got.TotalCount != 0 {
				t.Errorf("unavailable response must have [] files and zero counts: %+v", got)
			}
		})
	}
}

func TestTriageHandlerEmptyFingerprintSnapshots(t *testing.T) {
	db := &triageStoreStub{mockPatchsetLister: mockPatchsetLister{patchsets: []store.Patchset{
		{Number: 1, HeadSHA: "h1", HasFingerprints: true},
		{Number: 2, HeadSHA: "h2", HasFingerprints: true},
	}}}
	response, err := (&Triage{Store: db}).Get(context.Background(), TriageRequest{PRID: "github:o/r:1", FromPatchset: 1})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	assertTriageJSONShape(t, encoded, nil)
	if response.State != triageStateOK || response.Files == nil || len(response.Files) != 0 || response.ChangedCount != 0 || response.TotalCount != 0 {
		t.Fatalf("empty snapshot = %+v", response)
	}
}

func assertTriageJSONShape(t *testing.T, encoded []byte, baseline *TriageBaseline) {
	t.Helper()
	var fieldsByName map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fieldsByName); err != nil {
		t.Fatal(err)
	}
	fields := []string{"pr_id", "state", "reason", "from_patchset", "to_patchset", "baseline", "files", "changed_count", "total_count"}
	if len(fieldsByName) != len(fields) {
		t.Errorf("TRIAGE_READY fields = %d, want %d: %s", len(fieldsByName), len(fields), encoded)
	}
	for _, field := range fields {
		if _, ok := fieldsByName[field]; !ok {
			t.Errorf("TRIAGE_READY missing %q: %s", field, encoded)
		}
	}
	if string(fieldsByName["files"]) == "null" {
		t.Errorf("files must encode as an array: %s", encoded)
	}
	if baseline == nil && string(fieldsByName["baseline"]) != "null" {
		t.Errorf("explicit pair must encode baseline:null: %s", encoded)
	}
}

func TestTriageRejectsInvalidRequest(t *testing.T) {
	for _, req := range []TriageRequest{{}, {PRID: "p", FromPatchset: -1}, {PRID: "p", ToPatchset: -1}} {
		_, err := (&Triage{}).Get(context.Background(), req)
		assertServiceCode(t, err, CodeInvalid)
	}
}
