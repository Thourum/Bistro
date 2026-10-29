package services

import (
	"context"
	"errors"
	"log/slog"

	"github.com/itterno/bistro/internal/git"
)

const (
	repoLocationSourceLocal  = "local"
	repoLocationStatusOK     = "ok"
	checkoutPRNotFound       = "PR not found"
	checkoutHeadMissing      = "PR head SHA missing"
	checkoutNoLocalPath      = "no local path"
	checkoutNotLinked        = "no local checkout linked — link a local checkout for this repo first"
	checkoutInspectionFailed = "unable to inspect checkout"
	checkoutHeadMoved        = "PR head changed since it was inspected; try again"
	checkoutFailed           = "unable to check out PR head"
	checkoutWorktreeDirty    = "worktree dirty"
)

// Editor inspects and checks out PR heads in user-linked working copies.
type Editor struct {
	PRs       prReader
	Repos     repoLocationReader
	Inspector prCheckoutInspector
	Checker   prCheckoutChecker
	Panics    PanicReporter
	Log       *slog.Logger
}

type CheckoutStatus struct {
	PRID        string `json:"pr_id"`
	LocalPath   string `json:"local_path"`
	HeadSHA     string `json:"head_sha"`
	HeadMatches bool   `json:"head_matches"`
}
type CheckoutHeadRequest struct {
	PRID            string `json:"pr_id"`
	ExpectedHeadSHA string `json:"expected_head_sha"`
}
type CheckoutResult struct {
	PRID      string `json:"pr_id"`
	LocalPath string `json:"local_path"`
}

type checkoutTarget struct {
	repoID    string
	localPath string
	headSHA   string
}

// GitCheckoutInspector adapts the git package's read-only checkout inspection.
type GitCheckoutInspector struct{}

// InspectPRCheckout does not modify the working copy.
func (GitCheckoutInspector) InspectPRCheckout(localPath, headSHA string) (git.CheckoutInspection, error) {
	return git.InspectPRCheckout(localPath, headSHA)
}

func (s *Editor) resolveTarget(ctx context.Context, prID string) (checkoutTarget, error) {
	info, err := s.PRs.GetPRCheckoutInfo(ctx, prID)
	if err != nil {
		return checkoutTarget{}, err
	}
	if info == nil {
		return checkoutTarget{}, WithReason(CodeNotFound, checkoutPRNotFound, checkoutPRNotFound)
	}
	if info.HeadSHA == "" {
		return checkoutTarget{}, WithReason(CodeInvalid, checkoutHeadMissing, checkoutHeadMissing)
	}
	location, err := s.Repos.Get(ctx, info.RepoID)
	if err != nil {
		return checkoutTarget{}, err
	}
	if location == nil || location.LocalPath == "" {
		return checkoutTarget{}, WithReason(CodeInvalid, checkoutNoLocalPath, checkoutNoLocalPath)
	}
	if location.Source != repoLocationSourceLocal || location.LocationStatus != repoLocationStatusOK {
		return checkoutTarget{}, WithReason(CodeInvalid, checkoutNotLinked, checkoutNotLinked)
	}
	return checkoutTarget{repoID: info.RepoID, localPath: location.LocalPath, headSHA: info.HeadSHA}, nil
}

// CheckoutStatus compares the linked checkout's HEAD with the current cached PR head.
func (s *Editor) CheckoutStatus(ctx context.Context, req PRRef) (CheckoutStatus, error) {
	return guard(s.Panics, "Editor.CheckoutStatus", func() (CheckoutStatus, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return CheckoutStatus{}, Errorf(CodeInvalid, "pr_id is required")
		}
		target, err := s.resolveTarget(ctx, prID)
		if err != nil {
			return CheckoutStatus{}, err
		}
		inspection, err := s.Inspector.InspectPRCheckout(target.localPath, target.headSHA)
		if err != nil {
			if contextError(err) != nil {
				return CheckoutStatus{}, err
			}
			return CheckoutStatus{}, WithReason(CodeInvalid, checkoutInspectionFailed, checkoutInspectionFailed)
		}
		return CheckoutStatus{PRID: prID, LocalPath: target.localPath, HeadSHA: target.headSHA, HeadMatches: inspection.HeadMatches}, nil
	})
}

// CheckoutHead re-resolves the target so a force-push cannot silently change the requested checkout.
func (s *Editor) CheckoutHead(ctx context.Context, req CheckoutHeadRequest) (CheckoutResult, error) {
	return guard(s.Panics, "Editor.CheckoutHead", func() (CheckoutResult, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return CheckoutResult{}, Errorf(CodeInvalid, "pr_id is required")
		}
		target, err := s.resolveTarget(ctx, prID)
		if err != nil {
			return CheckoutResult{}, err
		}
		if req.ExpectedHeadSHA != "" && req.ExpectedHeadSHA != target.headSHA {
			return CheckoutResult{}, WithReason(CodeInvalid, checkoutHeadMoved, checkoutHeadMoved)
		}
		if err := s.Checker.CheckoutPRHead(ctx, target.repoID, target.localPath, target.headSHA); err != nil {
			if contextError(err) != nil {
				return CheckoutResult{}, err
			}
			reason := checkoutFailed
			if errors.Is(err, git.ErrWorktreeDirty) {
				reason = checkoutWorktreeDirty
			}
			return CheckoutResult{}, WithReason(CodeInvalid, reason, reason)
		}
		return CheckoutResult{PRID: prID, LocalPath: target.localPath}, nil
	})
}
