package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/logging"
)

type Viewed struct {
	Viewed      fileViewedMarker
	CurrentUser currentUserReader
	DiffStore   diffReader
	Baseline    viewedBaselineStore
	Panics      PanicReporter
	Log         *slog.Logger
}
type MarkFileViewedRequest struct {
	PRID        string `json:"pr_id"`
	FilePath    string `json:"file_path"`
	ContentHash string `json:"content_hash"`
}
type FileViewed struct {
	PRID     string `json:"pr_id"`
	FilePath string `json:"file_path"`
}

// MarkFile validates the canonical hash before persisting viewed state.
func (s *Viewed) MarkFile(ctx context.Context, req MarkFileViewedRequest) (FileViewed, error) {
	return guard(s.Panics, "Viewed.MarkFile", func() (FileViewed, error) {
		req.PRID = NormalizeID(req.PRID)
		if req.PRID == "" || req.FilePath == "" || req.ContentHash == "" {
			return FileViewed{}, Errorf(CodeInvalid, "pr_id, file_path, and content_hash are required")
		}
		login, ok, err := s.CurrentUser.Get(ctx, githubProvider)
		if err != nil {
			return FileViewed{}, err
		}
		if !ok || login == "" {
			return FileViewed{}, Errorf(CodeUnauthenticated, "no authenticated user")
		}
		ctx = logging.WithLogger(ctx, serviceLogger(s.Log))
		if s.DiffStore != nil {
			valid, reason, err := validateFileHash(ctx, s.DiffStore, req.PRID, req.FilePath, req.ContentHash)
			if err != nil {
				return FileViewed{}, err
			}
			if !valid {
				return FileViewed{}, WithReason(CodeInvalid, reason, "unable to mark file viewed")
			}
		}
		if err := s.Viewed.MarkFileViewed(ctx, req.PRID, req.FilePath, login, req.ContentHash); err != nil {
			return FileViewed{}, err
		}
		recordAllViewedBaseline(ctx, s, req.PRID, login)
		return FileViewed{PRID: req.PRID, FilePath: req.FilePath}, nil
	})
}

// validateFileHash computes the canonical content hash for filePath in the
// stored diffs and checks it against the client-supplied hash.
// Fails closed: returns (false, reason) when Get errors or the file is
// not found in stored diffs, so stale hashes cannot be accepted silently.
func validateFileHash(ctx context.Context, ds diffReader, prID, filePath, clientHash string) (bool, string, error) {
	diffs, err := ds.Get(prID)
	if err != nil {
		if contextError(err) == nil {
			logging.FromContext(ctx).Warn("content hash validation diff lookup failed", "pr_id", prID, "error", err)
		}
		return false, "could not validate content_hash", contextError(err)
	}
	for _, f := range diffs {
		if f.FilePath == filePath {
			canonical := viewedHash(f)
			if canonical != clientHash {
				prefix := func(s string) string {
					const hashPrefixLength = 8
					if len(s) > hashPrefixLength {
						return s[:hashPrefixLength]
					}
					return s
				}
				return false, fmt.Sprintf(
					"content_hash mismatch: client %s… server %s… (file content may have changed)",
					prefix(clientHash), prefix(canonical)), nil
			}
			return true, "", nil
		}
	}
	// File not found in stored diffs → fail closed; cannot verify.
	return false, fmt.Sprintf("could not validate content_hash: file %q not found in stored diffs", filePath), nil
}
