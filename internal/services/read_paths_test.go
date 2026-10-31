package services

import (
	"context"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestDiffStorageFailureIsInternal(t *testing.T) {
	db := storetest.Open(t)
	s := &Diff{DiffStore: prdiff.New(db.DB)}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := s.Get(context.Background(), PRRef{PRID: "github:o/r:1"})
	assertServiceCode(t, err, CodeInternal)
}

func TestReadPathsDistinguishUnknownPRFromEmptyPatchsets(t *testing.T) {
	db := storetest.Open(t)
	mustExecServicesDB(t, db.DB, `INSERT INTO prs (id,provider,owner,repo,number,title,author,state,created_at,updated_at,base_branch,head_branch,base_sha,head_sha)
 VALUES ('github:o/r:1','github','o','r',1,'T','a','open',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'main','f','base','head')`)
	t.Run("patchsets", func(t *testing.T) {
		s := &Patchsets{Store: db}
		_, err := s.List(context.Background(), PRRef{PRID: "github:o/r:2"})
		assertServiceCode(t, err, CodeNotFound)
		result, err := s.List(context.Background(), PRRef{PRID: "github:O/R:1"})
		if err != nil || result.Patchsets == nil || len(result.Patchsets) != 0 {
			t.Fatalf("known PR empty patchsets = %+v, %v", result, err)
		}
	})
	t.Run("triage", func(t *testing.T) {
		s := &Triage{Store: db}
		_, err := s.Get(context.Background(), TriageRequest{PRID: "github:o/r:2"})
		assertServiceCode(t, err, CodeNotFound)
		result, err := s.Get(context.Background(), TriageRequest{PRID: "github:O/R:1"})
		if err != nil || result.State != triageStateNoPatchsets || result.Files == nil {
			t.Fatalf("known PR empty triage = %+v, %v", result, err)
		}
	})
}

func TestReadPathsPreservePRLookupErrors(t *testing.T) {
	db := storetest.Open(t)
	checks := []struct {
		name string
		read func(context.Context) error
	}{
		{"patchsets", func(ctx context.Context) error {
			_, err := (&Patchsets{Store: db}).List(ctx, PRRef{PRID: "github:o/r:1"})
			return err
		}},
		{"triage", func(ctx context.Context) error {
			_, err := (&Triage{Store: db}).Get(ctx, TriageRequest{PRID: "github:o/r:1"})
			return err
		}},
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			assertServiceCode(t, check.read(cancelled), CodeCancelled)
			assertServiceCode(t, check.read(expired), CodeTimeout)
		})
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		t.Run(check.name+" storage failure", func(t *testing.T) { assertServiceCode(t, check.read(context.Background()), CodeInternal) })
	}
}

func TestDiffMissingRowsAreNotFound(t *testing.T) {
	db := storetest.Open(t)
	_, err := (&Diff{DiffStore: prdiff.New(db.DB)}).Get(context.Background(), PRRef{PRID: "github:o/r:1"})
	assertServiceCode(t, err, CodeNotFound)
}
