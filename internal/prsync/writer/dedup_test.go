package writer

import (
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/storetest"
)

func dedupGroupIDs(t *testing.T, w *Writer, ids ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, id := range ids {
		var group *string
		if err := w.db.QueryRow(`SELECT dedup_group_id FROM comments WHERE id = ?`, id).Scan(&group); err != nil {
			t.Fatalf("read dedup_group_id for %s: %v", id, err)
		}
		if group != nil {
			out[id] = *group
		}
	}
	return out
}

func similarBotComments() []*provider.Comment {
	path := "internal/auth/login.go"
	line10, line12 := 10, 12
	return []*provider.Comment{
		{
			ID: "c-rabbit", Author: "coderabbitai[bot]", Kind: "review",
			Body:     "Potential nil pointer dereference: config may be nil when the file is missing.",
			FilePath: path, LineNo: line10,
		},
		{
			ID: "c-greptile", Author: "greptile-apps[bot]", Kind: "review",
			Body:     "Potential nil pointer dereference — config may be nil when file is missing.",
			FilePath: path, LineNo: line12,
		},
	}
}

func TestWriterDedupGroupsSimilarBotComments(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db).WithClassifier(newClassifierProvider(t))

	if err := w.WriteSyncResults(classifyTestResult(similarBotComments()...)); err != nil {
		t.Fatalf("sync: %v", err)
	}

	groups := dedupGroupIDs(t, w, "c-rabbit", "c-greptile")
	if len(groups) != 2 || groups["c-rabbit"] == "" || groups["c-rabbit"] != groups["c-greptile"] {
		t.Errorf("similar bot comments must share a group id: %v", groups)
	}
}

func TestWriterDedupIdempotentAcrossResync(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db).WithClassifier(newClassifierProvider(t))

	if err := w.WriteSyncResults(classifyTestResult(similarBotComments()...)); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	first := dedupGroupIDs(t, w, "c-rabbit", "c-greptile")
	if err := w.WriteSyncResults(classifyTestResult(similarBotComments()...)); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	second := dedupGroupIDs(t, w, "c-rabbit", "c-greptile")
	if first["c-rabbit"] != second["c-rabbit"] {
		t.Errorf("group id changed across re-sync: %v vs %v", first, second)
	}
}

func TestWriterDedupDissimilarStaysNull(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db).WithClassifier(newClassifierProvider(t))
	path := "main.go"
	comments := []*provider.Comment{
		{ID: "c-1", Author: "coderabbitai[bot]", Kind: "review", Body: "Nil pointer risk in the config loader.", FilePath: path, LineNo: 10},
		{ID: "c-2", Author: "greptile-apps[bot]", Kind: "review", Body: "Consider renaming this variable for clarity and readability.", FilePath: path, LineNo: 11},
	}

	if err := w.WriteSyncResults(classifyTestResult(comments...)); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if groups := dedupGroupIDs(t, w, "c-1", "c-2"); len(groups) != 0 {
		t.Errorf("dissimilar comments must stay ungrouped: %v", groups)
	}
}
