package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
)

type prWriteMode int

const (
	listOnlyWrite prWriteMode = iota
	hydratedWrite
)

// A list-only write preserves existing detail fields unless the diff endpoints
// changed. A hydrated write refreshes them; the caller decides which is safe.
func (w *Writer) writePRRow(ctx context.Context, tx *sql.Tx, pr *provider.PR, mode prWriteMode) error {
	if pr.ID == "" {
		return errors.New("pr id cannot be empty")
	}
	requestedReviewers := pr.RequestedReviewers
	if requestedReviewers == nil {
		requestedReviewers = []string{}
	}
	reviewersJSON, err := json.Marshal(requestedReviewers)
	if err != nil {
		return fmt.Errorf("marshal requested reviewers for PR %s: %w", pr.ID, err)
	}
	_, err = tx.ExecContext(ctx, prUpsertQuery,
		pr.ID, pr.Provider, pr.Owner, pr.Repo, pr.Number, pr.Title, pr.Description,
		pr.Author, pr.State, pr.CreatedAt, pr.UpdatedAt, pr.MergedAt, pr.BaseBranch,
		pr.HeadBranch, pr.BaseSHA, pr.HeadSHA, pr.CommentCount, pr.CIStatus,
		pr.Draft, pr.ReviewStatus, pr.ApprovedBy, pr.ChangesRequestedBy,
		string(reviewersJSON), git.Identity{Provider: pr.Provider, Owner: pr.Owner, Repo: pr.Repo}.ID(), int(mode),
	)
	if err != nil {
		return fmt.Errorf("write PR %s: %w", pr.ID, err)
	}
	return nil
}

const prUpsertQuery = `
	INSERT INTO prs (
		id, provider, owner, repo, number, title, body, author, state,
		created_at, updated_at, merged_at,
		base_branch, head_branch, base_sha, head_sha,
		comment_count, ci_status, draft,
		review_status, approved_by, changes_requested_by,
		requested_reviewers,
		repo_id,
		hydrated
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		comment_count = CASE
			WHEN excluded.hydrated = 0 AND (prs.base_sha <> excluded.base_sha OR prs.head_sha <> excluded.head_sha) THEN 0
			WHEN excluded.hydrated = 0 THEN prs.comment_count
			ELSE excluded.comment_count
		END,
		ci_status = CASE
			WHEN excluded.hydrated = 0 AND (prs.base_sha <> excluded.base_sha OR prs.head_sha <> excluded.head_sha) THEN NULL
			WHEN excluded.hydrated = 0 THEN prs.ci_status
			ELSE excluded.ci_status
		END,
		draft = excluded.draft,
		review_status = CASE
			WHEN excluded.hydrated = 0 AND (prs.base_sha <> excluded.base_sha OR prs.head_sha <> excluded.head_sha) THEN NULL
			WHEN excluded.hydrated = 0 THEN prs.review_status
			ELSE excluded.review_status
		END,
		approved_by = CASE
			WHEN excluded.hydrated = 0 AND (prs.base_sha <> excluded.base_sha OR prs.head_sha <> excluded.head_sha) THEN NULL
			WHEN excluded.hydrated = 0 THEN prs.approved_by
			ELSE excluded.approved_by
		END,
		changes_requested_by = CASE
			WHEN excluded.hydrated = 0 AND (prs.base_sha <> excluded.base_sha OR prs.head_sha <> excluded.head_sha) THEN NULL
			WHEN excluded.hydrated = 0 THEN prs.changes_requested_by
			ELSE excluded.changes_requested_by
		END,
		requested_reviewers = excluded.requested_reviewers,
		repo_id             = excluded.repo_id,
		hydrated = CASE
			WHEN excluded.hydrated = 0 AND (prs.base_sha <> excluded.base_sha OR prs.head_sha <> excluded.head_sha) THEN 0
			ELSE MAX(prs.hydrated, excluded.hydrated)
		END
`

type commentWrite struct {
	prID           string
	comment        *provider.Comment
	patchsetNumber int
	engine         *classify.Engine
}

func (w *Writer) writeComment(ctx context.Context, tx *sql.Tx, write commentWrite) error {
	comment := write.comment
	if comment.ID == "" {
		return errors.New("comment id cannot be empty")
	}
	if write.prID == "" {
		return fmt.Errorf("pr id cannot be empty for comment %s", comment.ID)
	}
	kind := comment.Kind
	if kind == "" {
		kind = "issue"
	}
	var patchsetNumber any
	if kind == "review" && write.patchsetNumber > 0 {
		patchsetNumber = write.patchsetNumber
	}
	classification := classifyComment(write.engine, comment)
	if _, err := tx.ExecContext(ctx, commentUpsertQuery,
		comment.ID, write.prID, comment.Author, comment.Body, comment.CreatedAt,
		comment.UpdatedAt, nullableInt(comment.LineNo), nullableString(comment.FilePath),
		kind, nullableString(comment.CommitSHA), nullableString(comment.OriginalSHA),
		nullableString(comment.Side), comment.Position, comment.OriginalPosition,
		nullableString(comment.InReplyTo), nullableString(comment.ThreadID), patchsetNumber,
		nullableString(comment.DiffHunk), comment.OriginalLine, comment.StartLine,
		comment.OriginalStartLine, nullableString(comment.StartSide), nullableString(comment.SubjectType),
		classification.authorType, classification.botVendor, classification.severity, classification.source,
	); err != nil {
		return fmt.Errorf("write comment %s: %w", comment.ID, err)
	}
	if write.engine == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES (?, ?)
		 ON CONFLICT(comment_id) DO UPDATE SET fingerprint = excluded.fingerprint`,
		comment.ID, write.engine.Fingerprint(),
	); err != nil {
		return fmt.Errorf("write classification state for comment %s: %w", comment.ID, err)
	}
	return nil
}

type commentClassification struct {
	authorType any
	botVendor  any
	severity   any
	source     any
}

func classifyComment(engine *classify.Engine, comment *provider.Comment) commentClassification {
	if engine == nil {
		return commentClassification{} // SQL COALESCE preserves cached classification.
	}
	classification := engine.Classify(comment.Author, comment.Body, comment.FilePath)
	return commentClassification{
		authorType: classification.AuthorType, botVendor: nullableString(classification.BotVendor),
		severity: nullableString(classification.Severity), source: classification.Source,
	}
}

const commentUpsertQuery = `
	INSERT INTO comments (
		id, pr_id, author, body, created_at, updated_at, line_number, file_path,
		kind, commit_sha, original_sha, side, position, original_position, in_reply_to, thread_id,
		patchset_number, diff_hunk,
		original_line, start_line, original_start_line, start_side, subject_type,
		author_type, bot_vendor, severity, classification_source
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		diff_hunk         = excluded.diff_hunk,
		original_line     = excluded.original_line,
		start_line        = excluded.start_line,
		original_start_line = excluded.original_start_line,
		start_side        = excluded.start_side,
		subject_type      = excluded.subject_type,
		author_type       = COALESCE(excluded.author_type, comments.author_type),
		bot_vendor        = CASE WHEN excluded.author_type IS NULL THEN comments.bot_vendor ELSE excluded.bot_vendor END,
		severity          = CASE WHEN excluded.author_type IS NULL THEN comments.severity ELSE excluded.severity END,
		classification_source = COALESCE(excluded.classification_source, comments.classification_source)
`

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
