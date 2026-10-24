package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

func (w *Writer) dedupPRComments(ctx context.Context, tx *sql.Tx, prID string) error {
	inputs, err := readDedupInputs(ctx, tx, prID)
	if err != nil {
		return err
	}
	groups := classify.Dedup(inputs)
	for _, input := range inputs {
		groupID, ok := groups[input.ID]
		if ok {
			_, err = tx.ExecContext(ctx,
				`UPDATE comments SET dedup_group_id = ? WHERE id = ? AND (dedup_group_id IS NULL OR dedup_group_id != ?)`,
				groupID, input.ID, groupID)
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE comments SET dedup_group_id = NULL WHERE id = ? AND dedup_group_id IS NOT NULL`, input.ID)
		}
		if err != nil {
			return fmt.Errorf("dedup update %s: %w", input.ID, err)
		}
	}
	return nil
}

func readDedupInputs(ctx context.Context, tx *sql.Tx, prID string) (inputs []classify.DedupInput, readErr error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, COALESCE(author_type, ''), file_path, anchored_line, line_number, body
		 FROM comments WHERE pr_id = ?`, prID)
	if err != nil {
		return nil, fmt.Errorf("dedup select: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			readErr = errors.Join(readErr, fmt.Errorf("close dedup rows: %w", err))
		}
	}()
	for rows.Next() {
		var input classify.DedupInput
		var lineNumber *int
		if err := rows.Scan(&input.ID, &input.AuthorType, &input.FilePath, &input.AnchoredLine, &lineNumber, &input.Body); err != nil {
			return nil, fmt.Errorf("dedup scan: %w", err)
		}
		if lineNumber != nil && *lineNumber > 0 {
			input.LineNumber = lineNumber // Provider line zero means no line.
		}
		inputs = append(inputs, input)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dedup iterate: %w", err)
	}
	return inputs, nil
}

type reviewThreadWrite struct {
	threads        []*provider.ReviewThread
	patchsetNumber int
	engine         *classify.Engine
}

// Thread and comment identifiers come from the provider; preserve that
// association without modifying caller-owned comment objects.
func (w *Writer) writeReviewThreads(ctx context.Context, tx *sql.Tx, write reviewThreadWrite) {
	for _, thread := range write.threads {
		if thread == nil || thread.ID == "" {
			w.log.Warn("skipping review thread with empty ID")
			continue
		}
		if thread.StateKnown {
			w.deletePromotedProvisionalThreads(ctx, tx, thread)
		}
		if err := writeReviewThreadRow(ctx, tx, thread); err != nil {
			w.log.Error("write review_thread failed", "thread_id", thread.ID, "error", err)
			continue // No thread row for its comments to reference.
		}
		w.writeThreadComments(ctx, tx, thread, write)
	}
}

func (w *Writer) writeThreadComments(ctx context.Context, tx *sql.Tx, thread *provider.ReviewThread, write reviewThreadWrite) {
	for _, comment := range thread.Comments {
		if comment == nil {
			continue
		}
		commentCopy := *comment
		if commentCopy.ThreadID == "" {
			commentCopy.ThreadID = thread.ID
		}
		if err := w.writeComment(ctx, tx, commentWrite{
			prID: commentCopy.PRID, comment: &commentCopy,
			patchsetNumber: commentPatchsetNumber(&commentCopy, write.patchsetNumber), engine: write.engine,
		}); err != nil {
			w.log.Error("write review comment failed", "comment_id", comment.ID,
				"thread_id", thread.ID, "error", err)
		}
	}
}

func writeReviewThreadRow(ctx context.Context, tx *sql.Tx, thread *provider.ReviewThread) error {
	const query = `
		INSERT INTO review_threads (id, pr_id, file_path, line, resolved, outdated, state_known)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			resolved    = excluded.resolved,
			outdated    = excluded.outdated,
			state_known = excluded.state_known,
			file_path   = excluded.file_path,
			line        = excluded.line`
	if _, err := tx.ExecContext(ctx, query, thread.ID, thread.PRID,
		nullableString(thread.FilePath), nullableInt(thread.Line),
		thread.Resolved, thread.Outdated, thread.StateKnown,
	); err != nil {
		return fmt.Errorf("write review thread %s: %w", thread.ID, err)
	}
	return nil
}

// Deleting stale REST-only threads leaves child comments detached until they
// are upserted under the provider's canonical thread ID in this transaction.
func (w *Writer) deletePromotedProvisionalThreads(ctx context.Context, tx *sql.Tx, thread *provider.ReviewThread) {
	for _, provisionalID := range provisionalThreadIDs(thread) {
		if provisionalID == "" || provisionalID == thread.ID {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM review_threads WHERE id = ? AND state_known = 0`, provisionalID,
		); err != nil {
			w.log.Warn("delete promoted provisional review_thread failed",
				"provisional_id", provisionalID, "canonical_id", thread.ID, "error", err)
		}
	}
}

// provisionalThreadIDs mirrors the REST reply-chain fallback: each root
// comment starts a provisional thread, and replies inherit the root.
func provisionalThreadIDs(thread *provider.ReviewThread) []string {
	byID := make(map[string]*provider.Comment, len(thread.Comments))
	for _, comment := range thread.Comments {
		if comment != nil && comment.ID != "" {
			byID[comment.ID] = comment
		}
	}

	memo := make(map[string]string, len(byID))
	visiting := make(map[string]bool, len(byID))
	var rootOf func(id string) string
	rootOf = func(id string) string {
		if root, ok := memo[id]; ok {
			return root
		}
		if visiting[id] {
			memo[id] = id // A cycle cannot inherit another root.
			return id
		}
		comment, ok := byID[id]
		if !ok || comment.InReplyTo == "" {
			memo[id] = id
			return id
		}
		visiting[id] = true
		root := rootOf(comment.InReplyTo)
		visiting[id] = false
		memo[id] = root
		return root
	}

	seen := make(map[string]struct{}, len(thread.Comments))
	ids := make([]string, 0, 1)
	for _, comment := range thread.Comments {
		if comment == nil || comment.ID == "" {
			continue
		}
		root := rootOf(comment.ID)
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		ids = append(ids, root)
	}
	return ids
}

// writeDiffs replaces a PR's diff cache in the same savepoint as the PR row and
// patchset, preserving the old cache if any critical insert fails.
func (w *Writer) writeDiffs(ctx context.Context, tx *sql.Tx, prID string, diffs []store.DiffFile) (writeErr error) {
	if _, err := tx.ExecContext(ctx, `DELETE FROM pr_diffs WHERE pr_id = ?`, prID); err != nil {
		return fmt.Errorf("clear pr_diffs for %s: %w", prID, err)
	}
	if len(diffs) == 0 {
		return nil // An empty diff clears stale cached files.
	}
	const query = `
		INSERT INTO pr_diffs (
			pr_id, file_path, status, old_path, is_binary, is_truncated,
			patch_bytes, hunk_count, patch, additions, deletions, diff_status, patch_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare pr_diffs insert for %s: %w", prID, err)
	}
	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			// A failed statement close must not advance an incompletely written
			// diff cache; the caller rolls back this PR's savepoint.
			writeErr = errors.Join(writeErr, fmt.Errorf("close pr_diffs insert for %s: %w", prID, closeErr))
		}
	}()
	for _, diff := range diffs {
		if _, err := stmt.ExecContext(ctx, prID, diff.FilePath, diffStatus(diff.Status),
			nullableString(diff.OldPath), diff.IsBinary, diff.IsTruncated,
			diff.PatchBytes, diff.HunkCount, diff.Patch, diff.Additions, diff.Deletions,
			storedDiffStatus(diff.DiffStatus), diff.PatchID); err != nil {
			return fmt.Errorf("insert pr_diff %s/%s: %w", prID, diff.FilePath, err)
		}
	}
	return nil
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

// writeAnchorUpdates runs after releasing the PR savepoint: anchor failures
// cannot roll back the PR row, patchset, or diff. Each comment is best-effort.
func (w *Writer) writeAnchorUpdates(ctx context.Context, tx *sql.Tx, updates []git.AnchorResult, currentPatchsetNum int) {
	const query = `
		UPDATE comments
		SET anchored_line                  = ?,
		    anchor_head_sha                = ?,
		    outdated                       = ?,
		    outdated_near_line             = ?,
		    anchor_patchset_number         = ?,
		    anchor_current_patchset_number = ?
		WHERE id = ?`
	for _, update := range updates {
		var anchoredLine any
		if update.Anchored && update.AnchoredLine > 0 {
			anchoredLine = update.AnchoredLine
		}
		var nearLine any
		if update.Outdated && update.OutdatedNearLine > 0 {
			nearLine = update.OutdatedNearLine
		}
		// Source and current patchset differ for rebased comments. A missing
		// number from fetch is filled with the patchset recorded in this batch.
		sourceNumber := update.PatchsetNumber
		if sourceNumber == 0 {
			sourceNumber = currentPatchsetNum
		}
		currentNumber := update.CurrentPatchsetNumber
		if currentNumber == 0 {
			currentNumber = currentPatchsetNum
		}
		if _, err := tx.ExecContext(ctx, query, anchoredLine, update.HeadSHA,
			update.Outdated, nearLine, nullableInt(sourceNumber),
			nullableInt(currentNumber), update.CommentID,
		); err != nil {
			w.log.Warn("writeAnchorUpdates: failed to update anchor", "comment_id", update.CommentID, "error", err)
		}
	}
}
