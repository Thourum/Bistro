package prsync

import (
	"context"
	"database/sql"
	"testing"

	"github.com/itterno/bistro/internal/store"
)

// ── helpers ───────────────────────────────────────────────────────────────

func mustExecSync(t *testing.T, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("mustExecSync %q: %v", q, err)
	}
}

func insertPRForSync(t *testing.T, db *sql.DB, prID string) {
	t.Helper()
	num := 0
	for _, c := range prID {
		num = num*31 + int(c)
	}
	if num < 0 {
		num = -num
	}
	num = (num%100000 + 1)
	mustExecSync(t, db,
		`INSERT OR IGNORE INTO prs
		 (id, provider, owner, repo, number, title, author, state,
		  created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		 VALUES (?, 'github', 'o', 'r', ?, 'T', 'a', 'open',
		         '2026-01-01', '2026-01-01', 'main', 'f', 'base', 'head')`,
		prID, num,
	)
}

func insertThreadSync(t *testing.T, db *sql.DB, prID, threadID string, resolved, stateKnown bool) {
	t.Helper()
	insertPRForSync(t, db, prID)
	r, sk := 0, 0
	if resolved {
		r = 1
	}
	if stateKnown {
		sk = 1
	}
	mustExecSync(t, db,
		`INSERT OR IGNORE INTO review_threads (id, pr_id, resolved, outdated, state_known) VALUES (?, ?, ?, 0, ?)`,
		threadID, prID, r, sk,
	)
}

func prepareReplyThreadFixture(t *testing.T, db *store.DB, prID, threadID, commentID string) {
	t.Helper()
	insertPRForSync(t, db.DB, prID)
	mustExecSync(t, db.DB, `INSERT INTO review_threads (id, pr_id, resolved, state_known) VALUES (?, ?, 0, 1)`, threadID, prID)
	mustExecSync(t, db.DB, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind, thread_id)
		VALUES (?, ?, 'a', 'root', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'review', ?)`, commentID, prID, threadID)
}
