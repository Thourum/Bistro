package services

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/prdiff"
)

type mockPatchsetLister struct {
	patchsets []store.Patchset
	err       error
}

func (m *mockPatchsetLister) ListPatchsets(context.Context, string) ([]store.Patchset, error) {
	return m.patchsets, m.err
}
func (m *mockPatchsetLister) CurrentPatchset(context.Context, string) (store.Patchset, bool, error) {
	return store.Patchset{}, false, nil
}

func (m *mockPatchsetLister) GetPRCheckoutInfo(context.Context, string) (*store.PRCheckoutInfo, error) {
	return &store.PRCheckoutInfo{}, nil
}

type errorCurrentUserReader struct{ err error }

func (s *errorCurrentUserReader) Get(context.Context, string) (string, bool, error) {
	return "", false, s.err
}
func mustExecServicesDB(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestPatchsetsList(t *testing.T) {
	for _, patchsets := range [][]store.Patchset{nil, {{Number: 1}, {Number: 2}}} {
		response, err := (&Patchsets{Store: &mockPatchsetLister{patchsets: patchsets}}).List(context.Background(), PRRef{PRID: "github:O/R:1"})
		if err != nil {
			t.Fatal(err)
		}
		if response.PRID != "github:o/r:1" || response.Patchsets == nil || len(response.Patchsets) != len(patchsets) {
			t.Fatalf("response = %+v", response)
		}
		if len(patchsets) > 0 && !reflect.DeepEqual(response.Patchsets, patchsets) {
			t.Fatalf("patchsets = %+v", response.Patchsets)
		}
	}
	_, err := (&Patchsets{}).List(context.Background(), PRRef{})
	assertServiceCode(t, err, CodeInvalid)
}

func TestViewedValidatesHashBeforeWriting(t *testing.T) {
	db, diffs := newViewedBaselineTestStores(t)
	s := &Viewed{Viewed: db, DiffStore: diffs, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}}
	const mismatchReason = "content_hash mismatch: client stale… server id-a… (file content may have changed)"
	_, err := s.MarkFile(context.Background(), MarkFileViewedRequest{PRID: "github:O/R:1", FilePath: "a.go", ContentHash: "stale"})
	se := assertServiceCode(t, err, CodeInvalid)
	if se.Reason != mismatchReason {
		t.Fatalf("reason = %q, want %q", se.Reason, mismatchReason)
	}
	viewed, err := db.ListViewedFiles(context.Background(), "github:o/r:1", "alice")
	if err != nil || len(viewed) != 0 {
		t.Fatalf("viewed after rejection = %+v, %v", viewed, err)
	}
	result, err := s.MarkFile(context.Background(), MarkFileViewedRequest{PRID: "github:O/R:1", FilePath: "a.go", ContentHash: "id-a"})
	if err != nil {
		t.Fatal(err)
	}
	if result != (FileViewed{PRID: "github:o/r:1", FilePath: "a.go"}) {
		t.Fatalf("result = %+v", result)
	}
	viewed, err = db.ListViewedFiles(context.Background(), "github:o/r:1", "alice")
	if err != nil || len(viewed) != 1 || viewed[0].ContentHash != "id-a" {
		t.Fatalf("viewed = %+v, %v", viewed, err)
	}
	s.CurrentUser = &mockCurrentUserReader{}
	_, err = s.MarkFile(context.Background(), MarkFileViewedRequest{PRID: "p", FilePath: "a.go", ContentHash: "hash"})
	assertServiceCode(t, err, CodeUnauthenticated)
}

type errorDiffReader struct{ err error }

func (s errorDiffReader) GetUnified(string) (string, error) { return "", s.err }
func (s errorDiffReader) Get(string) ([]prdiff.File, error) { return nil, s.err }
func TestServicesPreserveContextErrors(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		code := CodeCancelled
		if errors.Is(cause, context.DeadlineExceeded) {
			code = CodeTimeout
		}
		_, err := (&Diff{DiffStore: errorDiffReader{err: cause}}).Get(context.Background(), PRRef{PRID: "p"})
		assertServiceCode(t, err, code)
		_, err = (&Patchsets{Store: &mockPatchsetLister{err: cause}}).List(context.Background(), PRRef{PRID: "p"})
		assertServiceCode(t, err, code)
		_, err = (&Triage{Store: &triageStoreStub{mockPatchsetLister: mockPatchsetLister{err: cause}}}).Get(context.Background(), TriageRequest{PRID: "p"})
		assertServiceCode(t, err, code)
		_, err = (&Interdiff{Patchsets: &mockPatchsetLister{err: cause}, RepoForPR: func(context.Context, string) (string, string, bool, error) { return "", "", false, nil }}).Get(context.Background(), InterdiffRequest{PRID: "p", Path: "a.go", FromPatchset: 1})
		assertServiceCode(t, err, code)
		_, err = (&Viewed{CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, DiffStore: errorDiffReader{err: cause}}).MarkFile(context.Background(), MarkFileViewedRequest{PRID: "p", FilePath: "a.go", ContentHash: "hash"})
		assertServiceCode(t, err, code)
		_, err = (&ReadState{CurrentUser: &errorCurrentUserReader{err: cause}}).MarkRead(context.Background(), MarkReadRequest{PRIDs: []string{"p"}})
		assertServiceCode(t, err, code)
	}
}
