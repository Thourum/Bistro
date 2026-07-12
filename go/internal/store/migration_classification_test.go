package store

import "testing"

func TestMigration027CommentDismissals(t *testing.T) {
	db := openTestDBForQueue(t) // reuse the existing test helper that runs all migrations

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
