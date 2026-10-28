package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/ws"
)

const refreshTestRule = `name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]"]
classifiers:
  - name: nit_prefix
    body_pattern: '^nit:'
    severity: nit
`

func refreshRulesDir(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coderabbit.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	return dir
}

func requestComments(t *testing.T, h ws.MessageHandler, prID string) []commentRow {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"pr_id": prID})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "r1", Type: "REQUEST_COMMENTS", Data: payload})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	_, data := commentsResponse(t, resp)
	comments, ok := data["comments"].([]commentRow)
	if !ok {
		t.Fatalf("comments is %T, want []commentRow", data["comments"])
	}
	return comments
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func storedClassification(t *testing.T, db *sql.DB, id string) (severity, source, fingerprint string) {
	t.Helper()
	var sev, src, fp *string
	err := db.QueryRow(`
		SELECT c.severity, c.classification_source, s.fingerprint
		FROM comments c LEFT JOIN comment_classification_state s ON s.comment_id = c.id
		WHERE c.id = ?`, id).Scan(&sev, &src, &fp)
	if err != nil {
		t.Fatalf("read stored classification: %v", err)
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return deref(sev), deref(src), deref(fp)
}

func TestCommentsReadyIncludesClassification(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	insertTestComment(t, db, "c1", "pr-1", "coderabbitai[bot]", "nit: rename", nil, nil)
	if _, err := db.Exec(`UPDATE comments SET author_type='bot', bot_vendor='coderabbit', severity='nit', classification_source='rule:coderabbit.yml:nit_prefix' WHERE id='c1'`); err != nil {
		t.Fatalf("seed classification: %v", err)
	}

	comments := requestComments(t, NewRequestCommentsHandler(db, testRepoLogger()), "pr-1")

	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	row := comments[0]
	if strDeref(row.AuthorType) != "bot" || strDeref(row.Severity) != "nit" || strDeref(row.ClassificationSource) != "rule:coderabbit.yml:nit_prefix" {
		t.Errorf("classification fields not served: %+v", row)
	}
}

func TestRefreshIfStaleReclassifies(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	insertTestComment(t, db, "c1", "pr-1", "coderabbitai[bot]", "nit: rename", nil, nil)
	// Stored under an old fingerprint with an outdated severity.
	if _, err := db.Exec(`UPDATE comments SET author_type='bot', severity='warning', classification_source='rule:old.yml:x' WHERE id='c1'`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES ('c1', 'old')`); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	provider := classify.NewEngineProvider(refreshRulesDir(t, refreshTestRule), func() string { return "rules" }, nil)
	h := NewRequestCommentsHandlerWithClassifier(db, testRepoLogger(), provider)

	comments := requestComments(t, h, "pr-1")

	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	sev, src, fp := storedClassification(t, db, "c1")
	if sev != "nit" || src != "rule:coderabbit.yml:nit_prefix" {
		t.Errorf("not re-classified: severity=%q source=%q", sev, src)
	}
	if fp != provider.Current().Fingerprint() {
		t.Errorf("fingerprint not updated: %q", fp)
	}
	if strDeref(comments[0].Severity) != "nit" {
		t.Errorf("served row not refreshed: %+v", comments[0])
	}
}

func TestRefreshIfStaleNoopWhenCurrent(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	provider := classify.NewEngineProvider(refreshRulesDir(t, refreshTestRule), func() string { return "rules" }, nil)
	insertTestComment(t, db, "c1", "pr-1", "coderabbitai[bot]", "nit: rename", nil, nil)
	// Simulate a row the writer classified with a bogus source but the
	// CURRENT fingerprint — a matching fingerprint must never be touched.
	if _, err := db.Exec(`UPDATE comments SET author_type='bot', severity='warning', classification_source='rule:sentinel.yml:x' WHERE id='c1'`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES ('c1', ?)`, provider.Current().Fingerprint()); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	h := NewRequestCommentsHandlerWithClassifier(db, testRepoLogger(), provider)

	requestComments(t, h, "pr-1")

	_, src, _ := storedClassification(t, db, "c1")
	if src != "rule:sentinel.yml:x" {
		t.Errorf("current-fingerprint row was rewritten: source=%q", src)
	}
}

func TestRefreshPicksUpRuleEditWithoutRestart(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	dir := refreshRulesDir(t, refreshTestRule)
	provider := classify.NewEngineProvider(dir, func() string { return "rules" }, nil)
	h := NewRequestCommentsHandlerWithClassifier(db, testRepoLogger(), provider)
	insertTestComment(t, db, "c1", "pr-1", "coderabbitai[bot]", "nit: rename", nil, nil)

	// First serve classifies under the original rule.
	requestComments(t, h, "pr-1")
	sev, _, _ := storedClassification(t, db, "c1")
	if sev != "nit" {
		t.Fatalf("first serve: severity=%q, want nit", sev)
	}

	// Edit the YAML on disk — same handler, no restart.
	edited := []byte("name: CodeRabbit\nvendor: coderabbit\nmatch:\n  author: [\"coderabbitai[bot]\"]\nclassifiers:\n  - name: nit_prefix\n    body_pattern: '^nit:'\n    severity: suggestion\n")
	if err := os.WriteFile(filepath.Join(dir, "coderabbit.yml"), edited, 0o644); err != nil {
		t.Fatalf("edit rule: %v", err)
	}

	comments := requestComments(t, h, "pr-1")

	sev, src, _ := storedClassification(t, db, "c1")
	if sev != "suggestion" || src != "rule:coderabbit.yml:nit_prefix" {
		t.Errorf("rule edit not applied: severity=%q source=%q", sev, src)
	}
	if len(comments) != 1 || strDeref(comments[0].Severity) != "suggestion" {
		t.Errorf("served row stale after edit: %+v", comments)
	}
}

func TestRequestCommentsKicksLLMTier(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	insertTestComment(t, db, "c1", "pr-1", "alice", "hi", nil, nil)
	var kicked []string
	h := NewRequestCommentsHandlerWithLLMKick(db, testRepoLogger(), nil, func(prID string) {
		kicked = append(kicked, prID)
	})

	requestComments(t, h, "pr-1")

	if len(kicked) != 1 || kicked[0] != "pr-1" {
		t.Errorf("kick = %v, want [pr-1]", kicked)
	}
}

func TestRequestCommentsNilKickIsFine(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	insertTestComment(t, db, "c1", "pr-1", "alice", "hi", nil, nil)

	comments := requestComments(t, NewRequestCommentsHandler(db, testRepoLogger()), "pr-1")

	if len(comments) != 1 {
		t.Errorf("got %d comments, want 1", len(comments))
	}
}
