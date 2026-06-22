package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"log/slog"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// Writer handles database writes for sync results.
type Writer struct {
	log *slog.Logger
	db  *store.DB
}

// NewWriter creates a new database writer. If log is nil, slog.Default() is
// used so the writer can always log unconditionally.
func NewWriter(log *slog.Logger, db *store.DB) *Writer {
	if log == nil {
		log = slog.Default()
	}
	return &Writer{
		log: log,
		db:  db,
	}
}

// WriteSyncResults persists all sync results to the database in a single transaction.
// Per-item errors are logged and skipped; they do not abort the transaction.
func (w *Writer) WriteSyncResults(results []*PRFetchResult) error {
	return w.WriteSyncResultsContext(context.Background(), results)
}

// WriteSyncResultsContext persists all sync results to the database in a single transaction,
// respecting the provided context for cancellation.
//
// Atomicity strategy — per-PR savepoints:
//
//	For each accepted PR result a SQLite SAVEPOINT guards three critical writes
//	(PR row, patchset, diffs). If any critical write fails the savepoint is
//	rolled back so the PR's partial changes never reach the outer transaction.
//	Other PRs in the batch are unaffected. Comments and review threads remain
//	best-effort (per-item skip with no savepoint rollback).
func (w *Writer) WriteSyncResultsContext(ctx context.Context, results []*PRFetchResult) error {
	// Prefer the request-scoped logger from ctx (carries sync_id, repo, etc.)
	// over the writer's own logger; fall back to the writer's logger when ctx
	// doesn't carry one.
	log := logging.FromContext(ctx)
	if log == slog.Default() && w.log != nil {
		log = w.log
	}

	if len(results) == 0 {
		log.Debug("WriteSyncResults: nothing to write")
		return nil
	}

	if w.db == nil {
		log.Debug("WriteSyncResults: no DB configured, skipping", "results", len(results))
		return nil
	}

	overallStart := time.Now()

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		log.Error("failed to start transaction", "error", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	prCount := 0
	prSkipped := 0
	prErrors := 0
	commentCount := 0
	diffCount := 0

	// savepoint is the per-PR savepoint name reused each iteration.
	// It is always RELEASED (or ROLLBACK+RELEASE) before the next iteration.
	const savepoint = "sp_pr"

	for _, result := range results {
		if result == nil {
			prSkipped++
			continue
		}

		if result.Error != nil {
			prErrors++
			prID := ""
			if result.PR != nil {
				prID = result.PR.ID
			}
			log.Warn("skipping pr write due to upstream error",
				"pr_id", prID,
				"error", result.Error,
			)
			continue
		}

		if result.PR == nil {
			prSkipped++
			log.Debug("skipping nil PR in result")
			continue
		}

		prID := result.PR.ID

		// ── Open per-PR savepoint ──────────────────────────────────────────
		if _, spErr := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); spErr != nil {
			// Savepoint creation failure is extremely rare (e.g. nested limit).
			log.Error("could not create savepoint, skipping PR",
				"pr_id", prID, "error", spErr)
			prErrors++
			continue
		}

		// rollbackSP rolls back and releases the savepoint, counting a PR error.
		// Returns true so callers can: if rollbackSP(...) { continue }.
		rollbackSP := func(msg string, err error) bool {
			log.Error(msg, "pr_id", prID, "error", err)
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint)
			_, _ = tx.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint)
			prErrors++
			return true
		}

		writeStart := time.Now()

		// ── Critical write 1: PR row ───────────────────────────────────────
		if err := w.writePRRow(ctx, tx, result.PR); err != nil {
			if rollbackSP("failed to write PR row (savepoint rolled back)", err) {
				continue
			}
		}

		// ── Critical write 2: patchset ────────────────────────────────────
		if err := w.recordPatchsetIfChanged(ctx, tx, prID, result.PR.HeadSHA, result.PR.BaseSHA); err != nil {
			if rollbackSP("failed to record patchset (savepoint rolled back)", err) {
				continue
			}
		}

		// ── Critical write 3: diffs ───────────────────────────────────────
		// Only when DiffComputed=true; failure rolls back PR+patchset too.
		if result.DiffComputed {
			if err := w.writeDiffs(ctx, tx, prID, result.DiffFiles); err != nil {
				if rollbackSP("failed to write diffs (savepoint rolled back)", err) {
					continue
				}
			}
			diffCount += len(result.DiffFiles)
		}

		// Look up the current patchset number for this PR's head+base so it can
		// be stamped on any review comments we write for this sync cycle.
		var currentPatchsetNum int
		tx.QueryRowContext(ctx,
			`SELECT number FROM patchsets WHERE pr_id = ? AND head_sha = ? AND base_sha = ? LIMIT 1`,
			prID, result.PR.HeadSHA, result.PR.BaseSHA,
		).Scan(&currentPatchsetNum) // best-effort; 0 = unknown (patchset not yet recorded)

		// ── Best-effort: comments (per-item skip, no savepoint rollback) ──
		for _, comment := range result.Comments {
			if comment == nil {
				continue
			}
			// Only stamp currentPatchsetNum for review comments with a current
			// position (Position != nil && > 0). Outdated/rebased comments
			// (Position == nil, only OriginalPosition) must keep their existing
			// patchset_number via COALESCE(NULL, existing) — overwriting with the
			// current patchset number would destroy the original patchset identity.
			commentPatchsetNum := 0
			if comment.Kind == "review" && comment.Position != nil && *comment.Position > 0 {
				commentPatchsetNum = currentPatchsetNum
			}
			commentStart := time.Now()
			if err := w.writeComment(ctx, tx, prID, comment, commentPatchsetNum); err != nil {
				log.Error("failed to write comment",
					"comment_id", comment.ID,
					"pr_id", prID,
					"duration_ms", time.Since(commentStart).Milliseconds(),
					"error", err,
				)
				continue
			}
			commentCount++
		}

		// ── Best-effort: review threads (per-item skip, no savepoint rollback)
		if len(result.Threads) > 0 {
			w.writeReviewThreads(ctx, tx, result.Threads, currentPatchsetNum)
		}

		// ── Release savepoint — commit this PR's work into the outer tx ───
		if _, spErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); spErr != nil {
			// Very rare. The work is already in the outer tx; log and continue.
			log.Warn("release savepoint failed (work retained in outer tx)",
				"pr_id", prID, "error", spErr)
		}

		// ── Best-effort anchor write-back (outside savepoint) ─────────────
		// Anchor updates run after the savepoint so their failure can never
		// roll back the PR/patchset/diffs that were already committed above.
		// Pass currentPatchsetNum so anchor results with PatchsetNumber=0
		// (current-position comments whose patchset was first recorded in this
		// cycle) get the correct non-NULL anchor_patchset_number on first write
		// instead of requiring a second sync to become fresh.
		if len(result.AnchorUpdates) > 0 {
			w.writeAnchorUpdates(ctx, tx, result.AnchorUpdates, currentPatchsetNum)
			log.Debug("anchor updates written",
				"pr_id", prID, "count", len(result.AnchorUpdates))
		}

		prCount++
		log.Debug("pr written",
			"pr_id", prID,
			"pr_number", result.PR.Number,
			"state", result.PR.State,
			"duration_ms", time.Since(writeStart).Milliseconds(),
		)
	}

	commitStart := time.Now()
	if err := tx.Commit(); err != nil {
		log.Error("failed to commit transaction",
			"duration_ms", time.Since(commitStart).Milliseconds(),
			"error", err,
		)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	totalDur := time.Since(overallStart).Milliseconds()
	logFn := log.Info
	if totalDur > 500 {
		logFn = log.Warn
	}
	logFn("sync results written",
		"prs", prCount,
		"prs_skipped", prSkipped,
		"prs_errors", prErrors,
		"comments", commentCount,
		"diffs", diffCount,
		"duration_ms", totalDur,
	)
	return nil
}

