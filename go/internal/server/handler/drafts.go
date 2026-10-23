package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// maxDraftBodyLength matches GitHub's 65,536-character comment body limit.
const maxDraftBodyLength = 65536

// draftStore is the subset of store.ReviewDrafts the draft handlers use.
type draftStore interface {
	Create(ctx context.Context, login string, draft store.ReviewDraft) (store.ReviewDraft, error)
	UpdateBody(ctx context.Context, login, id, body string) (store.ReviewDraft, error)
	List(ctx context.Context, prID, login string) ([]store.ReviewDraft, error)
	Delete(ctx context.Context, login string, ids []string) error
}

// DraftDeps carries the draft handlers' dependencies.
type DraftDeps struct {
	Drafts      draftStore
	CurrentUser currentUserReader
	Broadcaster broadcaster
	Log         *slog.Logger
}

type draftSaveRequest struct {
	PRID      string `json:"pr_id"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line"`
	Line      int    `json:"line"`
	Body      string `json:"body"`
	CommitSHA string `json:"commit_sha"`
}

func (r draftSaveRequest) validate() error {
	body := strings.TrimSpace(r.Body)
	switch {
	case r.PRID == "":
		return errors.New("pr_id is required")
	case body == "":
		return errors.New("body is required")
	case len(body) > maxDraftBodyLength:
		return fmt.Errorf("body exceeds %d characters", maxDraftBodyLength)
	case r.ID != "":
		return nil // edits only replace the body
	case r.Path == "":
		return errors.New("path is required")
	case r.Side != "LEFT" && r.Side != "RIGHT":
		return fmt.Errorf("side must be LEFT or RIGHT, got %q", r.Side)
	case r.Line <= 0:
		return errors.New("line must be positive")
	case r.StartLine < 0 || (r.StartLine > 0 && r.StartLine >= r.Line):
		return errors.New("start_line must be before line")
	case r.CommitSHA == "":
		return errors.New("commit_sha is required")
	}
	return nil
}

func (d DraftDeps) logger(ctx context.Context, msg ws.InboundMessage) *slog.Logger {
	if d.Log != nil {
		return d.Log.With("request_id", msg.ID, "msg_type", msg.Type)
	}
	return logging.FromContext(ctx)
}

// currentLogin resolves the account drafts are namespaced under.
func (d DraftDeps) currentLogin(ctx context.Context) (string, error) {
	login, ok, err := d.CurrentUser.GetCurrentUser(ctx, githubProvider)
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	if !ok || login == "" {
		return "", errors.New("drafts require a signed-in account")
	}
	return login, nil
}

func (d DraftDeps) broadcastChanged(prID string) {
	d.Broadcaster.Broadcast(pushMessage{Type: "DRAFTS_CHANGED", Data: map[string]interface{}{"pr_id": prID}})
}

func draftResponse(id, msgType string, data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"id": id, "type": msgType, "data": data}
}

// NewDraftSaveHandler returns the handler for DRAFT_SAVE (create, or update body when id is set).
// Response: DRAFT_SAVED {draft} or {error}.
func NewDraftSaveHandler(deps DraftDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req draftSaveRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse DRAFT_SAVE: %w", err)
		}
		if err := req.validate(); err != nil {
			return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"error": err.Error()}), nil
		}
		login, err := deps.currentLogin(ctx)
		if err != nil {
			return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"error": err.Error()}), nil
		}
		body := strings.TrimSpace(req.Body)
		var saved store.ReviewDraft
		if req.ID != "" {
			saved, err = deps.Drafts.UpdateBody(ctx, login, req.ID, body)
		} else {
			saved, err = deps.Drafts.Create(ctx, login, store.ReviewDraft{
				PRID: req.PRID, Path: req.Path, Side: req.Side, StartLine: req.StartLine,
				Line: req.Line, Body: body, CommitSHA: req.CommitSHA,
			})
		}
		if err != nil {
			deps.logger(ctx, msg).Error("DRAFT_SAVE failed", "pr_id", req.PRID, "error", err)
			return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"error": "unable to save draft"}), nil
		}
		deps.broadcastChanged(req.PRID)
		return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"draft": saved}), nil
	}
}

// NewDraftsListHandler returns the handler for DRAFTS_LIST.
// Response: DRAFTS_READY {pr_id, drafts} or {pr_id, drafts: [], error}.
func NewDraftsListHandler(deps DraftDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse DRAFTS_LIST: %w", err)
		}
		if req.PRID == "" {
			return nil, errors.New("DRAFTS_LIST requires pr_id")
		}
		login, err := deps.currentLogin(ctx)
		if err != nil {
			return draftResponse(msg.ID, "DRAFTS_READY", map[string]interface{}{"pr_id": req.PRID, "drafts": []store.ReviewDraft{}}), nil
		}
		drafts, err := deps.Drafts.List(ctx, req.PRID, login)
		if err != nil {
			deps.logger(ctx, msg).Error("DRAFTS_LIST failed", "pr_id", req.PRID, "error", err)
			return draftResponse(msg.ID, "DRAFTS_READY", map[string]interface{}{"pr_id": req.PRID, "drafts": []store.ReviewDraft{}, "error": "unable to load drafts"}), nil
		}
		return draftResponse(msg.ID, "DRAFTS_READY", map[string]interface{}{"pr_id": req.PRID, "drafts": drafts}), nil
	}
}

// NewDraftDeleteHandler returns the handler for DRAFT_DELETE.
// Response: DRAFTS_DELETED {pr_id, deleted} or {error}.
func NewDraftDeleteHandler(deps DraftDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req struct {
			PRID     string   `json:"pr_id"`
			DraftIDs []string `json:"draft_ids"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse DRAFT_DELETE: %w", err)
		}
		if req.PRID == "" || len(req.DraftIDs) == 0 {
			return nil, errors.New("DRAFT_DELETE requires pr_id and draft_ids")
		}
		login, err := deps.currentLogin(ctx)
		if err != nil {
			return draftResponse(msg.ID, "DRAFTS_DELETED", map[string]interface{}{"error": err.Error()}), nil
		}
		if err := deps.Drafts.Delete(ctx, login, req.DraftIDs); err != nil {
			deps.logger(ctx, msg).Error("DRAFT_DELETE failed", "pr_id", req.PRID, "error", err)
			return draftResponse(msg.ID, "DRAFTS_DELETED", map[string]interface{}{"error": "unable to delete drafts"}), nil
		}
		deps.broadcastChanged(req.PRID)
		return draftResponse(msg.ID, "DRAFTS_DELETED", map[string]interface{}{"pr_id": req.PRID, "deleted": len(req.DraftIDs)}), nil
	}
}
