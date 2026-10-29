package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/itterno/bistro/internal/store/reviewdraft"
)

// maxDraftBodyLength matches GitHub's comment body limit in characters.
const maxDraftBodyLength = 65536

type draftStore interface {
	Create(context.Context, string, reviewdraft.Draft) (reviewdraft.Draft, error)
	UpdateBody(context.Context, string, string, string) (reviewdraft.Draft, error)
	List(context.Context, string, string) ([]reviewdraft.Draft, error)
	Delete(context.Context, string, []string) (int64, error)
}
type Drafts struct {
	Drafts      draftStore
	CurrentUser currentUserReader
	Events      Emitter
	Panics      PanicReporter
	Log         *slog.Logger
}
type DraftSaveRequest struct {
	PRID      string `json:"pr_id"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line"`
	Line      int    `json:"line"`
	Body      string `json:"body"`
	CommitSHA string `json:"commit_sha"`
}
type DraftDeleteRequest struct {
	PRID     string   `json:"pr_id"`
	DraftIDs []string `json:"draft_ids"`
}
type DraftsResponse struct {
	PRID   string              `json:"pr_id"`
	Drafts []reviewdraft.Draft `json:"drafts"`
}
type DraftSaved struct {
	Draft reviewdraft.Draft `json:"draft"`
}
type DraftsDeleted struct {
	PRID    string `json:"pr_id"`
	Deleted int    `json:"deleted"`
}

func (r DraftSaveRequest) validate() error {
	body := strings.TrimSpace(r.Body)
	switch {
	case r.PRID == "":
		return errors.New("pr_id is required")
	case body == "":
		return errors.New("body is required")
	case utf8.RuneCountInString(body) > maxDraftBodyLength:
		return fmt.Errorf("body exceeds %d characters", maxDraftBodyLength)
	case r.ID != "":
		return nil // Edits replace only the body.
	case r.Path == "":
		return errors.New("path is required")
	case r.Side != "LEFT" && r.Side != "RIGHT":
		return errors.New("side must be LEFT or RIGHT")
	case r.Line <= 0:
		return errors.New("line must be positive")
	case r.StartLine < 0 || (r.StartLine > 0 && r.StartLine >= r.Line):
		return errors.New("start_line must be before line")
	case r.CommitSHA == "":
		return errors.New("commit_sha is required")
	}
	return nil
}
func (s *Drafts) currentLogin(ctx context.Context) (string, error) {
	login, ok, err := s.CurrentUser.Get(ctx, githubProvider)
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	if !ok || login == "" {
		return "", Errorf(CodeUnauthenticated, "drafts require a signed-in account")
	}
	return login, nil
}

// Save creates a positioned draft or updates its body and emits the affected PR.
func (s *Drafts) Save(ctx context.Context, req DraftSaveRequest) (DraftSaved, error) {
	return guard(s.Panics, "Drafts.Save", func() (DraftSaved, error) {
		normalized := req
		normalized.PRID = NormalizeID(req.PRID)
		if err := normalized.validate(); err != nil {
			return DraftSaved{}, Errorf(CodeInvalid, "%s", err)
		}
		login, err := s.currentLogin(ctx)
		if err != nil {
			return DraftSaved{}, err
		}
		body := strings.TrimSpace(req.Body)
		var saved reviewdraft.Draft
		if req.ID != "" {
			saved, err = s.Drafts.UpdateBody(ctx, login, req.ID, body)
		} else {
			saved, err = s.Drafts.Create(ctx, login, reviewdraft.Draft{PRID: normalized.PRID, Path: req.Path, Side: req.Side, StartLine: req.StartLine, Line: req.Line, Body: body, CommitSHA: req.CommitSHA})
		}
		if err != nil {
			return DraftSaved{}, err
		}
		s.Events.Emit(EventDraftsChanged, DraftsChanged{PRID: normalized.PRID})
		return DraftSaved{Draft: saved}, nil
	})
}

// List returns only drafts belonging to the signed-in account.
func (s *Drafts) List(ctx context.Context, req PRRef) (DraftsResponse, error) {
	return guard(s.Panics, "Drafts.List", func() (DraftsResponse, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return DraftsResponse{}, Errorf(CodeInvalid, "pr_id is required")
		}
		login, err := s.currentLogin(ctx)
		if err != nil {
			return DraftsResponse{}, err
		}
		drafts, err := s.Drafts.List(ctx, prID, login)
		if err != nil {
			return DraftsResponse{}, err
		}
		if drafts == nil {
			drafts = []reviewdraft.Draft{}
		}
		return DraftsResponse{PRID: prID, Drafts: drafts}, nil
	})
}

// Delete reports committed deletions and preserves notifications for idempotent deletes.
func (s *Drafts) Delete(ctx context.Context, req DraftDeleteRequest) (DraftsDeleted, error) {
	return guard(s.Panics, "Drafts.Delete", func() (DraftsDeleted, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" || len(req.DraftIDs) == 0 {
			return DraftsDeleted{}, Errorf(CodeInvalid, "pr_id and draft_ids are required")
		}
		login, err := s.currentLogin(ctx)
		if err != nil {
			return DraftsDeleted{}, err
		}
		deleted, err := s.Drafts.Delete(ctx, login, req.DraftIDs)
		if err != nil {
			return DraftsDeleted{}, err
		}
		s.Events.Emit(EventDraftsChanged, DraftsChanged{PRID: prID})
		return DraftsDeleted{PRID: prID, Deleted: int(deleted)}, nil
	})
}
