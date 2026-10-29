package services

import (
	"context"
	"encoding/json"
	"log/slog"
)

const (
	defaultPRListLimit = 100
	maxPRListLimit     = 200
)

type PRListRequest struct {
	Filter string `json:"filter"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type PRRow struct {
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

type PRListResponse struct {
	PRs         []PRRow `json:"prs"`
	Count       int     `json:"count"`
	HasMore     bool    `json:"has_more"`
	ViewerLogin string  `json:"viewer_login"`
}

// PR serves cached lists and blocking on-demand hydration.
type PR struct {
	DB          prLister
	CurrentUser currentUserReader
	TouchesYou  touchesYouResolver
	GetSyncer   func() PRSyncer
	Panics      PanicReporter
	Log         *slog.Logger
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

// List keeps viewer annotations and optional expertise enrichment available offline.
func (s *PR) List(ctx context.Context, req PRListRequest) (PRListResponse, error) {
	return guard(s.Panics, "PR.List", func() (PRListResponse, error) {
		logger := serviceLogger(s.Log)
		if err := ctx.Err(); err != nil {
			return PRListResponse{}, err
		}
		if req.Limit <= 0 {
			req.Limit = defaultPRListLimit
		}
		req.Limit = min(req.Limit, maxPRListLimit)
		if req.Offset < 0 {
			req.Offset = 0
		}

		logger.Debug("pr list requested",
			"filter", req.Filter,
			"limit", req.Limit,
			"offset", req.Offset,
		)

		login := ""
		if s.CurrentUser != nil {
			resolvedLogin, found, err := s.CurrentUser.Get(ctx, githubProvider)
			if err != nil {
				// Listing remains available without viewer-specific annotations.
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

		rows, err := s.DB.Query(query, login, login, login, login, login, login, login, req.Limit, req.Offset)
		if err != nil {
			logger.Error("REQUEST_PR_LIST query failed", "err", err)
			return PRListResponse{}, Errorf(CodeInternal, "unable to load pull requests")
		}
		defer rows.Close()

		var prs []PRRow
		prIDs := []string{}
		for rows.Next() {
			var pr PRRow
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
				return PRListResponse{}, Errorf(CodeInternal, "unable to load pull requests")
			}
			pr.RequestedReviewers = decodeReviewers(logger, pr.ID, requestedReviewers)
			prs = append(prs, pr)
			prIDs = append(prIDs, pr.ID)
		}
		if err := rows.Err(); err != nil {
			logger.Error("REQUEST_PR_LIST iteration failed", "err", err)
			return PRListResponse{}, Errorf(CodeInternal, "unable to load pull requests")
		}

		if s.TouchesYou != nil && len(prIDs) > 0 {
			matches, err := s.TouchesYou.TouchesYouForPRs(ctx, prIDs)
			if err != nil {
				// Expertise enrichment is optional; keep the PR list usable.
				logger.Warn("touches_you resolution failed", "err", err)
			} else {
				for index, pr := range prs {
					prs[index].TouchesYou = matches[pr.ID]
				}
			}
		}

		if prs == nil {
			prs = []PRRow{}
		}

		logger.Debug("pr list ready",
			"filter", req.Filter,
			"returned", len(prs),
			"has_more", len(prs) == req.Limit,
		)

		return PRListResponse{PRs: prs, Count: len(prs), HasMore: len(prs) == req.Limit, ViewerLogin: login}, nil
	})
}

// Sync returns only after the PR's hydrated details have been persisted.
func (s *PR) Sync(ctx context.Context, req PRRef) (PRRef, error) {
	return guard(s.Panics, "PR.Sync", func() (PRRef, error) {
		req.PRID = NormalizeID(req.PRID)
		if req.PRID == "" {
			return PRRef{}, Errorf(CodeInvalid, "pr_id is required")
		}
		if err := ctx.Err(); err != nil {
			return PRRef{}, err
		}
		var syncer PRSyncer
		if s.GetSyncer != nil {
			syncer = s.GetSyncer()
		}
		if syncer == nil {
			return PRRef{}, Errorf(CodeUnauthenticated, "no authenticated provider")
		}
		if err := syncer.SyncPR(ctx, req.PRID); err != nil {
			return PRRef{}, err
		}
		return req, nil
	})
}