// writePRRow inserts or updates the PR row in the database using an upsert.
// It does NOT record patchsets — the caller (WriteSyncResultsContext) handles
// that separately inside the savepoint boundary.
func (w *Writer) writePRRow(ctx context.Context, tx *sql.Tx, pr *provider.PR) error {
	if pr.ID == "" {
		return fmt.Errorf("PR ID cannot be empty")
	}

	const query = `
		INSERT INTO prs (
			id, provider, owner, repo, number, title, body, author, state,
			created_at, updated_at, merged_at,
			base_branch, head_branch, base_sha, head_sha,
			comment_count, ci_status, draft,
			review_status, approved_by, changes_requested_by,
			requested_reviewers,
			repo_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title                  = excluded.title,
			body                   = excluded.body,
			state                  = excluded.state,
			updated_at             = excluded.updated_at,
			merged_at              = excluded.merged_at,
			base_branch            = excluded.base_branch,
			head_branch            = excluded.head_branch,
			base_sha               = excluded.base_sha,
			head_sha               = excluded.head_sha,
			comment_count          = excluded.comment_count,
			ci_status              = excluded.ci_status,
			draft                  = excluded.draft,
			review_status          = excluded.review_status,
			approved_by            = excluded.approved_by,
			changes_requested_by   = excluded.changes_requested_by,
			requested_reviewers    = excluded.requested_reviewers,
			repo_id                = excluded.repo_id
	`

	repoID := pr.Provider + ":" + pr.Owner + "/" + pr.Repo

	draft := 0
	if pr.Draft {
		draft = 1
	}

	reviewers := pr.RequestedReviewers
	if reviewers == nil {
		reviewers = []string{}
	}
	requestedReviewers, err := json.Marshal(reviewers)
	if err != nil {
		return fmt.Errorf("failed to marshal requested reviewers for %s: %w", pr.ID, err)
	}

	res, err := tx.ExecContext(ctx, query,
		pr.ID,
		pr.Provider,
		pr.Owner,
		pr.Repo,
		pr.Number,
		pr.Title,
		pr.Description,
		pr.Author,
		pr.State,
		pr.CreatedAt,
		pr.UpdatedAt,
		pr.MergedAt,
		pr.BaseBranch,
		pr.HeadBranch,
		pr.BaseSHA,
		pr.HeadSHA,
		pr.CommentCount,
		pr.CIStatus,
		draft,
		pr.ReviewStatus,
		pr.ApprovedBy,
		pr.ChangesRequestedBy,
		string(requestedReviewers),
		repoID,
	)
	if err != nil {
		return fmt.Errorf("failed to write PR %s: %w", pr.ID, err)
	}

	if logging.FromContext(ctx) != slog.Default() {
		if affected, err := res.RowsAffected(); err == nil {
			logging.FromContext(ctx).Debug("pr upsert rows affected",
				"pr_id", pr.ID,
				"rows_affected", affected,
			)
		}
	}

	return nil
}

