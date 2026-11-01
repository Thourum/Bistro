package services

import (
	"context"
	"log/slog"
	"sync"

	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
)

type ValidateDevFolderRequest struct {
	Path string `json:"path"`
}
type ValidateDevFolderResult struct {
	CanonicalPath string `json:"canonical_path"`
}
type ScanDevFolderRequest struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type ScanDevFolderResult struct {
	Discovered int  `json:"discovered"`
	Healthy    int  `json:"healthy"`
	Degraded   int  `json:"degraded"`
	Skipped    int  `json:"skipped"`
	Partial    bool `json:"partial"`
}

// DevFolderScan validates development folders and registers discovered checkouts.
type DevFolderScan struct {
	Locations   *repolocation.Store
	SyncTrigger chan<- string
	ScanFunc    func(context.Context, string) (scan.Result, error)
	Panics      PanicReporter
	Log         *slog.Logger

	mu            sync.Mutex
	cancelCurrent context.CancelFunc
}

// Validate resolves a readable development folder to its canonical path.
func (s *DevFolderScan) Validate(ctx context.Context, req ValidateDevFolderRequest) (ValidateDevFolderResult, error) {
	return guard(s.Panics, "DevFolderScan.Validate", func() (ValidateDevFolderResult, error) {
		if err := ctx.Err(); err != nil {
			return ValidateDevFolderResult{}, err
		}
		path, err := scan.ValidateDevFolder(req.Path)
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return ValidateDevFolderResult{}, ctxErr
			}
			serviceLogger(s.Log).Warn("validate dev folder failed", "error", err)
			return ValidateDevFolderResult{}, locationError("invalid_path", "invalid development folder path")
		}
		return ValidateDevFolderResult{CanonicalPath: path}, nil
	})
}

// Scan runs one dev-folder scan. A newer Scan cancels this one; the
// renderer cancels by cancelling the call's promise (ctx).
func (s *DevFolderScan) Scan(ctx context.Context, req ScanDevFolderRequest) (ScanDevFolderResult, error) {
	return guard(s.Panics, "DevFolderScan.Scan", func() (ScanDevFolderResult, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		s.mu.Lock()
		if s.cancelCurrent != nil {
			s.cancelCurrent()
		}
		s.cancelCurrent = cancel
		s.mu.Unlock()
		if isBistroRepoDirOverrideSet() {
			return ScanDevFolderResult{}, nil
		}
		validated, err := s.Validate(ctx, ValidateDevFolderRequest{Path: req.Path})
		if err != nil {
			return ScanDevFolderResult{}, err
		}
		found, err := s.ScanFunc(ctx, validated.CanonicalPath)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ScanDevFolderResult{}, ctxErr
		}
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return ScanDevFolderResult{}, ctxErr
			}
			serviceLogger(s.Log).Error("dev-folder scan failed", "error", err)
			return ScanDevFolderResult{}, locationError("scan_failed", "Unable to scan development folder.")
		}
		healthy, degraded, err := updateScannedRepoLocations(ctx, s.Locations, found.Found)
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return ScanDevFolderResult{}, ctxErr
			}
			serviceLogger(s.Log).Error("persist dev-folder scan failed", "error", err)
			return ScanDevFolderResult{}, locationError("persistence_failed", "Unable to save discovered repositories.")
		}
		if err := ctx.Err(); err != nil {
			return ScanDevFolderResult{}, err
		}
		triggerSync(s.SyncTrigger, serviceLogger(s.Log))
		return ScanDevFolderResult{Discovered: len(found.Found), Healthy: healthy, Degraded: degraded, Skipped: found.Skipped, Partial: found.Partial}, nil
	})
}
