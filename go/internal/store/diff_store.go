package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	storedb "github.com/itterno/bistro/internal/store/db"
)

const (
	defaultFileDiffStatus   = "modified"
	defaultStoredDiffStatus = "ok"
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
	db      *sql.DB
	queries *storedb.Queries
}

// NewDiffStore creates a DiffStore backed by the given *sql.DB.
func NewDiffStore(db *sql.DB) *DiffStore {
	return &DiffStore{db: db, queries: storedb.New(db)}
}

// SaveDiffs atomically replaces all stored diffs for prID.
// It always deletes existing rows first — this removes files that were renamed
// or deleted in the current diff, and clears stale rows when diffs is empty.
// Passing nil or an empty slice clears all existing rows and is not a no-op.
func (s *DiffStore) SaveDiffs(prID string, diffs []DiffFile) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	// Always delete first: ensures removed/renamed files and empty diffs leave no orphans.
	if err := qtx.DeleteDiffsForPR(ctx, prID); err != nil {
		return fmt.Errorf("clear diffs for %s: %w", prID, err)
	}

	for _, d := range diffs {
		if err := qtx.InsertDiff(ctx, storedb.InsertDiffParams{
			PrID:        prID,
			FilePath:    d.FilePath,
			Status:      diffStatus(d.Status),
			OldPath:     nullableString(d.OldPath),
			IsBinary:    boolToInt(d.IsBinary),
			IsTruncated: boolToInt(d.IsTruncated),
			PatchBytes:  int64(d.PatchBytes),
			HunkCount:   int64(d.HunkCount),
			Patch:       d.Patch,
			Additions:   int64(d.Additions),
			Deletions:   int64(d.Deletions),
			DiffStatus:  storedDiffStatus(d.DiffStatus),
		}); err != nil {
			return fmt.Errorf("insert diff for %s/%s: %w", prID, d.FilePath, err)
		}
	}

	return tx.Commit()
}

// GetDiffs returns all stored DiffFile entries for the given PR, ordered by file path.
// Returns an empty slice (not an error) when no diffs exist.
func (s *DiffStore) GetDiffs(prID string) ([]DiffFile, error) {
	rows, err := s.queries.ListDiffsForPR(context.Background(), prID)
	if err != nil {
		return nil, fmt.Errorf("query diffs for %s: %w", prID, err)
	}

	diffs := make([]DiffFile, 0, len(rows))
	for _, row := range rows {
		diffs = append(diffs, diffFileFromRow(row))
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
		return defaultFileDiffStatus
	}
	return status
}

func storedDiffStatus(status string) string {
	if status == "" {
		return defaultStoredDiffStatus
	}
	return status
}

func diffFileFromRow(row storedb.ListDiffsForPRRow) DiffFile {
	return DiffFile{
		FilePath:    row.FilePath,
		Status:      row.Status,
		OldPath:     row.OldPath,
		IsBinary:    row.IsBinary != 0,
		IsTruncated: row.IsTruncated != 0,
		PatchBytes:  int(row.PatchBytes),
		HunkCount:   int(row.HunkCount),
		Patch:       row.Patch,
		Additions:   int(row.Additions),
		Deletions:   int(row.Deletions),
		DiffStatus:  row.DiffStatus,
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