// writeComment inserts or updates a comment in the database using an upsert.
// patchsetNum is the current patchset number for the PR (0 = unknown); it is
// stamped on kind='review' comments so anchoring can determine stale cache.
func (w *Writer) writeComment(ctx context.Context, tx *sql.Tx, prID string, comment *provider.Comment, patchsetNum int) error {
	if comment.ID == "" {
		return fmt.Errorf("comment ID cannot be empty")
	}
	if prID == "" {
		return fmt.Errorf("PR ID cannot be empty for comment %s", comment.ID)
	}

	kind := comment.Kind
	if kind == "" {
		kind = "issue"
	}

	// Stamp patchset_number only for review comments and only when known.
	var patchsetNumVal interface{}
	if kind == "review" && patchsetNum > 0 {
		patchsetNumVal = patchsetNum
	}

	const query = `
		INSERT INTO comments (
			id, pr_id, author, body, created_at, updated_at, line_number, file_path,
			kind, commit_sha, original_sha, side, position, original_position, in_reply_to, thread_id,
			patchset_number, diff_hunk
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			body              = excluded.body,
			updated_at        = excluded.updated_at,
			commit_sha        = excluded.commit_sha,
			original_sha      = excluded.original_sha,
			side              = excluded.side,
			position          = excluded.position,
			original_position = excluded.original_position,
			thread_id         = excluded.thread_id,
			patchset_number   = COALESCE(excluded.patchset_number, comments.patchset_number),
			diff_hunk         = excluded.diff_hunk
	`

	_, err := tx.ExecContext(ctx, query,
		comment.ID,
		prID,
		comment.Author,
		comment.Body,
		comment.CreatedAt,
		comment.UpdatedAt,
		nullableInt(comment.LineNo),
		nullableString(comment.FilePath),
		kind,
		nullableString(comment.CommitSHA),
		nullableString(comment.OriginalSHA),
		nullableString(comment.Side),
		comment.Position,
		comment.OriginalPosition,
		nullableString(comment.InReplyTo),
		nullableString(comment.ThreadID),
		patchsetNumVal,
		nullableString(comment.DiffHunk),
	)
	if err != nil {
		return fmt.Errorf("failed to write comment %s: %w", comment.ID, err)
	}

	return nil
}

