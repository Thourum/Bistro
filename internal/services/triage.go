package services

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
)

type Triage struct {
	Store       triageStore
	CurrentUser currentUserReader
	Panics      PanicReporter
	Log         *slog.Logger
}

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

type FileTriage struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type TriageBaseline struct {
	Source     string `json:"source"`
	ReviewedAt string `json:"reviewed_at"`
}

type TriageResponse struct {
	PRID         string          `json:"pr_id"`
	State        string          `json:"state"`
	Reason       string          `json:"reason"`
	FromPatchset int             `json:"from_patchset"`
	ToPatchset   int             `json:"to_patchset"`
	Baseline     *TriageBaseline `json:"baseline"`
	Files        []FileTriage    `json:"files"`
	ChangedCount int             `json:"changed_count"`
	TotalCount   int             `json:"total_count"`
}

type TriageRequest struct {
	PRID         string `json:"pr_id"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
}

// Get compares a patchset pair, defaulting to the user's last reviewed head.
func (s *Triage) Get(ctx context.Context, req TriageRequest) (TriageResponse, error) {
	return guard(s.Panics, "Triage.Get", func() (TriageResponse, error) {
		req.PRID = NormalizeID(req.PRID)
		if req.PRID == "" || req.FromPatchset < 0 || req.ToPatchset < 0 {
			return TriageResponse{}, Errorf(CodeInvalid, "pr_id and non-negative patchsets are required")
		}
		if err := requirePR(ctx, s.Store, req.PRID); err != nil {
			return TriageResponse{}, err
		}
		return buildTriage(logging.WithLogger(ctx, serviceLogger(s.Log)), s, req)
	})
}
func buildTriage(ctx context.Context, deps *Triage, req TriageRequest) (TriageResponse, error) {
	response := TriageResponse{PRID: req.PRID, Files: []FileTriage{}}
	patchsets, err := deps.Store.ListPatchsets(ctx, req.PRID)
	if err != nil {
		if contextError(err) == nil {
			logging.FromContext(ctx).Warn("triage: list patchsets failed", "error", err)
		}
		return withTriageState(response, triageStateNoPatchsets, "could not load patchsets"), contextError(err)
	}
	if len(patchsets) == 0 {
		return withTriageState(response, triageStateNoPatchsets, "no patchsets recorded yet for this PR"), nil
	}
	to := patchsets[len(patchsets)-1]
	if req.ToPatchset > 0 {
		requested := patchsetByNumber(patchsets, req.ToPatchset)
		if requested == nil {
			return withTriageState(response, triageStateNoPatchsets, fmt.Sprintf("P%d is not recorded", req.ToPatchset)), nil
		}
		to = *requested
	}
	if req.ToPatchset == 0 {
		current, found, err := deps.Store.CurrentPatchset(ctx, req.PRID)
		if err != nil {
			if contextError(err) == nil {
				logging.FromContext(ctx).Warn("triage: current patchset lookup failed", "error", err)
			}
			return withTriageState(response, triageStateNoPatchsets, "could not load patchsets"), contextError(err)
		}
		if found {
			to = current
		}
	}
	response.ToPatchset = to.Number

	from, baseline, reason, err := triageFrom(ctx, deps, req, patchsets)
	if err != nil {
		return response, err
	}
	if reason != "" {
		return withTriageState(response, triageStateNoBaseline, reason), nil
	}
	response.FromPatchset, response.Baseline = from.Number, baseline
	for _, patchset := range []store.Patchset{from, to} {
		if !patchset.HasFingerprints {
			return withTriageState(response, triageStateNoFingerprints, fmt.Sprintf(
				"P%d was recorded before file fingerprints; triage needs a newer patchset", patchset.Number)), nil
		}
	}
	return withTriageFiles(ctx, deps, response)
}

// triageFrom picks the baseline patchset: an explicit pair end, or the user's
// last review. A non-empty reason means there is no comparable baseline.
func triageFrom(ctx context.Context, deps *Triage, req TriageRequest, patchsets []store.Patchset) (store.Patchset, *TriageBaseline, string, error) {
	if req.FromPatchset > 0 {
		explicit := patchsetByNumber(patchsets, req.FromPatchset)
		if explicit == nil {
			return store.Patchset{}, nil, fmt.Sprintf("P%d is not recorded", req.FromPatchset), nil
		}
		return *explicit, nil, "", nil
	}
	if deps.CurrentUser == nil {
		return store.Patchset{}, nil, "no authenticated user", nil
	}
	login, ok, err := deps.CurrentUser.Get(ctx, githubProvider)
	if err != nil {
		if contextError(err) == nil {
			logging.FromContext(ctx).Warn("triage: current user lookup failed", "error", err)
		}
		return store.Patchset{}, nil, "no authenticated user", contextError(err)
	}
	if !ok || login == "" {
		return store.Patchset{}, nil, "no authenticated user", nil
	}
	record, found, err := deps.Store.GetLastReviewed(ctx, req.PRID, login)
	if err != nil {
		if contextError(err) == nil {
			logging.FromContext(ctx).Warn("triage: last reviewed lookup failed", "error", err)
		}
		return store.Patchset{}, nil, "could not load your last review", contextError(err)
	}
	if !found {
		return store.Patchset{}, nil, "no prior review recorded", nil
	}
	from, ok := resolveBaselinePatchset(patchsets, record)
	if !ok {
		return store.Patchset{}, nil, "no comparable prior review: Bistro never recorded the head you reviewed", nil
	}
	return from, &TriageBaseline{Source: string(record.Source), ReviewedAt: record.ReviewedAt.UTC().Format(time.RFC3339)}, "", nil
}

func withTriageFiles(ctx context.Context, deps *Triage, response TriageResponse) (TriageResponse, error) {
	baselineFiles, err := deps.Store.ListPatchsetFiles(ctx, response.PRID, response.FromPatchset)
	if err != nil {
		return unavailableTriageFiles(ctx, response, err)
	}
	targetFiles, err := deps.Store.ListPatchsetFiles(ctx, response.PRID, response.ToPatchset)
	if err != nil {
		return unavailableTriageFiles(ctx, response, err)
	}
	response.Files = triageFiles(baselineFiles, targetFiles)
	response.State, response.TotalCount = triageStateOK, len(response.Files)
	for _, file := range response.Files {
		if file.Status != triageUnchanged {
			response.ChangedCount++
		}
	}
	return response, nil
}

func unavailableTriageFiles(ctx context.Context, response TriageResponse, err error) (TriageResponse, error) {
	if cause := contextError(err); cause != nil {
		return response, cause
	}
	logging.FromContext(ctx).Warn("triage: list patchset files failed", "error", err)
	return withTriageState(response, triageStateNoFingerprints, "could not load file fingerprints"), nil
}

func withTriageState(response TriageResponse, state, reason string) TriageResponse {
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
func triageFiles(baseline, target map[string]string) []FileTriage {
	files := make([]FileTriage, 0, len(target)+len(baseline))
	for path, patchID := range target {
		baselineID, inBaseline := baseline[path]
		status := triageUnchanged
		switch {
		case !inBaseline:
			status = triageNew
		case baselineID != patchID:
			status = triageChanged
		}
		files = append(files, FileTriage{Path: path, Status: status})
	}
	for path := range baseline {
		if _, inTarget := target[path]; !inTarget {
			files = append(files, FileTriage{Path: path, Status: triageRemoved})
		}
	}
	slices.SortFunc(files, func(a, b FileTriage) int { return strings.Compare(a.Path, b.Path) })
	return files
}
