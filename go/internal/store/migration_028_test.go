package store

import "testing"

func TestMigration028DedupAndLLMCache(t *testing.T) {
	db := openTestDBForQueue(t) // reuse the existing test helper that runs all migrations

	var dedupColumnCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name = 'dedup_group_id'`,
	).Scan(&dedupColumnCount); err != nil || dedupColumnCount != 1 {
		t.Fatalf("comments.dedup_group_id missing (err=%v count=%d)", err, dedupColumnCount)
	}

	columns := map[string]string{
		"body_hash":  "TEXT",
		"severity":   "TEXT",
		"category":   "TEXT",
		"provider":   "TEXT",
		"model":      "TEXT",
		"created_at": "TIMESTAMP",
	}
	for name, wantType := range columns {
		var gotType string
		var count int
		err := db.QueryRow(
			`SELECT type, COUNT(*) FROM pragma_table_info('llm_classification_cache') WHERE name = ? GROUP BY type`,
			name,
		).Scan(&gotType, &count)
		if err != nil || count != 1 || gotType != wantType {
			t.Errorf("llm_classification_cache.%s shape wrong (err=%v count=%d type=%q want=%q)", name, err, count, gotType, wantType)
		}
	}

	var primaryKeyCount int
	if err := db.QueryRow(
		`SELECT pk FROM pragma_table_info('llm_classification_cache') WHERE name = 'body_hash'`,
	).Scan(&primaryKeyCount); err != nil || primaryKeyCount != 1 {
		t.Fatalf("llm_classification_cache.body_hash primary key wrong (err=%v pk=%d)", err, primaryKeyCount)
	}

	var indexCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 'llm_classification_cache' AND name NOT LIKE 'sqlite_autoindex_%'`,
	).Scan(&indexCount); err != nil || indexCount != 0 {
		t.Fatalf("unexpected explicit llm_classification_cache indexes (err=%v count=%d)", err, indexCount)
	}
}