// writeReviewThreads upserts review thread rows and their comments.
// currentPatchsetNum is applied to thread comments using the same rule as flat
// review comments: stamp only for kind='review' with current Position > 0;
// outdated comments (Position == nil) preserve existing patchset_number via COALESCE.
// Per-item errors are logged and skipped — they never abort the parent tx.
func (w *Writer) writeReviewThreads(ctx context.Context, tx *sql.Tx, threads []*provider.ReviewThread, currentPatchsetNum int) {
	for _, thread := range threads {
		if thread.ID == "" {
			w.log.Warn("skipping review thread with empty ID")
			continue
		}
		if thread.StateKnown {
			w.deletePromotedProvisionalThreads(ctx, tx, thread)
		}
		const threadQ = `
			INSERT INTO review_threads (id, pr_id, file_path, line, resolved, outdated, state_known)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				resolved    = excluded.resolved,
				outdated    = excluded.outdated,
				state_known = excluded.state_known,
				file_path   = excluded.file_path,
				line        = excluded.line`
		_, err := tx.ExecContext(ctx, threadQ,
			thread.ID, thread.PRID,
			nullableString(thread.FilePath),
			nullableInt(thread.Line),
			boolToInt(thread.Resolved),
			boolToInt(thread.Outdated),
			boolToInt(thread.StateKnown),
		)
		if err != nil {
			w.log.Error("write review_thread failed", "thread_id", thread.ID, "error", err)
			continue // skip this thread's comments too
		}
		for _, c := range thread.Comments {
			if c == nil {
				continue
			}
			// Apply the same patchset_number rule as flat review comments:
			// only stamp for current-position comments; outdated ones preserve
			// their existing patchset_number via COALESCE.
			commentPatchsetNum := 0
			if c.Kind == "review" && c.Position != nil && *c.Position > 0 {
				commentPatchsetNum = currentPatchsetNum
			}
			if err := w.writeComment(ctx, tx, c.PRID, c, commentPatchsetNum); err != nil {
				w.log.Error("write review comment failed",
					"comment_id", c.ID, "thread_id", thread.ID, "error", err)
				// continue: per-item skip, do not abort
			}
		}
	}
}

