package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/ws"
)

// diffReader is the subset of prdiff.Store required by the REQUEST_DIFF handler.
type diffReader interface {
	GetUnified(prID string) (string, error)
	Get(prID string) ([]prdiff.File, error)
}

// diffViewedChecker can look up whether a file has been viewed.
// Satisfied by *store.DB.
type diffViewedChecker interface {
	IsFileViewed(ctx context.Context, prID, filePath, login, contentHash string) (bool, error)
}

// DiffHandlerDeps bundles all dependencies for the REQUEST_DIFF handler.
// ViewedStore and CurrentUser are optional; when absent, content_hash is still
// computed but viewed is always false.
type DiffHandlerDeps struct {
	DiffStore   diffReader
	ViewedStore diffViewedChecker // optional
	CurrentUser currentUserReader // optional
	Log         *slog.Logger
}

// diffFileDetail is the per-file entry in the DIFF_READY response.
// content_hash is the patch ID of the file's change (server-computed, canonical).
// viewed is true only when the current user has seen this change.
type diffFileDetail struct {
	Path        string `json:"path"`
	Status      string `json:"status"`
	OldPath     string `json:"old_path,omitempty"`
	IsBinary    bool   `json:"is_binary"`
	IsTruncated bool   `json:"is_truncated"`
	DiffStatus  string `json:"diff_status"`
	PatchBytes  int    `json:"patch_bytes"`
	HunkCount   int    `json:"hunk_count"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	ContentHash string `json:"content_hash"`
	Viewed      bool   `json:"viewed"`
}

// viewedHash uses the patch ID so viewed state survives line-shift-only rebases.
// Rows cached before migration 035 have no stored patch ID; compute it instead.
func viewedHash(file prdiff.File) string {
	if file.PatchID != "" {
		return file.PatchID
	}
	return diff.PatchID(file.Patch)
}

// NewRequestDiffHandler returns a ws.MessageHandler that reads diffs from the
// database and returns them as a DIFF_READY message over WebSocket.
//
// DIFF_READY response data:
//
//	{
//	  "pr_id":     "...",
//	  "diff":      "<unified diff>",
//	  "files":     [{path, additions, deletions, content_hash, viewed}...],
//	  "additions": N,
//	  "deletions": N,
//	}
func NewRequestDiffHandler(ds diffReader, log *slog.Logger) ws.MessageHandler {
	deps := DiffHandlerDeps{DiffStore: ds, Log: log}
	return newRequestDiffHandler(deps)
}

// NewRequestDiffHandlerWithDeps creates the handler with full dependencies for
// content_hash and viewed-file enrichment.
func NewRequestDiffHandlerWithDeps(deps DiffHandlerDeps) ws.MessageHandler {
	return newRequestDiffHandler(deps)
}

func newRequestDiffHandler(deps DiffHandlerDeps) ws.MessageHandler {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if deps.Log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var data struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_DIFF payload: %w", err)
		}
		if data.PRID == "" {
			return nil, fmt.Errorf("REQUEST_DIFF: pr_id is required")
		}

		logger.Debug("diff requested", "pr_id", data.PRID)

		unified, err := deps.DiffStore.GetUnified(data.PRID)
		if err != nil {
			logger.Warn("failed to get diff", "pr_id", data.PRID, "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "DIFF_READY",
				"data": map[string]interface{}{
					"pr_id": data.PRID,
					"diff":  "",
					"error": "unable to load diff",
				},
			}, nil
		}

		// Resolve the current user login for viewed-file lookup.
		login := ""
		if deps.CurrentUser != nil {
			if l, ok, err := deps.CurrentUser.Get(ctx, githubProvider); err != nil {
				// Viewed status is optional enrichment, so keep the diff available.
				logger.Warn("failed to resolve current user for viewed status", "error", err)
			} else if ok {
				login = l
			}
		}

		files := []diffFileDetail{}
		var additions, deletions int

		diffFiles, filesErr := deps.DiffStore.Get(data.PRID)
		if filesErr != nil {
			logger.Warn("failed to get diff file stats", "pr_id", data.PRID, "err", filesErr)
		} else {
			for _, f := range diffFiles {
				hash := viewedHash(f)

				viewed := false
				if deps.ViewedStore != nil && login != "" {
					v, verr := deps.ViewedStore.IsFileViewed(ctx, data.PRID, f.FilePath, login, hash)
					if verr != nil {
						logger.Warn("IsFileViewed failed", "file", f.FilePath, "err", verr)
					} else {
						viewed = v
					}
				}

				files = append(files, diffFileDetail{
					Path:        f.FilePath,
					Status:      f.Status,
					OldPath:     f.OldPath,
					IsBinary:    f.IsBinary,
					IsTruncated: f.IsTruncated,
					DiffStatus:  f.DiffStatus,
					PatchBytes:  f.PatchBytes,
					HunkCount:   f.HunkCount,
					Additions:   f.Additions,
					Deletions:   f.Deletions,
					ContentHash: hash,
					Viewed:      viewed,
				})
				additions += f.Additions
				deletions += f.Deletions
			}
		}

		logger.Debug("diff ready",
			"pr_id", data.PRID,
			"bytes", len(unified),
			"files", len(files),
			"additions", additions,
			"deletions", deletions,
		)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "DIFF_READY",
			"data": map[string]interface{}{
				"pr_id":     data.PRID,
				"diff":      unified,
				"files":     files,
				"additions": additions,
				"deletions": deletions,
			},
		}, nil
	}
}

var _ diffReader = (*prdiff.Store)(nil)
