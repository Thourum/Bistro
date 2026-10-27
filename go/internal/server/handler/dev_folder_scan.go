package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/ws"
)

const scanPersistenceTimeout = 5 * time.Second

// ScanDevFolderDeps supplies the scan lifetime, persistence, and result delivery.
// Context is the app's run context, not a WebSocket request context.
type ScanDevFolderDeps struct {
	Context     context.Context
	Locations   *repolocation.Store
	SyncTrigger chan<- string
	Broadcast   func(any)
	Scan        func(context.Context, string) (scan.Result, error)
	Log         *slog.Logger
}

type devFolderScanFlow struct {
	id      string
	cancel  context.CancelFunc
	done    chan struct{}
	message ws.InboundMessage
	path    string
}

type devFolderScans struct {
	deps      ScanDevFolderDeps
	mu        sync.Mutex
	active    *devFolderScanFlow
	isClosing bool
	workers   sync.WaitGroup
}

// NewScanDevFolderHandlers returns nonblocking start/cancel handlers and a join
// function. Joining prevents new starts and cancels the active filesystem scan.
func NewScanDevFolderHandlers(deps ScanDevFolderDeps) (start, cancel ws.MessageHandler, wait func()) {
	scans := &devFolderScans{deps: deps}
	return scans.start, scans.cancel, scans.wait
}

func (s *devFolderScans) start(ctx context.Context, msg ws.InboundMessage) (any, error) {
	var data struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil, fmt.Errorf("invalid SCAN_DEV_FOLDER payload: %w", err)
	}
	if isBistroRepoDirOverrideSet() {
		return locationResponse(msg, scanDevFolderPayload{OK: true}), nil
	}
	canonicalPath, err := scan.ValidateDevFolder(data.Path)
	if err != nil {
		repoLocationLogger(ctx, s.deps.Log, msg).Warn("scan dev folder validation failed", "error", err)
		return locationResponse(msg, locationError("", operationScanDevFolder, "invalid_path", "invalid development folder path")), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isClosing || s.deps.Context.Err() != nil {
		return nil, nil
	}
	scanCtx, stop := context.WithCancel(s.deps.Context)
	flow := &devFolderScanFlow{
		id: msg.ID, cancel: stop, done: make(chan struct{}), message: msg, path: canonicalPath,
	}
	previous := s.active
	if previous != nil {
		previous.cancel()
	}
	s.active = flow
	s.workers.Add(1)
	go s.run(scanCtx, flow, previous)
	return nil, nil
}

func (s *devFolderScans) cancel(_ context.Context, msg ws.InboundMessage) (any, error) {
	var data struct {
		ScanID *string `json:"scan_id"`
	}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil, fmt.Errorf("invalid SCAN_CANCEL payload: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && (data.ScanID == nil || *data.ScanID == s.active.id) {
		s.active.cancel()
	}
	return nil, nil
}

func (s *devFolderScans) wait() {
	s.mu.Lock()
	s.isClosing = true
	if s.active != nil {
		s.active.cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}

func (s *devFolderScans) finish(flow *devFolderScanFlow) {
	flow.cancel()
	s.mu.Lock()
	if s.active == flow {
		s.active = nil
	}
	close(flow.done)
	s.mu.Unlock()
	s.workers.Done()
}

func (s *devFolderScans) run(scanCtx context.Context, flow, previous *devFolderScanFlow) {
	defer s.finish(flow)
	msg := flow.message
	if previous != nil {
		// Stop only cancels filesystem work. Finish old persistence before the
		// successor can update the same repository locations.
		select {
		case <-previous.done:
		case <-s.deps.Context.Done():
			return
		}
	}
	if s.deps.Context.Err() != nil {
		return
	}
	result, err := s.deps.Scan(scanCtx, flow.path)
	if s.deps.Context.Err() != nil {
		return
	}
	logger := repoLocationLogger(s.deps.Context, s.deps.Log, msg)
	isContextError := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	if err != nil && !isContextError {
		logger.Error("dev-folder scan failed", "error", err)
		s.deps.Broadcast(locationResponse(msg, locationError("", operationScanDevFolder, "scan_failed", "Unable to scan development folder.")))
		return
	}
	if isContextError || scanCtx.Err() != nil {
		result.Partial = true
	}
	s.persist(msg, result, logger)
}

func (s *devFolderScans) persist(msg ws.InboundMessage, result scan.Result, logger *slog.Logger) {
	// A user Stop must not cancel the writes for discoveries already found.
	writeCtx, stopWrite := context.WithTimeout(s.deps.Context, scanPersistenceTimeout)
	defer stopWrite()
	healthy, degraded, err := updateScannedRepoLocations(writeCtx, s.deps.Locations, result.Found)
	if s.deps.Context.Err() != nil {
		return
	}
	if err != nil {
		logger.Error("persist dev-folder scan failed", "error", err)
		s.deps.Broadcast(locationResponse(msg, locationError("", operationScanDevFolder, "persistence_failed", "Unable to save discovered repositories.")))
		return
	}
	triggerSync(s.deps.SyncTrigger, logger)
	s.deps.Broadcast(locationResponse(msg, scanDevFolderPayload{
		OK: true, Discovered: len(result.Found), Healthy: healthy, Degraded: degraded,
		Skipped: result.Skipped, Partial: result.Partial,
	}))
}
