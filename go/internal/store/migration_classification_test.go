package store

import "testing"

func TestMigration027CommentClassification(t *testing.T) {
	db := newTestDB(t) // reuse the existing test helper that runs all migrations

	// New columns exist on comments.
	for _, col := range []string{"author_type", "bot_vendor", "severity", "classification_source"} {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name = ?`, col,
		).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("comments.%s missing (err=%v count=%d)", col, err, count)
		}
	}

	// State side-table exists with the right shape.
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('comment_classification_state') WHERE name IN ('comment_id','fingerprint')`,
	).Scan(&n); err != nil || n != 2 {
		t.Errorf("comment_classification_state shape wrong (err=%v n=%d)", err, n)
	}
}

func TestMigration028CommentDismissals(t *testing.T) {
	db := newTestDB(t) // reuse the existing test helper that runs all migrations

	t.Run("schema has per-account composite primary key", func(t *testing.T) {
		columns := []struct {
			name string
			pk   int
		}{
			{name: "comment_id", pk: 1},
			{name: "account_provider", pk: 2},
			{name: "account_login", pk: 3},
			{name: "dismissed_at", pk: 0},
		}

		for _, column := range columns {
			var count int
			err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('comment_dismissals') WHERE name = ? AND pk = ?`,
				column.name,
				column.pk,
			).Scan(&count)
			if err != nil || count != 1 {
				t.Errorf("comment_dismissals.%s pk=%d missing (err=%v count=%d)", column.name, column.pk, err, count)
			}
		}
	})

	t.Run("duplicate triples fail and comment delete cascades", func(t *testing.T) {
		mustInsertPR(t, db, "pr-dismissals")
		mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at)
			VALUES ('comment-dismissed','pr-dismissals','alice','please fix','2026-01-01','2026-01-01')`)
		mustExec(t, db, `INSERT INTO comment_dismissals (comment_id, account_provider, account_login)
			VALUES ('comment-dismissed','github','alice')`)

		if _, err := db.Exec(`INSERT INTO comment_dismissals (comment_id, account_provider, account_login)
			VALUES ('comment-dismissed','github','alice')`); err == nil {
			t.Fatal("expected duplicate comment dismissal triple to fail")
		}

		mustExec(t, db, `DELETE FROM comments WHERE id = 'comment-dismissed'`)

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM comment_dismissals WHERE comment_id = 'comment-dismissed'`).Scan(&count); err != nil {
			t.Fatalf("count cascaded dismissals: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected comment dismissal to cascade on comment delete, got %d rows", count)
		}
	})
}
