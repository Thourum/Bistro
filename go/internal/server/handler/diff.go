package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// diffReader is the subset of store.DiffStore required by the REQUEST_DIFF handler.
type diffReader interface {
	GetUnifiedDiff(prID string) (string, error)
	GetDiffs(prID string) ([]store.DiffFile, error)
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
// content_hash is SHA-256 of the stored patch (server-computed, canonical).
// viewed is true only when the current user has seen this exact patch.
type diffFileDetail struct {
	Path        string `json:"path"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	ContentHash string `json:"content_hash"`
	Viewed      bool   `json:"viewed"`
}

// diffFileStat is kept for backward-compat in existing tests that check
// the concrete type. New code should use diffFileDetail.
type diffFileStat struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// PatchContentHash computes the canonical content hash for a per-file diff patch.
// Algorithm: SHA-256 of the raw patch string, hex-encoded.
// This is the value that must appear in DIFF_READY content_hash and that clients
// echo back in MARK_FILE_VIEWED content_hash.
func PatchContentHash(patch string) string {
	sum := sha256.Sum256([]byte(patch))
	return hex.EncodeToString(sum[:])
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

		unified, err := deps.DiffStore.GetUnifiedDiff(data.PRID)
		if err != nil {
			logger.Warn("failed to get diff", "pr_id", data.PRID, "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "DIFF_READY",
				"data": map[string]interface{}{
					"pr_id": data.PRID,
					"diff":  "",
					"error": err.Error(),
				},
			}, nil
		}

		// Resolve the current user login for viewed-file lookup.
		login := ""
		if deps.CurrentUser != nil {
			if l, ok, _ := deps.CurrentUser.GetCurrentUser(ctx, "github"); ok {
				login = l
			}
		}

		files := []diffFileDetail{}
		var additions, deletions int

		diffFiles, filesErr := deps.DiffStore.GetDiffs(data.PRID)
		if filesErr != nil {
			logger.Warn("failed to get diff file stats", "pr_id", data.PRID, "err", filesErr)
		} else {
			for _, f := range diffFiles {
				hash := PatchContentHash(f.Patch)

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

var _ diffReader = (*store.DiffStore)(nil)
