package services

import (
	"context"
	"log/slog"

	"github.com/itterno/bistro/internal/store"
)

type Patchsets struct {
	Store  patchsetLister
	Panics PanicReporter
	Log    *slog.Logger
}
type PatchsetsResponse struct {
	PRID      string           `json:"pr_id"`
	Patchsets []store.Patchset `json:"patchsets"`
}

// List returns recorded patchsets, ordered by number.
func (s *Patchsets) List(ctx context.Context, req PRRef) (PatchsetsResponse, error) {
	return guard(s.Panics, "Patchsets.List", func() (PatchsetsResponse, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return PatchsetsResponse{}, Errorf(CodeInvalid, "pr_id is required")
		}
		patchsets, err := s.Store.ListPatchsets(ctx, prID)
		if err != nil {
			return PatchsetsResponse{}, err
		}
		if patchsets == nil {
			patchsets = []store.Patchset{}
		}
		return PatchsetsResponse{PRID: prID, Patchsets: patchsets}, nil
	})
}

// patchsetByNumber finds a patchset by its number in an ordered slice.
func patchsetByNumber(patchsets []store.Patchset, number int) *store.Patchset {
	for i := range patchsets {
		if patchsets[i].Number == number {
			return &patchsets[i]
		}
	}
	return nil
}
