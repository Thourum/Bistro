package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// DiffFile represents a single file's diff entry for storage.
type DiffFile struct {
	FilePath    string
	Status      string
	OldPath     string
	IsBinary    bool
	IsTruncated bool
	PatchBytes  int
	HunkCount   int
	Patch       string
	Additions   int
	Deletions   int
	DiffStatus  string // "ok", "truncated", "too_large"; empty string treated as "ok"
}

// DiffStore provides persistence for per-file PR diffs in the pr_diffs table.
type DiffStore struct {
	db *sql.DB
}

// NewDiffStore creates a DiffStore backed by the given *sql.DB.
func NewDiffStore(db *sql.DB) *DiffStore {
	return &DiffStore{db: db}
}

// SaveDiffs atomically replaces all stored diffs for prID.
// It always deletes existing rows first — this removes files that were renamed
// or deleted in the current diff, and clears stale rows when diffs is empty.
// Passing nil or an empty slice clears all existing rows and is not a no-op.
func (s *DiffStore) SaveDiffs(prID string, diffs []DiffFile) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Always delete first: ensures removed/renamed files and empty diffs leave no orphans.
	if _, err := tx.Exec(`DELETE FROM pr_diffs WHERE pr_id = ?`, prID); err != nil {
		return fmt.Errorf("clear diffs for %s: %w", prID, err)
	}

	if len(diffs) == 0 {
		return tx.Commit()
	}

	const query = `
		INSERT INTO pr_diffs (
			pr_id, file_path, status, old_path, is_binary, is_truncated,
			patch_bytes, hunk_count, patch, additions, deletions, diff_status
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, d := range diffs {
		if _, err := stmt.Exec(
			prID, d.FilePath, diffStatus(d.Status), nullableString(d.OldPath), boolToInt(d.IsBinary), boolToInt(d.IsTruncated),
			d.PatchBytes, d.HunkCount, d.Patch, d.Additions, d.Deletions, storedDiffStatus(d.DiffStatus),
		); err != nil {
			return fmt.Errorf("insert diff for %s/%s: %w", prID, d.FilePath, err)
		}
	}

	return tx.Commit()
}

// GetDiffs returns all stored DiffFile entries for the given PR, ordered by file path.
// Returns an empty slice (not an error) when no diffs exist.
func (s *DiffStore) GetDiffs(prID string) ([]DiffFile, error) {
	const query = `
		SELECT file_path, status, COALESCE(old_path, ''), is_binary, is_truncated,
		       patch_bytes, hunk_count, patch, additions, deletions, diff_status
		FROM pr_diffs
		WHERE pr_id = ?
		ORDER BY file_path
	`

	rows, err := s.db.Query(query, prID)
	if err != nil {
		return nil, fmt.Errorf("query diffs for %s: %w", prID, err)
	}
	defer rows.Close()

	diffs := make([]DiffFile, 0)
	for rows.Next() {
		var d DiffFile
		var isBinary, isTruncated int
		if err := rows.Scan(
			&d.FilePath, &d.Status, &d.OldPath, &isBinary, &isTruncated,
			&d.PatchBytes, &d.HunkCount, &d.Patch, &d.Additions, &d.Deletions, &d.DiffStatus,
		); err != nil {
			return nil, fmt.Errorf("scan diff row: %w", err)
		}
		d.IsBinary = isBinary != 0
		d.IsTruncated = isTruncated != 0
		diffs = append(diffs, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate diff rows: %w", err)
	}

	return diffs, nil
}

// GetUnifiedDiff assembles all per-file patches for the given PR into a single
// unified diff string suitable for display. Returns an error when no diffs are stored.
func (s *DiffStore) GetUnifiedDiff(prID string) (string, error) {
	diffs, err := s.GetDiffs(prID)
	if err != nil {
		return "", err
	}

	if len(diffs) == 0 {
		return "", fmt.Errorf("no diffs found for PR %s", prID)
	}

	var b strings.Builder
	for _, d := range diffs {
		// Emit a git diff header so consumers can identify file boundaries.
		oldPath := d.FilePath
		if d.OldPath != "" {
			oldPath = d.OldPath
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldPath, d.FilePath)
		b.WriteString(d.Patch)
		// Ensure each file patch ends with a newline before the next header.
		if len(d.Patch) > 0 && d.Patch[len(d.Patch)-1] != '\n' {
			b.WriteByte('\n')
		}
	}

	return b.String(), nil
}

func diffStatus(status string) string {
	if status == "" {
		return "modified"
	}
	return status
}

func storedDiffStatus(status string) string {
	if status == "" {
		return "ok"
	}
	return status
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
