package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

const (
	triageChanged   = "changed"
	triageNew       = "new"
	triageRemoved   = "removed"
	triageUnchanged = "unchanged"

	triageStateOK             = "ok"
	triageStateNoPatchsets    = "no_patchsets"
	triageStateNoBaseline     = "no_baseline"
	triageStateNoFingerprints = "no_fingerprints"
)

// triageStore is the DB subset REQUEST_TRIAGE reads. Satisfied by *store.DB.
type triageStore interface {
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
	ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error)
	GetLastReviewed(ctx context.Context, prID, login string) (store.LastReviewedRecord, bool, error)
}

// TriageHandlerDeps bundles REQUEST_TRIAGE dependencies.
type TriageHandlerDeps struct {
	Store       triageStore
	CurrentUser currentUserReader
	Log         *slog.Logger
}

type fileTriage struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type triageBaseline struct {
	Source     string `json:"source"`
	ReviewedAt string `json:"reviewed_at"`
}

type triageResponse struct {
	PRID         string          `json:"pr_id"`
	State        string          `json:"state"`
	Reason       string          `json:"reason"`
	FromPatchset int             `json:"from_patchset"`
	ToPatchset   int             `json:"to_patchset"`
	Baseline     *triageBaseline `json:"baseline"`
	Files        []fileTriage    `json:"files"`
	ChangedCount int             `json:"changed_count"`
	TotalCount   int             `json:"total_count"`
}

