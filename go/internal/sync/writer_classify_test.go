package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/provider"
)

const classifyTestRules = `name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]"]
classifiers:
  - name: nit_prefix
    body_pattern: '^nit:'
    severity: nit
`

func newClassifierProvider(t *testing.T) *classify.EngineProvider {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coderabbit.yml"), []byte(classifyTestRules), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	return classify.NewEngineProvider(dir, func() string { return "rules" }, nil)
}

func classificationRow(t *testing.T, w *Writer, commentID string) (authorType, severity, source, fingerprint string) {
	t.Helper()
	var at, sev, src, fp *string
	err := w.db.QueryRow(`
		SELECT c.author_type, c.severity, c.classification_source, s.fingerprint
		FROM comments c
		LEFT JOIN comment_classification_state s ON s.comment_id = c.id
		WHERE c.id = ?`, commentID,
	).Scan(&at, &sev, &src, &fp)
	if err != nil {
		t.Fatalf("read classification row for %s: %v", commentID, err)
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return deref(at), deref(sev), deref(src), deref(fp)
}

func classifyTestResult(comments ...*provider.Comment) []*PRFetchResult {
	now := time.Now()
	for _, c := range comments {
		c.CreatedAt, c.UpdatedAt = now, now
	}
	return []*PRFetchResult{{
		PR: &provider.PR{
			ID: "github:org/repo:1", Number: 1, Title: "t", State: "open",
			HeadSHA: "h", BaseSHA: "b",
		},
		Comments: comments,
	}}
}

func TestWriterClassifiesComments(t *testing.T) {
	db := openTestDB(t)
	p := newClassifierProvider(t)
	w := NewWriter(nil, db).WithClassifier(p)

	err := w.WriteSyncResults(classifyTestResult(
		&provider.Comment{ID: "c-bot", Author: "coderabbitai[bot]", Body: "nit: rename this"},
		&provider.Comment{ID: "c-human", Author: "alice", Body: "looks good"},
	))
	if err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	at, sev, src, fp := classificationRow(t, w, "c-bot")
	if at != "bot" || sev != "nit" || src != "rule:coderabbit.yml:nit_prefix" {
		t.Errorf("bot comment: author_type=%q severity=%q source=%q", at, sev, src)
	}
	if fp != p.Current().Fingerprint() {
		t.Errorf("fingerprint = %q, want engine fingerprint %q", fp, p.Current().Fingerprint())
	}

	at, sev, src, _ = classificationRow(t, w, "c-human")
	if at != "human" || sev != "" || src != "unclassified" {
		t.Errorf("human comment: author_type=%q severity=%q source=%q", at, sev, src)
	}
}

func TestWriterNilEngineSkipsClassification(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(nil, db) // no classifier

	err := w.WriteSyncResults(classifyTestResult(
		&provider.Comment{ID: "c-1", Author: "coderabbitai[bot]", Body: "nit: rename"},
	))
	if err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	at, sev, src, fp := classificationRow(t, w, "c-1")
	if at != "" || sev != "" || src != "" || fp != "" {
		t.Errorf("nil engine must leave columns NULL: author_type=%q severity=%q source=%q fp=%q", at, sev, src, fp)
	}
}

func TestWriterNoMatchPersistsUnclassified(t *testing.T) {
	db := openTestDB(t)
	w := NewWriter(nil, db).WithClassifier(newClassifierProvider(t))

	err := w.WriteSyncResults(classifyTestResult(
		&provider.Comment{ID: "c-1", Author: "random-thing[bot]", Body: "no rule matches this"},
	))
	if err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	at, sev, src, _ := classificationRow(t, w, "c-1")
	if at != "bot" || sev != "" || src != "unclassified" {
		t.Errorf("unmatched bot: author_type=%q severity=%q source=%q", at, sev, src)
	}
}

func TestWriterReclassifiesOnResync(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	rulePath := filepath.Join(dir, "coderabbit.yml")
	if err := os.WriteFile(rulePath, []byte(classifyTestRules), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	p := classify.NewEngineProvider(dir, func() string { return "rules" }, nil)
	w := NewWriter(nil, db).WithClassifier(p)

	comment := &provider.Comment{ID: "c-1", Author: "coderabbitai[bot]", Body: "nit: rename"}
	if err := w.WriteSyncResults(classifyTestResult(comment)); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Rule edit: nits become warnings. Re-sync must re-classify.
	edited := []byte("name: CodeRabbit\nvendor: coderabbit\nmatch:\n  author: [\"coderabbitai[bot]\"]\nclassifiers:\n  - name: nit_prefix\n    body_pattern: '^nit:'\n    severity: warning\n")
	if err := os.WriteFile(rulePath, edited, 0o644); err != nil {
		t.Fatalf("edit rules: %v", err)
	}
	if err := w.WriteSyncResults(classifyTestResult(comment)); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	_, sev, _, fp := classificationRow(t, w, "c-1")
	if sev != "warning" {
		t.Errorf("re-sync severity = %q, want warning", sev)
	}
	if fp != p.Current().Fingerprint() {
		t.Errorf("fingerprint not updated on re-sync")
	}
}
