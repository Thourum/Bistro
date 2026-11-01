package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/logging"
)

type Interdiff struct {
	Patchsets interdiffPatchsets
	Interdiff func(diff.InterdiffRequest) (diff.InterdiffResult, error)
	RepoForPR func(context.Context, string) (string, string, bool, error)
	Panics    PanicReporter
	Log       *slog.Logger
}
type InterdiffRequest struct {
	PRID         string `json:"pr_id"`
	Path         string `json:"path"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
}

type InterdiffResponse struct {
	PRID         string `json:"pr_id"`
	Path         string `json:"path"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason"`
	Patch        string `json:"patch"`
	Rebased      bool   `json:"rebased"`
}

// Get computes the head-to-head diff for one file.
func (s *Interdiff) Get(ctx context.Context, req InterdiffRequest) (InterdiffResponse, error) {
	return guard(s.Panics, "Interdiff.Get", func() (InterdiffResponse, error) {
		req.PRID = NormalizeID(req.PRID)
		if req.PRID == "" || req.Path == "" || req.FromPatchset <= 0 || req.ToPatchset < 0 {
			return InterdiffResponse{}, Errorf(CodeInvalid, "pr_id, path, and positive from_patchset are required")
		}
		return buildInterdiff(logging.WithLogger(ctx, serviceLogger(s.Log)), s, req)
	})
}
func buildInterdiff(ctx context.Context, deps *Interdiff, req InterdiffRequest) (InterdiffResponse, error) {
	logger := logging.FromContext(ctx)
	response := InterdiffResponse{PRID: req.PRID, Path: req.Path, FromPatchset: req.FromPatchset, ToPatchset: req.ToPatchset}
	repoURL, repoPath, ok, repoErr := deps.RepoForPR(ctx, req.PRID)
	if err := contextError(repoErr); err != nil {
		return response, err
	}
	if errors.Is(repoErr, sql.ErrNoRows) {
		return response, Errorf(CodeNotFound, "PR not found")
	}
	patchsets, err := deps.Patchsets.ListPatchsets(ctx, req.PRID)
	if err != nil {
		if contextError(err) == nil {
			logger.Warn("interdiff: list patchsets failed", "error", err)
		}
		return unavailableInterdiff(response, "could not load patchsets"), contextError(err)
	}
	from := patchsetByNumber(patchsets, req.FromPatchset)
	to := patchsetByNumber(patchsets, req.ToPatchset)
	if req.ToPatchset == 0 && len(patchsets) > 0 {
		to = &patchsets[len(patchsets)-1]
		current, found, err := deps.Patchsets.CurrentPatchset(ctx, req.PRID)
		if err != nil {
			if contextError(err) == nil {
				logger.Warn("interdiff: current patchset lookup failed", "error", err)
			}
			return unavailableInterdiff(response, "could not load patchsets"), contextError(err)
		}
		if found {
			to = &current
		}
	}
	if from == nil || to == nil {
		return unavailableInterdiff(response, "patchset is not recorded"), nil
	}
	response.ToPatchset = to.Number
	if repoErr != nil {
		logger.Warn("interdiff: repo lookup failed", "error", repoErr)
		return unavailableInterdiff(response, "could not locate the repository"), nil
	}
	if !ok {
		return unavailableInterdiff(response, "no local copy of this repository yet"), nil
	}
	result, err := deps.Interdiff(diff.InterdiffRequest{
		RepoURL: repoURL, RepoPath: repoPath, Path: req.Path,
		FromHead: from.HeadSHA, ToHead: to.HeadSHA,
		FromMergeBase: from.MergeBaseSHA, ToMergeBase: to.MergeBaseSHA,
	})
	if errors.Is(err, diff.ErrHeadUnavailable) {
		return unavailableInterdiff(response, fmt.Sprintf("could not load a patchset head (P%d or P%d): %s", from.Number, to.Number, err)), nil
	}
	if err != nil {
		if contextError(err) == nil {
			logger.Warn("interdiff failed", "path", req.Path, "error", err)
		}
		return unavailableInterdiff(response, "could not compute the interdiff"), contextError(err)
	}
	response.Available, response.Patch, response.Rebased = true, result.Patch, result.Rebased
	return response, nil
}

func unavailableInterdiff(response InterdiffResponse, reason string) InterdiffResponse {
	response.Reason = reason
	return response
}