type triageRequest struct {
	PRID         string `json:"pr_id"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
}

// NewTriageHandler returns a WS handler for REQUEST_TRIAGE: per-file status of
// the target patchset relative to a baseline, from stored patch IDs.
func NewTriageHandler(deps TriageHandlerDeps) ws.MessageHandler {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req triageRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse REQUEST_TRIAGE: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("REQUEST_TRIAGE requires pr_id")
		}
		logger := log.With("request_id", msg.ID, "msg_type", msg.Type, "pr_id", req.PRID)
		response := buildTriage(logging.WithLogger(ctx, logger), deps, req)
		return map[string]interface{}{"id": msg.ID, "type": "TRIAGE_READY", "data": response}, nil
	}
}

func buildTriage(ctx context.Context, deps TriageHandlerDeps, req triageRequest) triageResponse {
	response := triageResponse{PRID: req.PRID, Files: []fileTriage{}}
	patchsets, err := deps.Store.ListPatchsets(ctx, req.PRID)
	if err != nil {
		logging.FromContext(ctx).Warn("triage: list patchsets failed", "error", err)
		return withTriageState(response, triageStateNoPatchsets, "could not load patchsets")
	}
	if len(patchsets) == 0 {
		return withTriageState(response, triageStateNoPatchsets, "no patchsets recorded yet for this PR")
	}
	to := patchsets[len(patchsets)-1]
	if req.ToPatchset > 0 {
		requested := patchsetByNumber(patchsets, req.ToPatchset)
		if requested == nil {
			return withTriageState(response, triageStateNoPatchsets, fmt.Sprintf("P%d is not recorded", req.ToPatchset))
		}
		to = *requested
	}
	response.ToPatchset = to.Number

	from, baseline, reason := triageFrom(ctx, deps, req, patchsets)
	if reason != "" {
		return withTriageState(response, triageStateNoBaseline, reason)
	}
	response.FromPatchset, response.Baseline = from.Number, baseline
	for _, patchset := range []store.Patchset{from, to} {
		if !patchset.HasFingerprints {
			return withTriageState(response, triageStateNoFingerprints, fmt.Sprintf(
				"P%d was recorded before file fingerprints; triage needs a newer patchset", patchset.Number))
		}
	}
	return withTriageFiles(ctx, deps, response)
}

// triageFrom picks the baseline patchset: an explicit pair end, or the user's
// last review. A non-empty reason means there is no comparable baseline.
func triageFrom(ctx context.Context, deps TriageHandlerDeps, req triageRequest, patchsets []store.Patchset) (store.Patchset, *triageBaseline, string) {
	if req.FromPatchset > 0 {
		explicit := patchsetByNumber(patchsets, req.FromPatchset)
		if explicit == nil {
			return store.Patchset{}, nil, fmt.Sprintf("P%d is not recorded", req.FromPatchset)
		}
		return *explicit, nil, ""
	}
	login, ok, err := deps.CurrentUser.GetCurrentUser(ctx, githubProvider)
	if err != nil {
		logging.FromContext(ctx).Warn("triage: current user lookup failed", "error", err)
		return store.Patchset{}, nil, "no authenticated user"
	}
	if !ok || login == "" {
		return store.Patchset{}, nil, "no authenticated user"
	}
	record, found, err := deps.Store.GetLastReviewed(ctx, req.PRID, login)
	if err != nil {
		logging.FromContext(ctx).Warn("triage: last reviewed lookup failed", "error", err)
		return store.Patchset{}, nil, "could not load your last review"
	}
	if !found {
		return store.Patchset{}, nil, "no prior review recorded"
	}
	from, ok := resolveBaselinePatchset(patchsets, record)
	if !ok {
		return store.Patchset{}, nil, "no comparable prior review: Bistro never recorded the head you reviewed"
	}
	return from, &triageBaseline{Source: string(record.Source), ReviewedAt: record.ReviewedAt.UTC().Format(time.RFC3339)}, ""
}

func withTriageFiles(ctx context.Context, deps TriageHandlerDeps, response triageResponse) triageResponse {
	baselineFiles, err := deps.Store.ListPatchsetFiles(ctx, response.PRID, response.FromPatchset)
	if err == nil {
		var targetFiles map[string]string
		targetFiles, err = deps.Store.ListPatchsetFiles(ctx, response.PRID, response.ToPatchset)
		if err == nil {
			response.Files = triageFiles(baselineFiles, targetFiles)
		}
	}
	if err != nil {
		logging.FromContext(ctx).Warn("triage: list patchset files failed", "error", err)
		return withTriageState(response, triageStateNoFingerprints, "could not load file fingerprints")
	}
	response.State, response.TotalCount = triageStateOK, len(response.Files)
	for _, file := range response.Files {
		if file.Status != triageUnchanged {
			response.ChangedCount++
		}
	}
	return response
}

func withTriageState(response triageResponse, state, reason string) triageResponse {
	response.State, response.Reason = state, reason
	return response
}

// resolveBaselinePatchset maps a stored review to a recorded patchset: the
// stored number when its head still matches, else the latest patchset at the
// reviewed head.
func resolveBaselinePatchset(patchsets []store.Patchset, record store.LastReviewedRecord) (store.Patchset, bool) {
	if stored := patchsetByNumber(patchsets, record.PatchsetNum); stored != nil && stored.HeadSHA == record.SHA {
		return *stored, true
	}
	for i := len(patchsets) - 1; i >= 0; i-- {
		if patchsets[i].HeadSHA == record.SHA {
			return patchsets[i], true
		}
	}
	return store.Patchset{}, false
}

// triageFiles compares per-file patch IDs; files only in the baseline are
// removed. Sorted by path.
func triageFiles(baseline, target map[string]string) []fileTriage {
	files := make([]fileTriage, 0, len(target)+len(baseline))
	for path, patchID := range target {
		baselineID, inBaseline := baseline[path]
		status := triageUnchanged
		switch {
		case !inBaseline:
			status = triageNew
		case baselineID != patchID:
			status = triageChanged
		}
		files = append(files, fileTriage{Path: path, Status: status})
	}
	for path := range baseline {
		if _, inTarget := target[path]; !inTarget {
			files = append(files, fileTriage{Path: path, Status: triageRemoved})
		}
	}
	slices.SortFunc(files, func(a, b fileTriage) int { return strings.Compare(a.Path, b.Path) })
	return files
}
