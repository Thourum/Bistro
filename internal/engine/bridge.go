package engine

import (
	"context"
	"fmt"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/services"
)

func bridgeEvent(ev events.Event) (name string, payload any, ok bool) {
	d := eventData(ev.Data)
	switch ev.Type {
	case "rate_limited":
		return services.EventRateLimited, services.RateLimited{ResetIn: d.float("reset_in")}, true
	case "PR_METADATA_CHANGED":
		return services.EventPRMetadataChanged, services.PRMetadataChanged{PRID: d.str("pr_id"), Owner: d.str("owner"), Repo: d.str("repo"), Number: d.int("number"), Field: d.str("field"), Old: d.str("old"), New: d.str("new")}, true
	case "CI_STATUS_CHANGED":
		return services.EventCIStatusChanged, services.CIStatusChanged{PRID: d.str("pr_id"), Owner: d.str("owner"), Repo: d.str("repo"), Number: d.int("number"), OldStatus: d.str("old_status"), NewStatus: d.str("new_status")}, true
	case "THREAD_RESOLVED", "THREAD_REOPENED":
		return services.EventThreadStateChanged, services.ThreadStateChanged{ThreadID: d.str("thread_id"), PRID: d.str("pr_id"), Resolved: ev.Type == "THREAD_RESOLVED"}, true
	case "COMMENT_POSTED":
		return services.EventCommentPosted, services.CommentPosted{PRID: d.str("pr_id"), ThreadID: d.str("thread_id"), InReplyTo: d.str("in_reply_to")}, true
	case "DRAFTS_CHANGED":
		return services.EventDraftsChanged, services.DraftsChanged{PRID: d.str("pr_id")}, true
	case "action_complete":
		success, _ := d["success"].(bool)
		return services.EventActionComplete, services.ActionComplete{PR: d.str("pr"), Action: d.str("action"), Success: success, Error: d.str("error")}, true
	case "pr_approved":
		return services.EventPRApproved, services.PRApproved{PRID: d.str("pr_id")}, true
	}
	return "", nil, false
}

type eventData map[string]any

func (d eventData) str(key string) string {
	value, ok := d[key]
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}

func (d eventData) int(key string) int {
	switch value := d[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

func (d eventData) float(key string) float64 {
	switch value := d[key].(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	}
	return 0
}

// Must be called with a.provMu held for writing (or before the App is shared).
func (a *App) startEventBridge(parent context.Context) {
	if a.bridgeCancel != nil {
		a.bridgeCancel()
	}
	if a.orchestrator == nil {
		a.bridgeCancel = nil
		return
	}
	ctx, cancel := context.WithCancel(parent)
	a.bridgeCancel = cancel
	orchestrator := a.orchestrator
	ready := make(chan struct{})
	subscribed := false
	a.startLoop(ctx, "event-bridge", func(ctx context.Context) {
		ch := orchestrator.GetEventChannel()
		defer orchestrator.UnsubscribeEventChannel(ch)
		if !subscribed {
			subscribed = true
			close(ready)
		}
		a.bridgeEvents(ctx, ch)
	})
	// Startup replay may emit immediately; wait for the first subscription.
	select {
	case <-ready:
	case <-ctx.Done():
	}
}

func (a *App) bridgeEvents(ctx context.Context, ch <-chan *events.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ctx.Err() != nil {
				return
			}
			if name, payload, ok := bridgeEvent(*ev); ok {
				a.emitter.Emit(name, payload)
			}
		}
	}
}
