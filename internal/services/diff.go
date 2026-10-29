package services

import (
	"context"
	"errors"
	"log/slog"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/store/prdiff"
)

type Diff struct {
	DiffStore   diffReader
	ViewedStore diffViewedChecker
	CurrentUser currentUserReader
	Panics      PanicReporter
	Log         *slog.Logger
}
type DiffResponse struct {
	PRID      string     `json:"pr_id"`
	Diff      string     `json:"diff"`
	Files     []DiffFile `json:"files"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
}
type DiffFile struct {
	Path        string `json:"path"`
	Status      string `json:"status"`
	OldPath     string `json:"old_path,omitempty"`
	IsBinary    bool   `json:"is_binary"`
	IsTruncated bool   `json:"is_truncated"`
	DiffStatus  string `json:"diff_status"`
	PatchBytes  int    `json:"patch_bytes"`
	HunkCount   int    `json:"hunk_count"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	ContentHash string `json:"content_hash"`
	Viewed      bool   `json:"viewed"`
}

// viewedHash uses the patch ID so viewed state survives line-shift-only rebases.
// Rows cached before migration 035 have no stored patch ID; compute it instead.
func viewedHash(file prdiff.File) string {
	if file.PatchID != "" {
		return file.PatchID
	}
	return diff.PatchID(file.Patch)
}

// Get loads the cached diff and joins viewed flags for the current user.
func (s *Diff) Get(ctx context.Context, req PRRef) (DiffResponse, error) {
	return guard(s.Panics, "Diff.Get", func() (DiffResponse, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return DiffResponse{}, Errorf(CodeInvalid, "pr_id is required")
		}
		unified, err := s.DiffStore.GetUnified(prID)
		if err != nil {
			if contextError(err) != nil {
				return DiffResponse{}, err
			}
			if errors.Is(err, prdiff.ErrNotFound) {
				return DiffResponse{}, Errorf(CodeNotFound, "unable to load diff")
			}
			return DiffResponse{}, err
		}

		login := ""
		if s.CurrentUser != nil {
			l, ok, err := s.CurrentUser.Get(ctx, githubProvider)
			if err != nil {
				return DiffResponse{}, err
			}
			if ok {
				login = l
			}
		}
		diffFiles, err := s.DiffStore.Get(prID)
		if err != nil {
			return DiffResponse{}, err
		}
		response := DiffResponse{PRID: prID, Diff: unified, Files: []DiffFile{}}
		for _, file := range diffFiles {
			hash := viewedHash(file)
			viewed := false
			if s.ViewedStore != nil && login != "" {
				viewed, err = s.ViewedStore.IsFileViewed(ctx, prID, file.FilePath, login, hash)
				if err != nil {
					return DiffResponse{}, err
				}
			}
			response.Files = append(response.Files, DiffFile{Path: file.FilePath, Status: file.Status, OldPath: file.OldPath, IsBinary: file.IsBinary, IsTruncated: file.IsTruncated, DiffStatus: file.DiffStatus, PatchBytes: file.PatchBytes, HunkCount: file.HunkCount, Additions: file.Additions, Deletions: file.Deletions, ContentHash: hash, Viewed: viewed})
			response.Additions += file.Additions
			response.Deletions += file.Deletions
		}
		return response, nil
	})
}
