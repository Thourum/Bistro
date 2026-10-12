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

// prCheckoutInfoReader is the DB subset required to resolve a PR's repo and
// head SHA for checkout inspection.
type prCheckoutInfoReader interface {
	GetPRCheckoutInfo(ctx context.Context, prID string) (*store.PRCheckoutInfo, error)
}

// repoLocationReader is the DB subset required to resolve a repo's local path.
type repoLocationReader interface {
	Get(ctx context.Context, repoID string) (*store.RepoLocation, error)
}

// prCheckoutInspector inspects a local checkout against a head SHA. Satisfied
// by the git package's InspectPRCheckout free function via a thin adapter, so
// tests can substitute a stub.
type prCheckoutInspector interface {
	InspectPRCheckout(localPath, headSHA string) (git.CheckoutInspection, error)
}

// prCheckoutChecker performs the CHECKOUT_PR_HEAD mutation. Satisfied by
// *git.Client. repoID is passed through so the checkout takes the same
// per-repo lock as sync's fetch/clone paths.
type prCheckoutChecker interface {
	CheckoutPRHead(ctx context.Context, repoID, localPath, headSHA string) error
}

// GitCheckoutInspector adapts the git package's free function to
// prCheckoutInspector so app.go can pass it without a wrapper type.
type GitCheckoutInspector struct{}

// InspectPRCheckout inspects a local checkout without changing it.
func (GitCheckoutInspector) InspectPRCheckout(localPath, headSHA string) (git.CheckoutInspection, error) {
	return git.InspectPRCheckout(localPath, headSHA)
}

// resolvePRCheckoutTarget looks up a PR's repo_id + head_sha, then the repo's
// local_path. Only a user-linked working checkout qualifies (source "local",
// status "ok" — the same contract shouldPreserveLocalPath enforces); Bistro's
// own cache clones are bare and have no worktree to open or check out.
// Returns a user-facing error string (not a Go error) for the "can't proceed"
// cases the WS handlers report as typed failures.
func resolvePRCheckoutTarget(
	ctx context.Context,
	prs prCheckoutInfoReader,
	repos repoLocationReader,
	prID string,
) (repoID, localPath, headSHA string, errReason string) {
	info, err := prs.GetPRCheckoutInfo(ctx, prID)
	if err != nil {
		logging.FromContext(ctx).Warn("PR checkout lookup failed", "error", err)
		return "", "", "", "unable to look up PR"
	}
	if info == nil {
		return "", "", "", "PR not found"
	}

	location, err := repos.Get(ctx, info.RepoID)
	if err != nil {
		logging.FromContext(ctx).Warn("PR checkout location lookup failed", "error", err)
		return "", "", "", "unable to look up repo location"
	}
	if location == nil || location.LocalPath == "" {
		return "", "", "", "no local path"
	}
	if location.Source != "local" || location.LocationStatus != "ok" {
		return "", "", "", "no local checkout linked — link a local checkout for this repo first"
	}

	return info.RepoID, location.LocalPath, info.HeadSHA, ""
}

// NewCheckPRCheckoutHandler returns a WS handler for CHECK_PR_CHECKOUT.
//
// Request: {"pr_id": "..."}
// Response type: PR_CHECKOUT_STATUS
// Response data: {"pr_id": "...", "local_path": "...", "head_sha": "...", "head_matches": bool}
// or {"pr_id": "...", "error": "..."}
func NewCheckPRCheckoutHandler(
	prs prCheckoutInfoReader,
	repos repoLocationReader,
	inspector prCheckoutInspector,
	log *slog.Logger,
) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := log.With("request_id", msg.ID, "msg_type", msg.Type)

		var req struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse CHECK_PR_CHECKOUT: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("CHECK_PR_CHECKOUT requires pr_id")
		}

		_, localPath, headSHA, reason := resolvePRCheckoutTarget(ctx, prs, repos, req.PRID)
		if reason != "" {
			logger.Warn("CHECK_PR_CHECKOUT failed", "pr_id", req.PRID, "reason", reason)
			return checkoutStatusResponse(msg.ID, req.PRID, "", "", false, reason), nil
		}

		inspection, err := inspector.InspectPRCheckout(localPath, headSHA)
		if err != nil {
			logger.Warn("CHECK_PR_CHECKOUT inspection failed", "pr_id", req.PRID, "error", err)
			return checkoutStatusResponse(msg.ID, req.PRID, localPath, headSHA, false, "unable to inspect checkout"), nil
		}

		return checkoutStatusResponse(msg.ID, req.PRID, localPath, headSHA, inspection.HeadMatches, ""), nil
	}
}

