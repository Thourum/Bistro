package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

type prListRequest struct {
	Filter string `json:"filter"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type prRow struct {
	ID                 string   `json:"id"`
	Provider           string   `json:"provider"`
	Owner              string   `json:"owner"`
	Repo               string   `json:"repo"`
	Number             int      `json:"number"`
	Title              string   `json:"title"`
	Body               *string  `json:"body"`
	Author             string   `json:"author"`
	State              string   `json:"state"`
	CreatedAt          string   `json:"created_at"`
	UpdatedAt          string   `json:"updated_at"`
	MergedAt           *string  `json:"merged_at"`
	BaseBranch         string   `json:"base_branch"`
	HeadBranch         string   `json:"head_branch"`
	BaseSHA            string   `json:"base_sha"`
	HeadSHA            string   `json:"head_sha"`
	CommentCount       int      `json:"comment_count"`
	CIStatus           *string  `json:"ci_status"`
	Draft              int      `json:"draft"`
	ReviewStatus       *string  `json:"review_status"`
	ApprovedBy         *string  `json:"approved_by"`
	ChangesRequestedBy *string  `json:"changes_requested_by"`
	RequestedReviewers []string `json:"requested_reviewers"`
	TouchesYou         bool     `json:"touches_you"`
	Mentioned          bool     `json:"mentioned"`
	ViewerCommented    bool     `json:"viewer_commented"`
	Unread             bool     `json:"unread"`
	ReviewedHeadStale  bool     `json:"reviewed_head_stale"`
	Hydrated           int      `json:"hydrated"`
}

// prLister is the subset of *sql.DB needed to list PRs.
type prLister interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

type touchesYouResolver interface {
	TouchesYouForPRs(ctx context.Context, prIDs []string) (map[string]bool, error)
}

// decodeReviewers parses the requested_reviewers JSON column. NULL (rows
// written before migration 010) and malformed values decode to an empty slice.
func decodeReviewers(logger *slog.Logger, prID string, raw *string) []string {
	if raw == nil || *raw == "" {
		return []string{}
	}
	var reviewers []string
	if err := json.Unmarshal([]byte(*raw), &reviewers); err != nil {
		logger.Warn("invalid requested_reviewers JSON", "pr_id", prID, "err", err)
		return []string{}
	}
	if reviewers == nil {
		return []string{}
	}
	return reviewers
}

func prListErrorResponse(requestID string, err error) map[string]interface{} {
	return map[string]interface{}{
		"id":   requestID,
		"type": "PR_LIST_READY",
		"data": map[string]interface{}{
			"prs":      []interface{}{},
			"count":    0,
			"has_more": false,
			"error":    err.Error(),
		},
	}
}

func NewRequestPRListHandler(db prLister, cu currentUserReader, log *slog.Logger) ws.MessageHandler {
	return NewRequestPRListHandlerWithExpertise(db, nil, cu, log)
}

func NewRequestPRListHandlerWithExpertise(db prLister, touchesYou touchesYouResolver, cu currentUserReader, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req prListRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_PR_LIST payload: %w", err)
		}

		if req.Limit <= 0 || req.Limit > 200 {
			req.Limit = 100
		}
		if req.Offset < 0 {
			req.Offset = 0
		}

		logger.Debug("pr list requested",
			"filter", req.Filter,
			"limit", req.Limit,
			"offset", req.Offset,
		)

		login := ""
		if cu != nil {
			resolvedLogin, found, err := cu.GetCurrentUser(ctx, "github")
			if err != nil {
				logger.Warn("resolve current user for REQUEST_PR_LIST", "err", err)
			} else if found && resolvedLogin != "" {
				login = resolvedLogin
			}
		}

		query := `SELECT id, provider, owner, repo, number, title, body, author, state, created_at, updated_at, merged_at, base_branch, head_branch, base_sha, head_sha, comment_count, ci_status, draft, review_status, approved_by, changes_requested_by, requested_reviewers, mentioned,
			(? <> '' AND EXISTS(SELECT 1 FROM comments c WHERE c.pr_id = prs.id AND c.author = ?)) AS viewer_commented,
			(? <> '' AND COALESCE((SELECT unixepoch(r.last_read_at) FROM pr_read_state r WHERE r.pr_id = prs.id AND r.login = ?), 0) < unixepoch(prs.updated_at)) AS unread,
			(? <> '' AND EXISTS(SELECT 1 FROM last_reviewed_sha lrs WHERE lrs.pr_id = prs.id AND lrs.account_provider = 'github' AND lrs.account_login = ? AND lrs.login = ? AND lrs.sha != prs.head_sha)) AS reviewed_head_stale,
			hydrated FROM prs WHERE 1=1`

		switch req.Filter {
		case "open":
			query += " AND state = 'open' AND draft = 0"
		case "draft":
			query += " AND draft = 1"
		case "all":
			// no filter
		default:
			query += " AND state = 'open' AND draft = 0"
		}

		query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"

		rows, err := db.Query(query, login, login, login, login, login, login, login, req.Limit, req.Offset)
		if err != nil {
			logger.Error("REQUEST_PR_LIST query failed", "err", err)
			return prListErrorResponse(msg.ID, err), nil
		}
		defer rows.Close()

		var prs []prRow
		prIDs := []string{}
		for rows.Next() {
			var pr prRow
			var requestedReviewers *string
			if err := rows.Scan(
				&pr.ID, &pr.Provider, &pr.Owner, &pr.Repo, &pr.Number,
				&pr.Title, &pr.Body, &pr.Author, &pr.State,
				&pr.CreatedAt, &pr.UpdatedAt, &pr.MergedAt,
				&pr.BaseBranch, &pr.HeadBranch, &pr.BaseSHA, &pr.HeadSHA,
				&pr.CommentCount, &pr.CIStatus, &pr.Draft,
				&pr.ReviewStatus, &pr.ApprovedBy, &pr.ChangesRequestedBy,
				&requestedReviewers, &pr.Mentioned, &pr.ViewerCommented, &pr.Unread,
				&pr.ReviewedHeadStale, &pr.Hydrated,
			); err != nil {
				logger.Error("REQUEST_PR_LIST scan failed", "err", err)
				return prListErrorResponse(msg.ID, err), nil
			}
			pr.RequestedReviewers = decodeReviewers(logger, pr.ID, requestedReviewers)
			prs = append(prs, pr)
			prIDs = append(prIDs, pr.ID)
		}
		if err := rows.Err(); err != nil {
			logger.Error("REQUEST_PR_LIST iteration failed", "err", err)
			return prListErrorResponse(msg.ID, err), nil
		}

		if touchesYou != nil && len(prIDs) > 0 {
			matches, err := touchesYou.TouchesYouForPRs(ctx, prIDs)
			if err != nil {
				logger.Warn("touches_you resolution failed", "err", err)
			} else {
				for index, pr := range prs {
					prs[index].TouchesYou = matches[pr.ID]
				}
			}
		}

		if prs == nil {
			prs = []prRow{}
		}

		logger.Debug("pr list ready",
			"filter", req.Filter,
			"returned", len(prs),
			"has_more", len(prs) == req.Limit,
		)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "PR_LIST_READY",
			"data": map[string]interface{}{
				"prs":          prs,
				"count":        len(prs),
				"has_more":     len(prs) == req.Limit,
				"viewer_login": login,
			},
		}, nil
	}
}
