package writer

import (
	"context"
	"reflect"
	"testing"

	gitdiff "github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/store/commitstats"
	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestWriteSyncResults_CommitStatsReplacedAndPrunedWithPR(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	const prNumber = 25
	pr := makePR("github:o/r:25", prNumber, "head", "base")
	first := commitstats.Stats{SHA: "a", Additions: 1, FileCount: 1, PatchID: "patch-a"}
	second := commitstats.Stats{SHA: "b", Deletions: 1, FileCount: 1, PatchID: "patch-b"}
	cache := commitstats.New(db.DB)
	for _, step := range []struct {
		name     string
		stats    []commitstats.Stats
		computed bool
		want     map[string]commitstats.Stats
	}{
		{"new PR", []commitstats.Stats{first, second}, true, map[string]commitstats.Stats{"a": first, "b": second}},
		{"force push", []commitstats.Stats{second}, true, map[string]commitstats.Stats{"b": second}},
		{"not computed", []commitstats.Stats{first}, false, map[string]commitstats.Stats{"b": second}},
		{"empty range", []commitstats.Stats{}, true, map[string]commitstats.Stats{}},
	} {
		t.Run(step.name, func(t *testing.T) {
			if err := w.WriteSyncResults([]*FetchedPR{{
				PR: pr, CommitStats: step.stats, CommitStatsComputed: step.computed,
			}}); err != nil {
				t.Fatalf("WriteSyncResults: %v", err)
			}
			got, err := cache.List(context.Background(), pr.ID, gitdiff.CommitStatsVersion)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if !reflect.DeepEqual(got, step.want) {
				t.Fatalf("stats = %+v, want %+v", got, step.want)
			}
		})
	}
}

func TestWriteSyncResults_CommitStatsFailureKeepsPR(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	if _, err := db.Exec("DROP TABLE commit_stats"); err != nil {
		t.Fatalf("drop commit_stats: %v", err)
	}
	const prNumber = 26
	pr := makePR("github:o/r:26", prNumber, "head", "base")
	const patch = "+new line"
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: pr, DiffComputed: true,
		DiffFiles:           []prdiff.File{{FilePath: "main.go", Patch: patch, Additions: 1}},
		CommitStats:         []commitstats.Stats{{SHA: "head", Additions: 1, FileCount: 1, PatchID: "patch"}},
		CommitStatsComputed: true,
	}}); err != nil {
		t.Fatalf("stats failure must not fail the write: %v", err)
	}
	var headSHA string
	if err := db.QueryRow("SELECT head_sha FROM prs WHERE id = ?", pr.ID).Scan(&headSHA); err != nil {
		t.Fatalf("PR row missing: %v", err)
	}
	if headSHA != pr.HeadSHA {
		t.Fatalf("head = %q, want %q", headSHA, pr.HeadSHA)
	}
	var gotPatch string
	if err := db.QueryRow("SELECT patch FROM pr_diffs WHERE pr_id = ? AND file_path = 'main.go'", pr.ID).Scan(&gotPatch); err != nil {
		t.Fatalf("diff missing: %v", err)
	}
	if gotPatch != patch {
		t.Fatalf("patch = %q, want %q", gotPatch, patch)
	}
	var patchsets int
	if err := db.QueryRow("SELECT COUNT(*) FROM patchsets WHERE pr_id = ?", pr.ID).Scan(&patchsets); err != nil {
		t.Fatalf("patchsets: %v", err)
	}
	if patchsets != 1 {
		t.Fatalf("patchsets = %d, want 1", patchsets)
	}
}

func TestWriteSyncResults_CommitStatsPartialFailureRestoresOldRows(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)
	const prNumber = 27
	pr := makePR("github:o/r:27", prNumber, "old-head", "base")
	old := commitstats.Stats{SHA: "old", Additions: 3, FileCount: 1, PatchID: "old-patch"}
	if err := w.WriteSyncResults([]*FetchedPR{{PR: pr, CommitStats: []commitstats.Stats{old}, CommitStatsComputed: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_second_stat BEFORE INSERT ON commit_stats
 WHEN NEW.sha = 'second' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	pr.HeadSHA = "new-head"
	const patch = "+new line"
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: pr, DiffComputed: true,
		DiffFiles:           []prdiff.File{{FilePath: "main.go", Patch: patch, Additions: 1}},
		CommitStats:         []commitstats.Stats{{SHA: "first", PatchID: "first-patch"}, {SHA: "second", PatchID: "second-patch"}},
		CommitStatsComputed: true,
	}}); err != nil {
		t.Fatalf("stats failure must not fail the write: %v", err)
	}
	got, err := commitstats.New(db.DB).List(context.Background(), pr.ID, gitdiff.CommitStatsVersion)
	if err != nil || !reflect.DeepEqual(got, map[string]commitstats.Stats{"old": old}) {
		t.Fatalf("old stats not restored: %+v, %v", got, err)
	}
	var head, gotPatch string
	if err := db.QueryRow("SELECT head_sha FROM prs WHERE id = ?", pr.ID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if head != pr.HeadSHA {
		t.Fatalf("head = %q, want %q", head, pr.HeadSHA)
	}
	if err := db.QueryRow("SELECT patch FROM pr_diffs WHERE pr_id = ? AND file_path = 'main.go'", pr.ID).Scan(&gotPatch); err != nil {
		t.Fatal(err)
	}
	if gotPatch != patch {
		t.Fatalf("patch = %q, want %q", gotPatch, patch)
	}
	var patchsets int
	if err := db.QueryRow("SELECT COUNT(*) FROM patchsets WHERE pr_id = ? AND head_sha = ?", pr.ID, pr.HeadSHA).Scan(&patchsets); err != nil {
		t.Fatal(err)
	}
	if patchsets != 1 {
		t.Fatalf("new patchsets = %d, want 1", patchsets)
	}
}
