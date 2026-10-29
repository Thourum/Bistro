package services

import (
	"context"
	"log/slog"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/store"
)

type threadLister interface {
	ListReviewThreads(context.Context, string) ([]store.ReviewThreadRow, error)
}

// ActionApplier executes review actions through the live orchestrator.
type ActionApplier interface {
	ApplyAction(context.Context, *actions.ActionRequest) *actions.ActionResult
}

// ApplierGetter avoids retaining an orchestrator after provider replacement.
type ApplierGetter func() ActionApplier

type Threads struct {
	DB         threadLister
	GetApplier ApplierGetter
	Panics     PanicReporter
	Log        *slog.Logger
}
type ThreadsResponse struct {
	PRID    string                  `json:"pr_id"`
	Threads []store.ReviewThreadRow `json:"threads"`
}
type ActionRequest struct {
	Action      string `json:"action"`
	PRID        string `json:"pr_id"`
	Body        string `json:"body"`
	ThreadID    string `json:"thread_id"`
	CommentID   string `json:"comment_id"`
	InReplyTo   string `json:"in_reply_to"`
	Line        int    `json:"line"`
	ReviewEvent string `json:"review_event"`
}
type ActionResult struct {
	PRID   string `json:"pr_id"`
	Action string `json:"action"`
}

// List includes unknown thread states so the UI can disable resolution for them.
func (s *Threads) List(ctx context.Context, req PRRef) (ThreadsResponse, error) {
	return guard(s.Panics, "Threads.List", func() (ThreadsResponse, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return ThreadsResponse{}, Errorf(CodeInvalid, "pr_id is required")
		}
		threads, err := s.DB.ListReviewThreads(ctx, prID)
		if err != nil {
			return ThreadsResponse{}, err
		}
		if threads == nil {
			threads = []store.ReviewThreadRow{}
		}
		return ThreadsResponse{PRID: prID, Threads: threads}, nil
	})
}

// SubmitAction preserves aliases and delegates draft absorption to the orchestrator.
func (s *Threads) SubmitAction(ctx context.Context, req ActionRequest) (ActionResult, error) {
	return guard(s.Panics, "Threads.SubmitAction", func() (ActionResult, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" || req.Action == "" {
			return ActionResult{}, Errorf(CodeInvalid, "pr_id and action are required")
		}
		body := req.Body
		if (req.Action == string(actions.ResolveThreadCommand) || req.Action == string(actions.UnresolveThreadCommand)) && req.ThreadID != "" {
			body = req.ThreadID
		}
		inReplyTo := req.InReplyTo
		if inReplyTo == "" {
			inReplyTo = req.CommentID
		}
		actionReq := &actions.ActionRequest{PRID: prID, Action: actions.ActionCommand(req.Action), Body: body, Line: req.Line, InReplyTo: inReplyTo, ThreadID: req.ThreadID, ReviewEvent: req.ReviewEvent}
		if err := actionReq.Validate(); err != nil {
			return ActionResult{}, Errorf(CodeInvalid, "%s", err)
		}
		if err := ctx.Err(); err != nil {
			return ActionResult{}, err
		}
		if s.GetApplier == nil {
			return ActionResult{}, Errorf(CodeUnauthenticated, "no authenticated provider")
		}
		applier := s.GetApplier()
		if applier == nil {
			return ActionResult{}, Errorf(CodeUnauthenticated, "no authenticated provider")
		}
		result := applier.ApplyAction(ctx, actionReq)
		if result == nil {
			return ActionResult{}, WithReason(CodeInternal, "action_failed", "action returned no result")
		}
		if !result.Success {
			if err := ctx.Err(); err != nil {
				return ActionResult{}, err
			}
			return ActionResult{}, WithReason(CodeInternal, "action_failed", result.Error)
		}
		return ActionResult{PRID: result.PRID, Action: string(result.Action)}, nil
	})
}
