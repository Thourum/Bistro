package writer

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRecordPatchsetIfChanged_First(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	pr := &provider.PR{
		ID: "github:o/r:3", Provider: "github", Owner: "o", Repo: "r", Number: 3,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base", HeadSHA: "sha-1",
	}
	if err := w.WriteSyncResults([]*FetchedPR{{PR: pr}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	var num int
	var head string
	if err := db.QueryRow(`SELECT number, head_sha FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&num, &head); err != nil {
		t.Fatalf("patchset not found: %v", err)
	}
	if num != 1 {
		t.Errorf("first patchset: got number=%d want 1", num)
	}
	if head != "sha-1" {
		t.Errorf("head_sha: got %q want sha-1", head)
	}
}

func TestRecordPatchsetIfChanged_Unchanged(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	pr := &provider.PR{
		ID: "github:o/r:4", Provider: "github", Owner: "o", Repo: "r", Number: 4,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base", HeadSHA: "sha-same",
	}
	for i := 0; i < 3; i++ {
		if err := w.WriteSyncResults([]*FetchedPR{{PR: pr}}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("unchanged head: got %d patchset rows want 1", count)
	}
}

func TestRecordPatchsetIfChanged_Changed(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	base := &provider.PR{
		ID: "github:o/r:5", Provider: "github", Owner: "o", Repo: "r", Number: 5,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base", HeadSHA: "sha-1",
	}
	if err := w.WriteSyncResults([]*FetchedPR{{PR: base}}); err != nil {
		t.Fatalf("write P1: %v", err)
	}
	pushed := *base
	pushed.HeadSHA = "sha-2"
	if err := w.WriteSyncResults([]*FetchedPR{{PR: &pushed}}); err != nil {
		t.Fatalf("write P2: %v", err)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, base.ID).Scan(&count) //nolint:errcheck
	if count != 2 {
		t.Errorf("changed head: got %d patchset rows want 2", count)
	}
	var num int
	db.QueryRow(`SELECT number FROM patchsets WHERE pr_id=? AND head_sha='sha-2'`, base.ID).Scan(&num) //nolint:errcheck
	if num != 2 {
		t.Errorf("P2 number: got %d want 2", num)
	}
}

func TestRecordPatchsetIfChanged_BaseOnlyChanged(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	base := &provider.PR{
		ID: "github:o/r:6", Provider: "github", Owner: "o", Repo: "r", Number: 6,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base-1", HeadSHA: "sha-same",
	}
	mustWritePR(t, w, base)
	advanced := *base
	advanced.BaseSHA = "base-2"
	mustWritePR(t, w, &advanced)

	patchsets, err := db.ListPatchsets(context.Background(), base.ID)
	if err != nil || len(patchsets) != 1 {
		t.Fatalf("base-tip move: got %d patchset rows (err %v), want 1", len(patchsets), err)
	}
	current, ok, err := db.CurrentPatchset(context.Background(), base.ID)
	if err != nil || !ok || current.Number != 1 {
		t.Fatalf("current patchset = %+v, ok=%v err=%v; want P1 despite changed base tip", current, ok, err)
	}
}