// NewCheckoutPRHeadHandler returns a WS handler for CHECKOUT_PR_HEAD.
//
// Request: {"pr_id": "...", "expected_head_sha"?: "..."} — when present, the
// checkout is refused if the PR's head has moved since the client inspected
// it (force-push race), so the user re-inspects instead of landing on a
// revision they never saw.
// Response type: PR_CHECKOUT_RESULT
// Response data: {"pr_id": "...", "ok": bool, "local_path"?: "...", "reason"?: "..."}
func NewCheckoutPRHeadHandler(
	prs prCheckoutInfoReader,
	repos repoLocationReader,
	checker prCheckoutChecker,
	log *slog.Logger,
) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := log.With("request_id", msg.ID, "msg_type", msg.Type)

		var req struct {
			PRID            string `json:"pr_id"`
			ExpectedHeadSHA string `json:"expected_head_sha"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse CHECKOUT_PR_HEAD: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("CHECKOUT_PR_HEAD requires pr_id")
		}

		repoID, localPath, headSHA, reason := resolvePRCheckoutTarget(ctx, prs, repos, req.PRID)
		if reason != "" {
			logger.Warn("CHECKOUT_PR_HEAD failed", "pr_id", req.PRID, "reason", reason)
			return checkoutResultResponse(msg.ID, req.PRID, "", false, reason), nil
		}
		if req.ExpectedHeadSHA != "" && req.ExpectedHeadSHA != headSHA {
			logger.Warn("CHECKOUT_PR_HEAD refused: head moved", "pr_id", req.PRID, "expected", req.ExpectedHeadSHA, "current", headSHA)
			return checkoutResultResponse(msg.ID, req.PRID, localPath, false, "PR head changed since it was inspected; try again"), nil
		}

		if err := checker.CheckoutPRHead(ctx, repoID, localPath, headSHA); err != nil {
			reason := "unable to check out PR head"
			if errors.Is(err, git.ErrWorktreeDirty) {
				reason = "worktree dirty"
			}
			logger.Warn("CHECKOUT_PR_HEAD checkout failed", "pr_id", req.PRID, "error", err)
			return checkoutResultResponse(msg.ID, req.PRID, localPath, false, reason), nil
		}

		logger.Debug("checked out PR head", "pr_id", req.PRID, "local_path", localPath, "head_sha", headSHA)
		return checkoutResultResponse(msg.ID, req.PRID, localPath, true, ""), nil
	}
}

func checkoutStatusResponse(msgID, prID, localPath, headSHA string, headMatches bool, reason string) interface{} {
	data := map[string]interface{}{
		"pr_id":        prID,
		"local_path":   localPath,
		"head_sha":     headSHA,
		"head_matches": headMatches,
	}
	if reason != "" {
		data["error"] = reason
	}
	return map[string]interface{}{
		"id":   msgID,
		"type": "PR_CHECKOUT_STATUS",
		"data": data,
	}
}

func checkoutResultResponse(msgID, prID, localPath string, ok bool, reason string) interface{} {
	data := map[string]interface{}{
		"pr_id": prID,
		"ok":    ok,
	}
	if localPath != "" {
		data["local_path"] = localPath
	}
	if reason != "" {
		data["reason"] = reason
	}
	return map[string]interface{}{
		"id":   msgID,
		"type": "PR_CHECKOUT_RESULT",
		"data": data,
	}
}
