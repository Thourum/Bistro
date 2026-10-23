package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// patchsetLister is the DB subset required by NewListPatchsetsHandler.
type patchsetLister interface {
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
}

// patchsetByNumber finds a patchset by its number in an ordered slice.
func patchsetByNumber(patchsets []store.Patchset, number int) *store.Patchset {
	for i := range patchsets {
		if patchsets[i].Number == number {
			return &patchsets[i]
		}
	}
	return nil
}

// fileViewedMarker is the DB subset required by NewMarkFileViewedHandler.
type fileViewedMarker interface {
	MarkFileViewed(ctx context.Context, prID, filePath, login, contentHash string) error
}

// currentUserReader is the subset used to resolve the authenticated login.
type currentUserReader interface {
	GetCurrentUser(ctx context.Context, provider string) (string, bool, error)
}

// NewListPatchsetsHandler returns a WS handler for GET_PATCHSETS.
//
// Request: {"pr_id": "github:owner/repo:N"}
// Response type: PATCHSETS_READY
// Response data: {"pr_id": "...", "patchsets": [{pr_id, number, head_sha, base_sha, pushed_at}...]}
func NewListPatchsetsHandler(ps patchsetLister, log *slog.Logger) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse GET_PATCHSETS: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("GET_PATCHSETS requires pr_id")
		}

		patchsets, err := ps.ListPatchsets(ctx, req.PRID)
		if err != nil {
			logger.Error("list patchsets failed", "pr_id", req.PRID, "error", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "PATCHSETS_READY",
				"data": map[string]interface{}{
					"pr_id":     req.PRID,
					"patchsets": []store.Patchset{},
					"error":     "unable to load patchsets",
				},
			}, nil
		}
		if patchsets == nil {
			patchsets = []store.Patchset{}
		}

		logger.Debug("patchsets ready", "pr_id", req.PRID, "count", len(patchsets))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "PATCHSETS_READY",
			"data": map[string]interface{}{
				"pr_id":     req.PRID,
				"patchsets": patchsets,
			},
		}, nil
	}
}

// NewMarkFileViewedHandler returns a WS handler for MARK_FILE_VIEWED.
//
// Request: {"pr_id": "...", "file_path": "...", "content_hash": "..."}
// Response type: FILE_VIEWED_ACK
// Response data: {"pr_id": "...", "file_path": "...", "ok": true/false, "reason"?: "..."}
//
// The handler validates the client-supplied content_hash against the server's
// canonical hash (SHA-256 of the stored patch). Mismatched hashes are rejected.
// Use NewMarkFileViewedHandlerWithDiffStore to enable hash validation.
func NewMarkFileViewedHandler(db fileViewedMarker, cu currentUserReader, log *slog.Logger) ws.MessageHandler {
	return newMarkFileViewedHandler(db, cu, nil, log)
}

// NewMarkFileViewedHandlerWithDiffStore creates the handler with a DiffStore for
// server-side hash validation. The client-supplied content_hash is checked against
// SHA-256(stored patch); mismatched hashes are rejected with ok=false.
func NewMarkFileViewedHandlerWithDiffStore(db fileViewedMarker, cu currentUserReader, ds diffReader, log *slog.Logger) ws.MessageHandler {
	return newMarkFileViewedHandler(db, cu, ds, log)
}

func newMarkFileViewedHandler(db fileViewedMarker, cu currentUserReader, ds diffReader, log *slog.Logger) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req struct {
			PRID        string `json:"pr_id"`
			FilePath    string `json:"file_path"`
			ContentHash string `json:"content_hash"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse MARK_FILE_VIEWED: %w", err)
		}
		if req.PRID == "" || req.FilePath == "" || req.ContentHash == "" {
			return nil, fmt.Errorf("MARK_FILE_VIEWED requires pr_id, file_path, and content_hash")
		}

		login, ok, lookupErr := cu.GetCurrentUser(ctx, githubProvider)
		if lookupErr != nil {
			logger.Warn("MARK_FILE_VIEWED: failed to resolve current user", "error", lookupErr)
			return fileViewedACK(msg.ID, req.PRID, req.FilePath, false, "unable to check authenticated user"), nil
		}
		if !ok || login == "" {
			logger.Warn("MARK_FILE_VIEWED: no current user, skipping",
				"pr_id", req.PRID, "file_path", req.FilePath)
			return fileViewedACK(msg.ID, req.PRID, req.FilePath, false, "no authenticated user"), nil
		}

		// Server-side hash validation when DiffStore is available.
		if ds != nil {
			if valid, reason := validateFileHash(logging.WithLogger(ctx, logger), ds, req.PRID, req.FilePath, req.ContentHash); !valid {
				logger.Warn("MARK_FILE_VIEWED: hash validation failed",
					"pr_id", req.PRID, "file_path", req.FilePath, "reason", reason)
				return fileViewedACK(msg.ID, req.PRID, req.FilePath, false, reason), nil
			}
		}

		if err := db.MarkFileViewed(ctx, req.PRID, req.FilePath, login, req.ContentHash); err != nil {
			logger.Error("mark file viewed failed",
				"pr_id", req.PRID, "file_path", req.FilePath, "error", err)
			return fileViewedACK(msg.ID, req.PRID, req.FilePath, false, "unable to mark file viewed"), nil
		}

		logger.Debug("file viewed marked",
			"pr_id", req.PRID, "file_path", req.FilePath, "login", login)

		return fileViewedACK(msg.ID, req.PRID, req.FilePath, true, ""), nil
	}
}

// validateFileHash computes the canonical content hash for filePath in the
// stored diffs and checks it against the client-supplied hash.
// Fails closed: returns (false, reason) when GetDiffs errors or the file is
// not found in stored diffs, so stale hashes cannot be accepted silently.
func validateFileHash(ctx context.Context, ds diffReader, prID, filePath, clientHash string) (bool, string) {
	diffs, err := ds.GetDiffs(prID)
	if err != nil {
		logging.FromContext(ctx).Warn("content hash validation diff lookup failed", "pr_id", prID, "error", err)
		return false, "could not validate content_hash"
	}
	for _, f := range diffs {
		if f.FilePath == filePath {
			canonical := PatchContentHash(f.Patch)
			if canonical != clientHash {
				prefix := func(s string) string {
					if len(s) > 8 {
						return s[:8]
					}
					return s
				}
				return false, fmt.Sprintf(
					"content_hash mismatch: client %s… server %s… (file content may have changed)",
					prefix(clientHash), prefix(canonical))
			}
			return true, ""
		}
	}
	// File not found in stored diffs → fail closed; cannot verify.
	return false, fmt.Sprintf("could not validate content_hash: file %q not found in stored diffs", filePath)
}

func fileViewedACK(msgID, prID, filePath string, ok bool, reason string) interface{} {
	data := map[string]interface{}{
		"pr_id":     prID,
		"file_path": filePath,
		"ok":        ok,
	}
	if reason != "" {
		data["reason"] = reason
	}
	return map[string]interface{}{
		"id":   msgID,
		"type": "FILE_VIEWED_ACK",
		"data": data,
	}
}
