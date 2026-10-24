package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

type commentsRequest struct {
	PRID string `json:"pr_id"`
}

// commentRow is the per-comment shape returned in COMMENTS_READY.
// Anchor fields (migrations 015, 018, 019) are populated for kind='review' comments.
//
// Freshness rules (both must hold for any anchor-derived fields to be returned):
//  1. anchor_head_sha        == prs.head_sha              (anchor for current head)
//  2. anchor_current_patchset_number == current_patchset  (anchor for current base+head patchset)
//
// For original-position / historical comments, an additional source check:
//  3. anchor_patchset_number == comments.patchset_number  (correct geometry was used)
//
// NULL anchor_current_patchset_number is always stale (pre-migration 019 data).
type commentRow struct {
	ID        string  `json:"id"`
	PRID      string  `json:"pr_id"`
	Author    string  `json:"author"`
	Body      string  `json:"body"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Line      *int    `json:"line,omitempty"`
	File      *string `json:"file,omitempty"`
	// migration 012 fields
	Kind              *string `json:"kind,omitempty"`
	CommitSHA         *string `json:"commit_sha,omitempty"`
	OriginalSHA       *string `json:"original_sha,omitempty"`
	Side              *string `json:"side,omitempty"`
	Position          *int    `json:"position,omitempty"`
	OriginalPosition  *int    `json:"original_position,omitempty"`
	OriginalLine      *int    `json:"original_line,omitempty"`
	StartLine         *int    `json:"start_line,omitempty"`
	OriginalStartLine *int    `json:"original_start_line,omitempty"`
	StartSide         *string `json:"start_side,omitempty"`
	SubjectType       *string `json:"subject_type,omitempty"`
	InReplyTo         *string `json:"in_reply_to,omitempty"`
	ThreadID          *string `json:"thread_id,omitempty"`
	PatchsetNumber    *int    `json:"patchset_number,omitempty"`
	// migration 020: diff hunk for review comments
	DiffHunk *string `json:"diff_hunk,omitempty"`
	// migration 015+018+019 anchor fields
	AnchoredLine                *int    `json:"anchored_line,omitempty"`
	AnchorHeadSHA               *string `json:"anchor_head_sha,omitempty"`
	Outdated                    bool    `json:"outdated"`
	OutdatedNearLine            *int    `json:"outdated_near_line,omitempty"`
	AnchorPatchsetNumber        *int    `json:"anchor_patchset_number,omitempty"`
	AnchorCurrentPatchsetNumber *int    `json:"anchor_current_patchset_number,omitempty"`
	// Anchored is computed: true only when anchor cache is fully fresh.
	Anchored bool `json:"anchored"`
	// migration 026: classification fields (Phase 3). Absent on legacy /
	// unclassified rows.
	AuthorType           *string `json:"author_type,omitempty"`
	BotVendor            *string `json:"bot_vendor,omitempty"`
	Severity             *string `json:"severity,omitempty"`
	ClassificationSource *string `json:"classification_source,omitempty"`
	// migration 027: per-account dismissal (Phase 3.5). Omitted when false.
	Dismissed bool `json:"dismissed,omitempty"`
	// migration 028: multi-bot dedup group (Phase 3.6). NULL when ungrouped.
	DedupGroupID *string `json:"dedup_group_id,omitempty"`
}

// commentsStore is what the comments handler needs from the DB: reads for
// serving, writes for the lazy re-classification pass (Go owns all SQLite
// writes; the refresh runs server-side, never renderer-side).
type commentsStore interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// NewRequestCommentsHandler returns the handler for REQUEST_COMMENTS messages
// without a classification engine (no refresh pass; classification columns
// are served as stored).
func NewRequestCommentsHandler(db commentsStore, log *slog.Logger) ws.MessageHandler {
	return NewRequestCommentsHandlerWithClassifier(db, log, nil)
}

// refreshIfStale re-runs the deterministic classification tier for comments
// of prID whose stored fingerprint differs from the provider's current one —
// the spec's lazy model: rule edits and mode changes apply on next PR open,
// no watcher, no restart. Errors are logged and never block serving:
// classification is enrichment, never a gate.
func refreshIfStale(db commentsStore, provider *classify.EngineProvider, prID string, log *slog.Logger) {
	eng := provider.Current()
	fp := eng.Fingerprint()

	rows, err := db.Query(
		`SELECT c.id, c.author, c.body, COALESCE(c.file_path, '')
		 FROM comments c
		 LEFT JOIN comment_classification_state s ON s.comment_id = c.id
		 WHERE c.pr_id = ? AND (s.fingerprint IS NULL OR s.fingerprint != ?)`,
		prID, fp,
	)
	if err != nil {
		log.Warn("classification refresh query failed", "pr_id", prID, "err", err)
		return
	}
	type staleComment struct{ id, author, body, path string }
	var stale []staleComment
	for rows.Next() {
		var s staleComment
		if err := rows.Scan(&s.id, &s.author, &s.body, &s.path); err != nil {
			log.Warn("classification refresh scan failed", "pr_id", prID, "err", err)
			continue
		}
		stale = append(stale, s)
	}
	rows.Close()

	for _, s := range stale {
		c := eng.Classify(s.author, s.body, s.path)
		if _, err := db.Exec(
			`UPDATE comments SET author_type = ?, bot_vendor = ?, severity = ?, classification_source = ?
			 WHERE id = ?`,
			c.AuthorType, nullIfEmpty(c.BotVendor), nullIfEmpty(c.Severity), c.Source, s.id,
		); err != nil {
			log.Warn("classification refresh update failed", "comment_id", s.id, "err", err)
			continue
		}
		if _, err := db.Exec(
			`INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES (?, ?)
			 ON CONFLICT(comment_id) DO UPDATE SET fingerprint = excluded.fingerprint`,
			s.id, fp,
		); err != nil {
			log.Warn("classification state upsert failed", "comment_id", s.id, "err", err)
		}
	}
	if len(stale) > 0 {
		log.Debug("re-classified stale comments", "pr_id", prID, "count", len(stale))
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// NewRequestCommentsHandlerWithClassifier returns the REQUEST_COMMENTS
// handler with lazy re-classification: comments whose stored fingerprint
// differs from the provider's current engine are re-classified before
// serving. Nil provider = no refresh.
func NewRequestCommentsHandlerWithClassifier(db commentsStore, log *slog.Logger, provider *classify.EngineProvider) ws.MessageHandler {
	return NewRequestCommentsHandlerWithLLMKick(db, log, provider, nil)
}

func withFreshAnchor(comment commentRow, currentHeadSHA *string, currentPatchsetNumber *int) commentRow {
	// Anchor validity ties the head and current (base, head) patchset together:
	// a retarget with the same head still invalidates old anchor geometry.
	fresh := comment.AnchorHeadSHA != nil && currentHeadSHA != nil &&
		*comment.AnchorHeadSHA == *currentHeadSHA &&
		comment.AnchorCurrentPatchsetNumber != nil && currentPatchsetNumber != nil &&
		*comment.AnchorCurrentPatchsetNumber == *currentPatchsetNumber
	if fresh && comment.PatchsetNumber != nil && *comment.PatchsetNumber != *currentPatchsetNumber {
		// Historical positions must have been calculated from their own patchset.
		fresh = comment.AnchorPatchsetNumber != nil && *comment.AnchorPatchsetNumber == *comment.PatchsetNumber
	}
	if fresh {
		comment.Anchored = comment.AnchoredLine != nil
		return comment
	}
	comment.AnchoredLine = nil
	comment.AnchorHeadSHA = nil
	comment.AnchorPatchsetNumber = nil
	comment.AnchorCurrentPatchsetNumber = nil
	comment.Outdated = false
	comment.OutdatedNearLine = nil
	comment.Anchored = false
	return comment
}

func commentsErrorResponse(requestID, prID string) map[string]interface{} {
	return map[string]interface{}{
		"id":   requestID,
		"type": "COMMENTS_READY",
		"data": map[string]interface{}{
			"pr_id":    prID,
			"comments": []commentRow{},
			"error":    "unable to load comments",
		},
	}
}

// NewRequestCommentsHandlerWithLLMKick additionally kicks the async BYOK LLM
// tier for the opened PR (identical code path as post-sync; the cache makes
// repeats cheap). kick is invoked after the deterministic refresh, before
// serving — the tier itself no-ops in non-LLM modes. Nil kick = no tier.
func NewRequestCommentsHandlerWithLLMKick(db commentsStore, log *slog.Logger, provider *classify.EngineProvider, kick func(prID string)) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req commentsRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_COMMENTS payload: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("REQUEST_COMMENTS requires pr_id")
		}

		logger.Debug("comments requested", "pr_id", req.PRID)

		// Lazy re-classification: rule edits / mode changes apply here,
		// on PR open, before the SELECT below serves the rows.
		if provider != nil {
			refreshIfStale(db, provider, req.PRID, logger)
		}
		// Async LLM refinement for this PR (no-op outside LLM modes).
		if kick != nil {
			kick(req.PRID)
		}

		rows, err := db.Query(
			`SELECT c.id, c.pr_id, c.author, c.body, c.created_at, c.updated_at,
			        c.line_number, c.file_path,
			        c.kind, c.commit_sha, c.original_sha, c.side, c.position,
			        c.original_position, c.in_reply_to, c.thread_id, c.patchset_number,
			        c.diff_hunk,
			        c.original_line, c.start_line, c.original_start_line, c.start_side, c.subject_type,
			        c.anchored_line, c.anchor_head_sha, c.outdated, c.outdated_near_line,
			        c.anchor_patchset_number,
			        c.anchor_current_patchset_number,
			        c.author_type, c.bot_vendor, c.severity, c.classification_source,
			        c.dedup_group_id,
			        (d.comment_id IS NOT NULL) AS dismissed,
			        p.head_sha  AS current_head_sha,
			        ps.number   AS current_patchset_number
			 FROM comments c
			 LEFT JOIN prs p  ON p.id = c.pr_id
			 LEFT JOIN patchsets ps
			        ON ps.pr_id    = c.pr_id
			       AND ps.number = (SELECT MAX(x.number) FROM patchsets x WHERE x.pr_id = c.pr_id AND x.head_sha = p.head_sha)
			 LEFT JOIN comment_dismissals d
			        ON d.comment_id = c.id
			       AND (d.account_provider, d.account_login) IN (SELECT provider, login FROM current_user)
			 WHERE c.pr_id = ?
			 ORDER BY c.created_at ASC`,
			req.PRID,
		)
		if err != nil {
			logger.Error("REQUEST_COMMENTS query failed", "pr_id", req.PRID, "err", err)
			return commentsErrorResponse(msg.ID, req.PRID), nil
		}
		defer rows.Close()

		comments := []commentRow{}
		for rows.Next() {
			var c commentRow
			var currentHeadSHA *string
			var currentPatchsetNumber *int
			if err := rows.Scan(
				&c.ID, &c.PRID, &c.Author, &c.Body,
				&c.CreatedAt, &c.UpdatedAt, &c.Line, &c.File,
				&c.Kind, &c.CommitSHA, &c.OriginalSHA, &c.Side,
				&c.Position, &c.OriginalPosition,
				&c.InReplyTo, &c.ThreadID, &c.PatchsetNumber,
				&c.DiffHunk,
				&c.OriginalLine, &c.StartLine, &c.OriginalStartLine, &c.StartSide, &c.SubjectType,
				&c.AnchoredLine, &c.AnchorHeadSHA, &c.Outdated, &c.OutdatedNearLine,
				&c.AnchorPatchsetNumber,
				&c.AnchorCurrentPatchsetNumber,
				&c.AuthorType, &c.BotVendor, &c.Severity, &c.ClassificationSource,
				&c.DedupGroupID,
				&c.Dismissed,
				&currentHeadSHA,
				&currentPatchsetNumber,
			); err != nil {
				logger.Error("REQUEST_COMMENTS scan failed", "pr_id", req.PRID, "err", err)
				continue
			}

			c = withFreshAnchor(c, currentHeadSHA, currentPatchsetNumber)
			comments = append(comments, c)
		}

		logger.Debug("comments ready", "pr_id", req.PRID, "count", len(comments))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "COMMENTS_READY",
			"data": map[string]interface{}{
				"pr_id":    req.PRID,
				"comments": comments,
			},
		}, nil
	}
}
