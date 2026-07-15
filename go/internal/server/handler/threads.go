package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// threadLister can list review threads for a PR from SQLite.
type threadLister interface {
	ListReviewThreads(ctx context.Context, prID string) ([]store.ReviewThreadRow, error)
}

// ActionApplier can apply an action request (approve, resolve_thread, etc.).
// Exported so app.go can satisfy it via the orchestrator.
type ActionApplier interface {
	ApplyAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult
}

// ApplierGetter is a function that returns the current ActionApplier (may be
// nil when no provider is authenticated). Using a getter instead of a direct
// reference ensures SUBMIT_ACTION always routes to the live orchestrator even
// after an auth/provider replacement.
type ApplierGetter func() ActionApplier

// NewGetReviewThreadsHandler returns a WS handler for GET_REVIEW_THREADS.
//
// Request: {"pr_id": "github:owner/repo:N"}
// Response type: REVIEW_THREADS_READY
// Response data:
//
//	{
//	  "pr_id": "...",
//	  "threads": [
//	    {
//	      "id":          "<thread node id>",
//	      "pr_id":       "...",
//	      "file_path":   "...",
//	      "line":        42,
//	      "resolved":    false,
//	      "outdated":    false,
//	      "state_known": true
//	    }
//	  ]
//	}
//
// Threads with state_known=false are included so the UI can show an "unknown"
// badge. resolve/unresolve actions are disabled by the UI for those threads.
func NewGetReviewThreadsHandler(db threadLister, log *slog.Logger) ws.MessageHandler {
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
			return nil, fmt.Errorf("parse GET_REVIEW_THREADS: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("GET_REVIEW_THREADS requires pr_id")
		}

		threads, err := db.ListReviewThreads(ctx, req.PRID)
		if err != nil {
			logger.Error("GET_REVIEW_THREADS query failed", "pr_id", req.PRID, "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "REVIEW_THREADS_READY",
				"data": map[string]interface{}{
					"pr_id":   req.PRID,
					"threads": []store.ReviewThreadRow{},
					"error":   err.Error(),
				},
			}, nil
		}
		if threads == nil {
			threads = []store.ReviewThreadRow{}
		}

		logger.Debug("review threads ready", "pr_id", req.PRID, "count", len(threads))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "REVIEW_THREADS_READY",
			"data": map[string]interface{}{
				"pr_id":   req.PRID,
				"threads": threads,
			},
		}, nil
	}
}

// NewSubmitActionHandler returns a WS handler for SUBMIT_ACTION.
// This is the generic action submission endpoint for all action types including
// resolve_thread and unresolve_thread.
//
// getApplier is called on every request so the handler always uses the current
// orchestrator, not a potentially stale one captured at registration time.
//
// Request:
//
//	{
//	  "action":    "resolve_thread" | "unresolve_thread" | "approve" | ...,
//	  "pr_id":     "github:owner/repo:N",
//	  "body":      "<thread node id or comment body>",
//	  "thread_id": "<thread node id>"   // alias for body on thread actions
//	}
//
// Response type: ACTION_RESULT
// Response data:
//
//	{
//	  "pr_id":   "...",
//	  "action":  "resolve_thread",
//	  "success": true,
//	  "error":   ""
//	}
func NewSubmitActionHandler(getApplier ApplierGetter, log *slog.Logger) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		applier := getApplier()
		if applier == nil {
			return nil, fmt.Errorf("SUBMIT_ACTION unavailable: no authenticated provider")
		}

		var req struct {
			Action    string `json:"action"`
			PRID      string `json:"pr_id"`
			Body      string `json:"body"`
			ThreadID  string `json:"thread_id"` // alias: resolve/unresolve actions use this field
			CommentID string `json:"comment_id"`
			InReplyTo string `json:"in_reply_to"`
			Line      int    `json:"line"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse SUBMIT_ACTION: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("SUBMIT_ACTION requires pr_id")
		}
		if req.Action == "" {
			return nil, fmt.Errorf("SUBMIT_ACTION requires action")
		}

		// For resolve/unresolve actions, thread_id is the canonical field and takes priority.
		body := req.Body
		if (req.Action == string(actions.ResolveThreadCommand) || req.Action == string(actions.UnresolveThreadCommand)) && req.ThreadID != "" {
			body = req.ThreadID
		}

		inReplyTo := req.InReplyTo
		if inReplyTo == "" {
			inReplyTo = req.CommentID
		}

		actionReq := &actions.ActionRequest{
			PRID:      req.PRID,
			Action:    actions.ActionCommand(req.Action),
			Body:      body,
			Line:      req.Line,
			InReplyTo: inReplyTo,
			ThreadID:  req.ThreadID,
		}

		if err := actionReq.Validate(); err != nil {
			return nil, fmt.Errorf("SUBMIT_ACTION validation: %w", err)
		}

		logger.Debug("submitting action", "pr_id", req.PRID, "action", req.Action)

		result := applier.ApplyAction(ctx, actionReq)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "ACTION_RESULT",
			"data": map[string]interface{}{
				"pr_id":   result.PRID,
				"action":  string(result.Action),
				"success": result.Success,
				"error":   result.Error,
			},
		}, nil
	}
}