// deletePromotedProvisionalThreads removes stale REST-only provisional thread rows
// when the same logical thread is later promoted to a canonical provider thread ID.
// Deleting is safe: comments.thread_id is ON DELETE SET NULL, and this write then
// immediately upserts the child comments with the canonical thread.ID in the same tx.
func (w *Writer) deletePromotedProvisionalThreads(ctx context.Context, tx *sql.Tx, thread *provider.ReviewThread) {
	for _, provisionalID := range provisionalThreadIDs(thread) {
		if provisionalID == "" || provisionalID == thread.ID {
			continue
		}
		_, err := tx.ExecContext(ctx,
			`DELETE FROM review_threads WHERE id = ? AND state_known = 0`,
			provisionalID,
		)
		if err != nil {
			w.log.Warn("delete promoted provisional review_thread failed",
				"provisional_id", provisionalID, "canonical_id", thread.ID, "error", err)
		}
	}
}

// provisionalThreadIDs derives the REST fallback root IDs for a thread's comments.
// It mirrors github's reply-chain fallback shape without importing provider-specific
// code: every root comment starts one provisional thread, replies inherit the root.
func provisionalThreadIDs(thread *provider.ReviewThread) []string {
	byID := make(map[string]*provider.Comment, len(thread.Comments))
	for _, c := range thread.Comments {
		if c != nil && c.ID != "" {
			byID[c.ID] = c
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
			// Cycle detected: treat this node as its own root.
			memo[id] = id
			return id
		}
		c, ok := byID[id]
		if !ok || c.InReplyTo == "" {
			memo[id] = id
			return id
		}
		visiting[id] = true
		root := rootOf(c.InReplyTo)
		visiting[id] = false
		memo[id] = root
		return root
	}

	seen := make(map[string]struct{}, len(thread.Comments))
	ids := make([]string, 0, 1)
	for _, c := range thread.Comments {
		if c == nil || c.ID == "" {
			continue
		}
		root := rootOf(c.ID)
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		ids = append(ids, root)
	}
	return ids
}

