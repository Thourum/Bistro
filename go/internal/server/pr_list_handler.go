package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/samuelasvanyi/bistro/internal/ws"
)

type prListRequest struct {
	Filter string `json:"filter"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type prRow struct {
	ID           string  `json:"id"`
	Provider     string  `json:"provider"`
	Owner        string  `json:"owner"`
	Repo         string  `json:"repo"`
	Number       int     `json:"number"`
	Title        string  `json:"title"`
	Body         *string `json:"body"`
	Author       string  `json:"author"`
	State        string  `json:"state"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	MergedAt     *string `json:"merged_at"`
	BaseBranch   string  `json:"base_branch"`
	HeadBranch   string  `json:"head_branch"`
	BaseSHA      string  `json:"base_sha"`
	HeadSHA      string  `json:"head_sha"`
	CommentCount int     `json:"comment_count"`
	CIStatus     *string `json:"ci_status"`
	Draft        int     `json:"draft"`
}

// prLister is the subset of *sql.DB needed to list PRs.
type prLister interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func newRequestPRListHandler(db prLister, log *slog.Logger) ws.MessageHandler {
	return func(msg ws.InboundMessage) (interface{}, error) {
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

		query := "SELECT id, provider, owner, repo, number, title, body, author, state, created_at, updated_at, merged_at, base_branch, head_branch, base_sha, head_sha, comment_count, ci_status, draft FROM prs WHERE 1=1"

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

		rows, err := db.Query(query, req.Limit, req.Offset)
		if err != nil {
			log.Error("REQUEST_PR_LIST query failed", "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "PR_LIST_READY",
				"data": map[string]interface{}{
					"prs":      []interface{}{},
					"total":    0,
					"has_more": false,
					"error":    err.Error(),
				},
			}, nil
		}
		defer rows.Close()

		var prs []prRow
		for rows.Next() {
			var pr prRow
			if err := rows.Scan(
				&pr.ID, &pr.Provider, &pr.Owner, &pr.Repo, &pr.Number,
				&pr.Title, &pr.Body, &pr.Author, &pr.State,
				&pr.CreatedAt, &pr.UpdatedAt, &pr.MergedAt,
				&pr.BaseBranch, &pr.HeadBranch, &pr.BaseSHA, &pr.HeadSHA,
				&pr.CommentCount, &pr.CIStatus, &pr.Draft,
			); err != nil {
				log.Error("REQUEST_PR_LIST scan failed", "err", err)
				continue
			}
			prs = append(prs, pr)
		}

		if prs == nil {
			prs = []prRow{}
		}

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "PR_LIST_READY",
			"data": map[string]interface{}{
				"prs":      prs,
				"total":    len(prs),
				"has_more": len(prs) == req.Limit,
			},
		}, nil
	}
}
