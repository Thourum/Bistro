package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigration036FoldsRepoIDCase(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	migrationFS, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		t.Fatalf("sub fs: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migrationFS, goose.WithLogger(goose.NopLogger()))
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 35); err != nil {
		t.Fatalf("migrate to 35: %v", err)
	}

	seed := []string{
		// Repo-list row: GitHub casing, watched, no location.
		`INSERT INTO repos (id, provider, owner, repo, watched) VALUES ('github:Thourum/testing', 'github', 'Thourum', 'testing', 1)`,
		// Location row: lowercase, degraded cache clone.
		`INSERT INTO repos (id, provider, owner, repo, watched, local_path, source, location_status, location_error_code)
		 VALUES ('github:thourum/testing', 'github', 'thourum', 'testing', 0, '/cache/t', 'cache', 'degraded', 'fetch_failed')`,
		// Lone mixed-case row with no twin.
		`INSERT INTO repos (id, provider, owner, repo, watched) VALUES ('github:Acme/API', 'github', 'Acme', 'API', 0)`,
		`INSERT INTO prs (id, provider, owner, repo, number, title, author, state, created_at, updated_at, base_branch, head_branch, base_sha, head_sha, repo_id)
		 VALUES ('github:Thourum/testing:1', 'github', 'Thourum', 'testing', 1, 't', 'a', 'open', '2026-01-01', '2026-01-01', 'main', 'f', 'b', 'h', 'github:Thourum/testing')`,
		`INSERT INTO prs (id, provider, owner, repo, number, title, author, state, created_at, updated_at, base_branch, head_branch, base_sha, head_sha, repo_id)
		 VALUES ('github:Thourum/testing:2', 'github', 'thourum', 'testing', 2, 'duplicate', 'a', 'open', '2026-01-01', '2026-01-01', 'main', 'f', 'b', 'h', 'github:Thourum/testing'),
		        ('github:thourum/testing:2', 'github', 'Thourum', 'testing', 2, 'canonical', 'a', 'open', '2026-01-01', '2026-01-01', 'main', 'f', 'b', 'h', 'github:thourum/testing')`,
		`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at)
		 VALUES ('comment', 'github:Thourum/testing:1', 'a', 'Keep Casing', '2026-01-01', '2026-01-01'),
		        ('duplicate-comment', 'github:Thourum/testing:2', 'a', 'duplicate', '2026-01-01', '2026-01-01'),
		        ('canonical-comment', 'github:thourum/testing:2', 'a', 'canonical', '2026-01-01', '2026-01-01')`,
		`INSERT INTO files (id, pr_id, file_path, status) VALUES ('file', 'github:Thourum/testing:1', 'README.md', 'modified')`,
		`INSERT INTO pr_diffs (pr_id, file_path, patch) VALUES ('github:Thourum/testing:1', 'README.md', 'Keep Casing')`,
		`INSERT INTO review_threads (id, pr_id) VALUES ('thread', 'github:Thourum/testing:1')`,
		`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		 VALUES ('github:Thourum/testing:1', 1, 'h', 'b', '2026-01-01'),
		        ('github:Thourum/testing:2', 1, 'h', 'b', '2026-01-01'),
		        ('github:thourum/testing:2', 1, 'h', 'b', '2026-01-01')`,
		`INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id)
		 VALUES ('github:Thourum/testing:1', 1, 'README.md', 'fingerprint'),
		        ('github:Thourum/testing:2', 1, 'README.md', 'duplicate'),
		        ('github:thourum/testing:2', 1, 'README.md', 'canonical')`,
		`INSERT INTO last_reviewed_sha (pr_id, login, sha) VALUES ('github:Thourum/testing:1', 'Viewer', 'h')`,
		`INSERT INTO files_viewed (pr_id, file_path, login, content_hash) VALUES ('github:Thourum/testing:1', 'README.md', 'Viewer', 'hash')`,
		`INSERT INTO pr_read_state (pr_id, login, last_read_at)
		 VALUES ('github:Thourum/testing:1', 'Viewer', '2026-01-01'),
		        ('github:Thourum/testing:2', 'Viewer', '2026-01-01'),
		        ('github:thourum/testing:2', 'Viewer', '2026-01-02')`,
		`INSERT INTO action_queue (id, action_type, pr_id, payload)
		 VALUES ('resolve', 'resolve_thread', 'github:Thourum/testing:1', '{"pr_id":"github:Thourum/testing:1","thread_id":"ThreadID"}'),
		        ('comment', 'post_comment', 'github:Thourum/testing:2', 'Keep Casing'),
		        ('json-comment', 'post_comment', 'github:Thourum/testing:1', '{"pr_id":"Keep Casing"}'),
		        ('github-abc-github:Thourum/testing:1-approve', 'approve', 'github:Thourum/testing:1', '')`,
		`INSERT INTO sync_cursors (scope, cursor_value) VALUES ('github:inbox:Viewer:search:involves', 'CursorValue')`,
		`INSERT INTO expertise_repo_state (repo_id, enabled, status, nudge_dismissed_at)
		 VALUES ('github:thourum/testing', 0, 'ready', NULL), ('github:Thourum/testing', 1, 'idle', '2026-01-01')`,
		`INSERT INTO expertise_settings (key, value) VALUES ('excludes:github:Thourum/testing', '["Vendor/**"]')`,
	}
	for _, statement := range seed {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var ids []string
	rows, err := db.QueryContext(ctx, `SELECT id FROM repos ORDER BY id`)
	if err != nil {
		t.Fatalf("list repos: %v", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 || ids[0] != "github:acme/api" || ids[1] != "github:thourum/testing" {
		t.Fatalf("repo ids = %v, want [github:acme/api github:thourum/testing]", ids)
	}

	var owner, localPath, source, status string
	var watched int
	if err := db.QueryRowContext(ctx,
		`SELECT owner, watched, local_path, source, location_status FROM repos WHERE id = 'github:thourum/testing'`,
	).Scan(&owner, &watched, &localPath, &source, &status); err != nil {
		t.Fatalf("read merged repo: %v", err)
	}
	if owner != "thourum" || watched != 1 || localPath != "/cache/t" || source != "cache" || status != "degraded" {
		t.Fatalf("merged repo = owner %q watched %d path %q source %q status %q", owner, watched, localPath, source, status)
	}

	var prRepoID string
	if err := db.QueryRowContext(ctx, `SELECT repo_id FROM prs WHERE id = 'github:thourum/testing:1'`).Scan(&prRepoID); err != nil || prRepoID != "github:thourum/testing" {
		t.Fatalf("prs.repo_id = %q (err=%v)", prRepoID, err)
	}

	checks := []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM repos WHERE owner <> lower(owner) OR repo <> lower(repo)`, 0},
		{`SELECT COUNT(*) FROM prs WHERE id <> lower(id) OR owner <> lower(owner) OR repo <> lower(repo) OR repo_id <> lower(repo_id)`, 0},
		{`SELECT COUNT(*) FROM prs`, 2},
		{`SELECT COUNT(*) FROM prs WHERE id = 'github:thourum/testing:2' AND title = 'canonical'`, 1},
		{`SELECT COUNT(*) FROM comments WHERE id = 'comment' AND pr_id = 'github:thourum/testing:1' AND body = 'Keep Casing'`, 1},
		{`SELECT COUNT(*) FROM comments WHERE id = 'duplicate-comment'`, 0},
		{`SELECT COUNT(*) FROM comments WHERE id = 'canonical-comment'`, 1},
		{`SELECT COUNT(*) FROM files WHERE pr_id = 'github:thourum/testing:1' AND file_path = 'README.md'`, 1},
		{`SELECT COUNT(*) FROM pr_diffs WHERE pr_id = 'github:thourum/testing:1' AND patch = 'Keep Casing'`, 1},
		{`SELECT COUNT(*) FROM review_threads WHERE pr_id = 'github:thourum/testing:1'`, 1},
		{`SELECT COUNT(*) FROM patchsets WHERE pr_id = 'github:thourum/testing:1'`, 1},
		{`SELECT COUNT(*) FROM patchset_files WHERE pr_id = 'github:thourum/testing:1' AND path = 'README.md' AND patch_id = 'fingerprint'`, 1},
		{`SELECT COUNT(*) FROM patchset_files WHERE pr_id = 'github:thourum/testing:2' AND patch_id = 'canonical'`, 1},
		{`SELECT COUNT(*) FROM patchset_files WHERE patch_id = 'duplicate'`, 0},
		{`SELECT COUNT(*) FROM last_reviewed_sha WHERE pr_id = 'github:thourum/testing:1' AND login = 'Viewer'`, 1},
		{`SELECT COUNT(*) FROM files_viewed WHERE pr_id = 'github:thourum/testing:1' AND file_path = 'README.md' AND login = 'Viewer'`, 1},
		{`SELECT COUNT(*) FROM pr_read_state WHERE pr_id = 'github:thourum/testing:1' AND login = 'Viewer'`, 1},
		{`SELECT COUNT(*) FROM pr_read_state WHERE pr_id = 'github:thourum/testing:2' AND last_read_at = '2026-01-02'`, 1},
		{`SELECT COUNT(*) FROM action_queue WHERE id = 'resolve' AND pr_id = 'github:thourum/testing:1' AND json_extract(payload, '$.pr_id') = 'github:thourum/testing:1' AND json_extract(payload, '$.thread_id') = 'ThreadID'`, 1},
		{`SELECT COUNT(*) FROM action_queue WHERE id = 'comment' AND pr_id = 'github:thourum/testing:2' AND payload = 'Keep Casing'`, 1},
		{`SELECT COUNT(*) FROM action_queue WHERE id = 'json-comment' AND pr_id = 'github:thourum/testing:1' AND payload = '{"pr_id":"Keep Casing"}'`, 1},
		{`SELECT COUNT(*) FROM action_queue WHERE id = 'github-abc-github:thourum/testing:1-approve'`, 1},
		{`SELECT COUNT(*) FROM sync_cursors WHERE scope = 'github:inbox:Viewer:search:involves' AND cursor_value = 'CursorValue'`, 1},
		{`SELECT COUNT(*) FROM expertise_repo_state`, 1},
		{`SELECT COUNT(*) FROM expertise_repo_state WHERE repo_id = 'github:thourum/testing' AND enabled = 1 AND status = 'ready' AND nudge_dismissed_at IS NOT NULL`, 1},
		{`SELECT COUNT(*) FROM expertise_settings WHERE key = 'excludes:github:thourum/testing' AND value = '["Vendor/**"]'`, 1},
	}
	for _, check := range checks {
		var count int
		if err := db.QueryRowContext(ctx, check.query).Scan(&count); err != nil || count != check.want {
			t.Errorf("%s = %d (err=%v), want %d", check.query, count, err, check.want)
		}
	}

	foreignKeys, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("check foreign keys: %v", err)
	}
	if foreignKeys.Next() {
		t.Error("migration left a foreign key violation")
	}
	if err := foreignKeys.Err(); err != nil {
		t.Fatalf("check foreign key rows: %v", err)
	}
	foreignKeys.Close()
}
