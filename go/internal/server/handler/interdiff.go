package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type interdiffPatchsets interface {
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
	CurrentPatchset(ctx context.Context, prID string) (store.Patchset, bool, error)
}

type interdiffer interface {
	Interdiff(req git.InterdiffRequest) (git.InterdiffResult, error)
}

// InterdiffHandlerDeps bundles REQUEST_INTERDIFF dependencies.
type InterdiffHandlerDeps struct {
	Patchsets interdiffPatchsets
	Git       interdiffer
	// RepoForPR returns the sync's clone URL and the on-disk repo for a PR;
	// ok=false when no repo is recorded yet.
	RepoForPR func(ctx context.Context, prID string) (repoURL, repoPath string, ok bool, err error)
	Log       *slog.Logger
}

type interdiffRequest struct {
	PRID         string `json:"pr_id"`
	Path         string `json:"path"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
}

type interdiffResponse struct {
	PRID         string `json:"pr_id"`
	Path         string `json:"path"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason"`
	Patch        string `json:"patch"`
	Rebased      bool   `json:"rebased"`
}

// NewInterdiffHandler returns a WS handler for REQUEST_INTERDIFF: one file at
// the from-patchset head vs. the to-patchset head.
func NewInterdiffHandler(deps InterdiffHandlerDeps) ws.MessageHandler {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req interdiffRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse REQUEST_INTERDIFF: %w", err)
		}
		if req.PRID == "" || req.Path == "" || req.FromPatchset <= 0 {
			return nil, fmt.Errorf("REQUEST_INTERDIFF requires pr_id, path, and from_patchset")
		}
		logger := log.With("request_id", msg.ID, "msg_type", msg.Type, "pr_id", req.PRID)
		response := buildInterdiff(logging.WithLogger(ctx, logger), deps, req)
		return map[string]interface{}{"id": msg.ID, "type": "INTERDIFF_READY", "data": response}, nil
	}
}

func buildInterdiff(ctx context.Context, deps InterdiffHandlerDeps, req interdiffRequest) interdiffResponse {
	logger := logging.FromContext(ctx)
	response := interdiffResponse{PRID: req.PRID, Path: req.Path, FromPatchset: req.FromPatchset, ToPatchset: req.ToPatchset}
	patchsets, err := deps.Patchsets.ListPatchsets(ctx, req.PRID)
	if err != nil {
		logger.Warn("interdiff: list patchsets failed", "error", err)
		return unavailableInterdiff(response, "could not load patchsets")
	}
	from := patchsetByNumber(patchsets, req.FromPatchset)
	to := patchsetByNumber(patchsets, req.ToPatchset)
	if req.ToPatchset == 0 && len(patchsets) > 0 {
		to = &patchsets[len(patchsets)-1]
		current, found, err := deps.Patchsets.CurrentPatchset(ctx, req.PRID)
		if err != nil {
			logger.Warn("interdiff: current patchset lookup failed", "error", err)
			return unavailableInterdiff(response, "could not load patchsets")
		}
		if found {
			to = &current
		}
	}
	if from == nil || to == nil {
		return unavailableInterdiff(response, "patchset is not recorded")
	}
	response.ToPatchset = to.Number
	repoURL, repoPath, ok, err := deps.RepoForPR(ctx, req.PRID)
	if err != nil {
		logger.Warn("interdiff: repo lookup failed", "error", err)
		return unavailableInterdiff(response, "could not locate the repository")
	}
	if !ok {
		return unavailableInterdiff(response, "no local copy of this repository yet")
	}
	result, err := deps.Git.Interdiff(git.InterdiffRequest{
		RepoURL: repoURL, RepoPath: repoPath, Path: req.Path,
		FromHead: from.HeadSHA, ToHead: to.HeadSHA,
		FromMergeBase: from.MergeBaseSHA, ToMergeBase: to.MergeBaseSHA,
	})
	if errors.Is(err, git.ErrHeadUnavailable) {
		return unavailableInterdiff(response, fmt.Sprintf("could not load a patchset head (P%d or P%d): %s", from.Number, to.Number, err))
	}
	if err != nil {
		logger.Warn("interdiff failed", "path", req.Path, "error", err)
		return unavailableInterdiff(response, "could not compute the interdiff")
	}
	response.Available, response.Patch, response.Rebased = true, result.Patch, result.Rebased
	return response
}

func unavailableInterdiff(response interdiffResponse, reason string) interdiffResponse {
	response.Reason = reason
	return response
}