// recordPatchsetIfChanged upserts a patchset row when head_sha changes.
// Must be called inside the PR's transaction (same tx as writePR).
//
// Algorithm:
//   - No prior patchset → nextNum = 1
//   - Prior patchset has same base_sha and head_sha → no-op
//   - Prior patchset has different base_sha or head_sha → nextNum = prior.number + 1
//
// UNIQUE(pr_id, base_sha, head_sha) is the race-safety guard; INSERT OR IGNORE
// makes concurrent calls for the same diff endpoints safely idempotent.
func (w *Writer) recordPatchsetIfChanged(ctx context.Context, tx *sql.Tx, prID, headSHA, baseSHA string) error {
	var latestNum int
	var latestHead string
	var latestBase string
	err := tx.QueryRowContext(ctx,
		`SELECT number, head_sha, base_sha FROM patchsets WHERE pr_id = ? ORDER BY number DESC LIMIT 1`,
		prID,
	).Scan(&latestNum, &latestHead, &latestBase)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("recordPatchsetIfChanged query: %w", err)
	}

	var nextNum int
	switch {
	case errors.Is(err, sql.ErrNoRows):
		nextNum = 1
	case latestHead == headSHA && latestBase == baseSHA:
		return nil // no-op: same diff endpoints
	default:
		nextNum = latestNum + 1
	}

	const insertQ = `
		INSERT OR IGNORE INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, insertQ, prID, nextNum, headSHA, baseSHA, time.Now()); err != nil {
		return fmt.Errorf("insert patchset %s P%d: %w", prID, nextNum, err)
	}
	return nil
}

// writeDiffs atomically replaces pr_diffs rows for prID within the provided
// transaction. It always deletes existing rows first (clears stale files,
// renames, and empty-diff state), then inserts the new set.
//
// This must be called within the same transaction as writePR so that PR row,
// patchset row, and diff cache advance together — never independently.
// Only call when DiffComputed==true; when diff generation failed the caller
// must skip this to preserve the last-known-good diff cache.
func (w *Writer) writeDiffs(ctx context.Context, tx *sql.Tx, prID string, diffs []store.DiffFile) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM pr_diffs WHERE pr_id = ?`, prID); err != nil {
		return fmt.Errorf("clear pr_diffs for %s: %w", prID, err)
	}
	if len(diffs) == 0 {
		return nil // cleared stale rows; zero-file diff is a valid empty state
	}
	const q = `INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("prepare pr_diffs insert for %s: %w", prID, err)
	}
	defer stmt.Close()
	for _, d := range diffs {
		if _, err := stmt.ExecContext(ctx, prID, d.FilePath, d.Patch, d.Additions, d.Deletions); err != nil {
			return fmt.Errorf("insert pr_diff %s/%s: %w", prID, d.FilePath, err)
		}
	}
	return nil
}

// writeAnchorUpdates persists AnchorResolver results for review comments.
// It runs OUTSIDE the per-PR savepoint (after RELEASE) so anchor failures never
// roll back the critical PR/patchset/diff writes. Best-effort: per-comment
// errors are logged and skipped.
//
// currentPatchsetNum is the patchset number just recorded by recordPatchsetIfChanged.
// It fills in zero values for both anchor_patchset_number and
// anchor_current_patchset_number when the patchset wasn't in the DB during fetch
// (first sync / new retarget). See comments on each field below.
func (w *Writer) writeAnchorUpdates(ctx context.Context, tx *sql.Tx, updates []git.AnchorResult, currentPatchsetNum int) {
	if len(updates) == 0 {
		return
	}
	const q = `
		UPDATE comments
		SET anchored_line                  = ?,
		    anchor_head_sha                = ?,
		    outdated                       = ?,
		    outdated_near_line             = ?,
		    anchor_patchset_number         = ?,
		    anchor_current_patchset_number = ?
		WHERE id = ?`
	for _, u := range updates {
		var anchoredLine interface{}
		if u.Anchored && u.AnchoredLine > 0 {
			anchoredLine = u.AnchoredLine
		}
		var nearLine interface{}
		if u.Outdated && u.OutdatedNearLine > 0 {
			nearLine = u.OutdatedNearLine
		}

		// anchor_patchset_number: SOURCE patchset (geometry for position-to-line).
		//   PatchsetNumber > 0: use as-is (historical patchset for original-position,
		//     or current patchset for current-position when found at fetch time).
		//   PatchsetNumber == 0: fill from currentPatchsetNum (current-position first sync).
		//   Both zero: NULL → stale.
		srcPs := u.PatchsetNumber
		if srcPs == 0 {
			srcPs = currentPatchsetNum
		}
		var anchorPatchsetNum interface{}
		if srcPs > 0 {
			anchorPatchsetNum = srcPs
		}

		// anchor_current_patchset_number: CURRENT patchset at resolution time.
		//   CurrentPatchsetNumber > 0: use as-is.
		//   CurrentPatchsetNumber == 0: fill from currentPatchsetNum.
		//   Both zero: NULL → stale.
		curPs := u.CurrentPatchsetNumber
		if curPs == 0 {
			curPs = currentPatchsetNum
		}
		var anchorCurPatchsetNum interface{}
		if curPs > 0 {
			anchorCurPatchsetNum = curPs
		}

		if _, err := tx.ExecContext(ctx, q,
			anchoredLine,
			u.HeadSHA, // always set; used for cache-validity check in handler
			boolToInt(u.Outdated),
			nearLine,
			anchorPatchsetNum,
			anchorCurPatchsetNum,
			u.CommentID,
		); err != nil {
			w.log.Warn("writeAnchorUpdates: failed to update anchor",
				"comment_id", u.CommentID, "error", err)
		}
	}
}

// nullableString returns nil for empty strings (maps to SQL NULL).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullableInt returns nil for zero ints (maps to SQL NULL for optional positions).
func nullableInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

// boolToInt converts bool to SQLite integer (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
