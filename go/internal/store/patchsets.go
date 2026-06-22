package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Patchset is one observed patchset row, keyed by (pr_id, number).
type Patchset struct {
	PRID     string    `json:"pr_id"`
	Number   int       `json:"number"`
	HeadSHA  string    `json:"head_sha"`
	BaseSHA  string    `json:"base_sha"`
	PushedAt time.Time `json:"pushed_at"`
}

// ListPatchsets returns all observed patchsets for pr_id ordered by number ascending.
func (db *DB) ListPatchsets(ctx context.Context, prID string) ([]Patchset, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT pr_id, number, head_sha, base_sha, pushed_at
		 FROM patchsets WHERE pr_id = ? ORDER BY number ASC`,
		prID,
	)
	if err != nil {
		return nil, fmt.Errorf("list patchsets (%s): %w", prID, err)
	}
	defer rows.Close()
	var out []Patchset
	for rows.Next() {
		var p Patchset
		if err := rows.Scan(&p.PRID, &p.Number, &p.HeadSHA, &p.BaseSHA, &p.PushedAt); err != nil {
			return nil, fmt.Errorf("scan patchset: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetCommentPatchsetNumbers returns a map of commentID → patchset_number for
// the given comment IDs. Only rows where patchset_number IS NOT NULL are
// included. Missing or NULL entries are simply absent from the returned map.
// Used by the anchor resolver to recover exact patchset identity for outdated
// review comments that were originally stored with a patchset_number.
func (db *DB) GetCommentPatchsetNumbers(ctx context.Context, commentIDs []string) (map[string]int, error) {
	if len(commentIDs) == 0 {
		return map[string]int{}, nil
	}
	// Build parameterised IN clause.
	placeholders := make([]string, len(commentIDs))
	args := make([]interface{}, len(commentIDs))
	for i, id := range commentIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT id, patchset_number FROM comments
	      WHERE id IN (` + strings.Join(placeholders, ",") + `)
	        AND patchset_number IS NOT NULL`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("get comment patchset numbers: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int, len(commentIDs))
	for rows.Next() {
		var id string
		var num int
		if err := rows.Scan(&id, &num); err != nil {
			return nil, fmt.Errorf("scan comment patchset number: %w", err)
		}
		out[id] = num
	}
	return out, rows.Err()
}

