package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// DiffFile represents a single file's diff entry for storage.
type DiffFile struct {
	FilePath  string
	Patch     string
	Additions int
	Deletions int
}

// DiffStore provides persistence for per-file PR diffs in the pr_diffs table.
type DiffStore struct {
	db *sql.DB
}

// NewDiffStore creates a DiffStore backed by the given *sql.DB.
func NewDiffStore(db *sql.DB) *DiffStore {
	return &DiffStore{db: db}
}

// SaveDiffs persists all diffs for the given PR, upserting on (pr_id, file_path).
// A nil or empty diffs slice is a no-op.
func (s *DiffStore) SaveDiffs(prID string, diffs []DiffFile) error {
	if len(diffs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	const query = `
		INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(pr_id, file_path) DO UPDATE SET
			patch      = excluded.patch,
			additions  = excluded.additions,
			deletions  = excluded.deletions
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, d := range diffs {
		if _, err := stmt.Exec(prID, d.FilePath, d.Patch, d.Additions, d.Deletions); err != nil {
			return fmt.Errorf("insert diff for %s/%s: %w", prID, d.FilePath, err)
		}
	}

	return tx.Commit()
}

// GetDiffs returns all stored DiffFile entries for the given PR, ordered by file path.
// Returns an empty slice (not an error) when no diffs exist.
func (s *DiffStore) GetDiffs(prID string) ([]DiffFile, error) {
	const query = `
		SELECT file_path, patch, additions, deletions
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
		if err := rows.Scan(&d.FilePath, &d.Patch, &d.Additions, &d.Deletions); err != nil {
			return nil, fmt.Errorf("scan diff row: %w", err)
		}
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
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", d.FilePath, d.FilePath)
		b.WriteString(d.Patch)
		// Ensure each file patch ends with a newline before the next header.
		if len(d.Patch) > 0 && d.Patch[len(d.Patch)-1] != '\n' {
			b.WriteByte('\n')
		}
	}

	return b.String(), nil
}
