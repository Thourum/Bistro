package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	gosync "sync"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// ExpertiseHook receives successful PR diff sync notifications for incremental expertise updates.
type ExpertiseHook interface {
	OnPRSynced(ctx context.Context, repoID, repoPath string, files []string)
}

// RepoPathResolver resolves a repo ID to a local checkout path.
type RepoPathResolver func(repoID string) (string, bool)

// Orchestrator coordinates sync operations
type Orchestrator struct {
	log             *slog.Logger
	provider        provider.Provider
	store           *store.DB
	git             *git.Client
	emitter         *events.EventEmitter
	rateLimitMu     gosync.RWMutex
	rateLimitState  *provider.RateLimitState
	queue           *store.ActionQueue
	diffStore       *store.DiffStore
	expertise       ExpertiseHook
	resolveRepoPath RepoPathResolver
	// reviewLogin/reviewSHAWriter are optional; if set they are attached to
	// every ActionExecutor so successful reviews record last_reviewed_sha.
	reviewLogin     actions.ReviewLoginGetter
	reviewSHAWriter actions.ReviewSHAWriter
	// anchorResolver is optional; when set it runs on each clone after diff
	// generation and populates PRFetchResult.AnchorUpdates.
	anchorResolver *git.AnchorResolver
	// liveness is cancelled when this orchestrator is superseded (provider
	// replaced or disconnected). ApplyAction checks it before executing so
	// stale orchestrators self-reject even if captured before disconnect.
	liveness context.Context
	// classifier is optional; when set, writers classify each comment in the write path.
	classifier *classify.EngineProvider
	// currentUser resolves the authenticated GitHub login for mention detection
	// during sync writes.
	currentUser currentUserReader
	// postCommit is optional; when set, writers invoke it after each committed
	// batch with the written PR ids.
	postCommit func(prIDs []string)
	// postReview is optional; when set, successful local review submissions invoke it
	// after last_reviewed_sha tracking has run.
	postReview func(ctx context.Context, prID string)
}

const (
	inboxCursorLag                = 2 * time.Minute
	replayReconciliationClockSkew = 10 * time.Minute
	// actionRetryCount matches the executor's bounded retry policy.
	actionRetryCount = 3
	// queuedIDHashBytes keeps queue IDs compact without exposing comment text.
	queuedIDHashBytes = 8
	// replayBatchSize caps actions claimed in one replay pass.
	replayBatchSize = 10
	// queueRetention windows bound local history of terminal actions.
	completedQueueRetention = 7 * 24 * time.Hour
	failedQueueRetention    = 24 * time.Hour
	// defaultRateLimitCooldown is used when the provider reports no reset time.
	defaultRateLimitCooldown = 60 * time.Second
	// inboxDiscoveryLimit bounds the number of discovered PR pointers per pass.
	inboxDiscoveryLimit = 100

	// prListWindow bounds per-repo PR list syncs to recently updated PRs;
	// prListMinCount guarantees quiet repos still surface their newest PRs.
	prListWindow   = 30 * 24 * time.Hour
	prListMinCount = 10

	// hydrationThreshold is the PR count above which a repo sync skips the
	// per-PR detail fetch (comments, CI, diff) and writes list-only rows;
	// details are hydrated on demand when the user selects a PR.
	hydrationThreshold = 100
)

// SupportsInboxDiscovery reports whether the provider can discover relevant PRs
// from provider-native inbox/search state without scanning every watched repo.
func (o *Orchestrator) SupportsInboxDiscovery() bool {
	_, ok := o.provider.(provider.InboxDiscoveryProvider)
	return ok
}

// NewOrchestrator creates a new sync orchestrator. If log is nil, slog.Default()
// is used — downstream code can rely on o.log being non-nil.
func NewOrchestrator(
	log *slog.Logger,
	prov provider.Provider,
	db *store.DB,
	gitClient *git.Client,
) *Orchestrator {
	if log == nil {
		log = slog.Default()
	}
	return &Orchestrator{
		log:      log,
		provider: prov,
		store:    db,
		git:      gitClient,
		emitter:  events.NewEventEmitter(),
	}
}

// shortID returns a random hex identifier for correlating a single sync cycle
// or action batch across log records.
func shortID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:])
}

// WithDiffStore attaches a DiffStore so diffs are persisted during sync.
// This is optional; if not set, diffs are generated but not stored.
func (o *Orchestrator) WithDiffStore(ds *store.DiffStore) *Orchestrator {
	o.diffStore = ds
	return o
}

// WithReviewTracking attaches login-resolver and SHA-writer so that successful
// review actions (approve, request-changes, comment) record last_reviewed_sha.
func (o *Orchestrator) WithReviewTracking(loginGetter actions.ReviewLoginGetter, shaWriter actions.ReviewSHAWriter) *Orchestrator {
	o.reviewLogin = loginGetter
	o.reviewSHAWriter = shaWriter
	return o
}

// WithAnchorResolver attaches an AnchorResolver so inline review comment
// anchors are resolved and cached after each successful diff clone.
func (o *Orchestrator) WithAnchorResolver(r *git.AnchorResolver) *Orchestrator {
	o.anchorResolver = r
	return o
}

// WithActionQueue attaches a persistent action queue for offline/rate-limited
// action replay. Production orchestrators should always have a queue attached.
func (o *Orchestrator) WithActionQueue(q *store.ActionQueue) *Orchestrator {
	o.queue = q
	return o
}

// WithLiveness attaches a context that is cancelled when this orchestrator is
// superseded (provider disconnected or replaced). ApplyAction checks it before
// executing so a handler that captured a stale orchestrator reference before
// disconnect will have its action rejected rather than executed with old credentials.
func (o *Orchestrator) WithLiveness(ctx context.Context) *Orchestrator {
	o.liveness = ctx
	return o
}

// WithClassifier attaches the classification engine provider; writers built
// by this orchestrator classify comments in the write path.
func (o *Orchestrator) WithClassifier(p *classify.EngineProvider) *Orchestrator {
	o.classifier = p
	return o
}

// WithCurrentUser attaches a current-user reader so sync writers can detect
// mentions of the authenticated GitHub account.
func (o *Orchestrator) WithCurrentUser(reader currentUserReader) *Orchestrator {
	o.currentUser = reader
	return o
}

// WithExpertise attaches the expertise engine hook for post-diff incremental updates.
func (o *Orchestrator) WithExpertise(h ExpertiseHook) *Orchestrator {
	o.expertise = h
	return o
}

// WithRepoPathResolver attaches local checkout path resolution for expertise updates.
func (o *Orchestrator) WithRepoPathResolver(resolve RepoPathResolver) *Orchestrator {
	o.resolveRepoPath = resolve
	return o
}

// WithPostCommit attaches a hook writers invoke after each committed batch
// with the written PR ids.
func (o *Orchestrator) WithPostCommit(fn func(prIDs []string)) *Orchestrator {
	o.postCommit = fn
	return o
}

// WithPostReview attaches a hook invoked after successful review-like actions.
func (o *Orchestrator) WithPostReview(fn func(ctx context.Context, prID string)) *Orchestrator {
	o.postReview = fn
	return o
}

// IsRateLimited reports whether the orchestrator is currently rate limited.
func (o *Orchestrator) IsRateLimited() bool {
	o.rateLimitMu.RLock()
	defer o.rateLimitMu.RUnlock()
	if o.rateLimitState == nil {
		return false
	}
	return o.rateLimitState.IsLimited()
}

// isRateLimitError reports whether err signals an HTTP 429 rate limit.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return pe.Type == provider.ErrorTypeRateLimit
	}
	return false
}

// GetEventChannel returns a read-only channel for subscribing to sync events.
// The channel is buffered; the caller should drain it promptly to avoid dropped events.
func (o *Orchestrator) GetEventChannel() <-chan *events.Event {
	return o.emitter.Subscribe()
}

// UnsubscribeEventChannel removes the subscription identified by ch and closes
// its underlying send channel so the receiver can detect closure.
func (o *Orchestrator) UnsubscribeEventChannel(ch <-chan *events.Event) {
	o.emitter.Unsubscribe(ch)
}

type syncPhaseError struct {
	phase      string
	message    string
	cause      error
	durationMS int64
}

// Sync executes a full sync operation. All log records emitted downstream are
// tagged with a per-cycle sync_id plus the repo slug so a full sync can be
// grep'd out of interleaved output.
func (o *Orchestrator) Sync(ctx context.Context, req *SyncRequest) (*SyncResult, error) {
	if req == nil {
		return nil, errors.New("sync request cannot be nil")
	}
	if req.Provider == "" || req.Owner == "" || req.Repo == "" {
		return nil, errors.New("provider, owner, and repo are required")
	}
	started := time.Now()
	syncID := shortID()
	log := logging.FromContext(ctx).With("sync_id", syncID,
		"repo", req.Owner+"/"+req.Repo, "provider", req.Provider)
	ctx = logging.WithLogger(ctx, log)
	result := &SyncResult{Errors: make([]string, 0)}
	log.Info("sync started")
	o.emitter.Emit(&events.Event{
		Type: "sync_started",
		Data: map[string]interface{}{
			"provider": req.Provider, "owner": req.Owner, "repo": req.Repo, "sync_id": syncID,
		},
		Timestamp: time.Now(),
	})

	fetchedPRs := o.syncListPhase(ctx, req, result)
	fetchedPRs = o.syncDetailsPhase(ctx, req, fetchedPRs, result)
	o.syncWritePhase(ctx, fetchedPRs, result)

	result.Duration = time.Since(started)
	log.Info("sync complete", "duration_ms", result.Duration.Milliseconds(),
		"prs_total", result.PRCount, "prs_updated", result.UpdatedPRs,
		"comments", result.CommentCount, "errors", len(result.Errors))
	o.emitter.Emit(events.NewCompleteEvent(result.PRCount, result.NewPRs, result.UpdatedPRs,
		len(result.Errors), result.Duration))
	return result, nil
}

func (o *Orchestrator) syncListPhase(ctx context.Context, req *SyncRequest, result *SyncResult) []*PRFetchResult {
	log := logging.FromContext(ctx)
	if !req.IncludePRs {
		log.Debug("phase list-prs skipped (include_prs=false)")
		return nil
	}
	log.Debug("phase list-prs start")
	started := time.Now()
	o.emitter.Emit(events.NewProgressEvent("fetching_prs", 0, 0))
	prs, err := o.syncPRList(ctx, req)
	phaseDuration := time.Since(started).Milliseconds()
	if err != nil {
		o.recordSyncPhaseError(log, result, syncPhaseError{phase: "phase list-prs failed", message: "failed to sync PRs", cause: err, durationMS: phaseDuration})
		return nil
	}
	result.PRCount = len(prs)
	fetchedPRs := make([]*PRFetchResult, len(prs))
	for i, pr := range prs {
		fetchedPRs[i] = &PRFetchResult{PR: pr}
	}
	log.Info("phase list-prs done", "count", len(prs), "duration_ms", phaseDuration)
	return fetchedPRs
}

func (o *Orchestrator) syncDetailsPhase(ctx context.Context, req *SyncRequest, fetchedPRs []*PRFetchResult, result *SyncResult) []*PRFetchResult {
	log := logging.FromContext(ctx)
	// Busy repositories use list-only rows, hydrated on demand by SyncPR.
	if len(fetchedPRs) > hydrationThreshold {
		log.Info("phase fetch-details skipped (list-only)", "pr_count", len(fetchedPRs), "threshold", hydrationThreshold)
		for _, fetched := range fetchedPRs {
			fetched.ListOnly = true
		}
		result.UpdatedPRs = len(fetchedPRs)
		return fetchedPRs
	}
	if len(fetchedPRs) == 0 {
		return fetchedPRs
	}
	log.Debug("phase fetch-details start", "pr_count", len(fetchedPRs),
		"include_comments", req.IncludeComments, "include_ci", req.IncludeCI, "include_diff", req.IncludeDiff)
	started := time.Now()
	o.emitter.Emit(events.NewProgressEvent("fetching_details", 0, len(fetchedPRs)))
	prs := make([]*provider.PR, len(fetchedPRs))
	for i, fetched := range fetchedPRs {
		prs[i] = fetched.PR
	}
	details, err := o.FetchPRDetails(ctx, prs, req.IncludeComments, req.IncludeCI, req.IncludeDiff)
	phaseDuration := time.Since(started).Milliseconds()
	if err != nil {
		o.recordSyncPhaseError(log, result, syncPhaseError{phase: "phase fetch-details failed", message: "failed to fetch details", cause: err, durationMS: phaseDuration})
		return fetchedPRs
	}
	for _, detail := range details {
		if detail.Error == nil {
			result.UpdatedPRs++
			result.CommentCount += len(detail.Comments)
		} else {
			result.Errors = append(result.Errors, detail.Error.Error())
		}
	}
	log.Info("phase fetch-details done", "duration_ms", phaseDuration,
		"pr_ok", result.UpdatedPRs, "pr_errors", len(details)-result.UpdatedPRs)
	return details
}

func (o *Orchestrator) syncWritePhase(ctx context.Context, fetchedPRs []*PRFetchResult, result *SyncResult) {
	if len(fetchedPRs) == 0 {
		return
	}
	log := logging.FromContext(ctx)
	log.Debug("phase write-db start", "pr_count", len(fetchedPRs))
	started := time.Now()
	o.emitter.Emit(events.NewProgressEvent("writing_database", 0, len(fetchedPRs)))
	writer := NewWriter(o.log, o.store).WithEmitter(o.emitter).WithClassifier(o.classifier).WithCurrentUser(o.currentUser).WithPostCommit(o.postCommit)
	if err := writer.WriteSyncResultsContext(ctx, fetchedPRs); err != nil {
		o.recordSyncPhaseError(log, result, syncPhaseError{phase: "phase write-db failed", message: "failed to write results", cause: err, durationMS: time.Since(started).Milliseconds()})
		return
	}
	log.Info("phase write-db done", "duration_ms", time.Since(started).Milliseconds())
}

func (o *Orchestrator) recordSyncPhaseError(log *slog.Logger, result *SyncResult, failure syncPhaseError) {
	errMsg := fmt.Sprintf("%s: %v", failure.message, failure.cause)
	result.Errors = append(result.Errors, errMsg)
	log.Error(failure.phase, "duration_ms", failure.durationMS, "error", failure.cause)
	o.emitter.Emit(events.NewErrorEvent(errMsg))
}

// SyncPR fetches full details (comments, CI, diff) for a single PR and writes
// them, marking the row hydrated. Used for on-demand hydration when the user
// selects a list-only PR from a very active repo.
func (o *Orchestrator) SyncPR(ctx context.Context, prID string) error {
	if o.provider == nil {
		return fmt.Errorf("provider not configured")
	}
	if o.store == nil {
		return fmt.Errorf("store not configured")
	}

	log := logging.FromContext(ctx).With("pr_id", prID)
	ctx = logging.WithLogger(ctx, log)

	pr, err := o.provider.GetPR(ctx, prID)
	if err != nil {
		return fmt.Errorf("get PR %s: %w", prID, err)
	}

	results, err := o.FetchPRDetails(ctx, []*provider.PR{pr}, true, true, true)
	if err != nil {
		return fmt.Errorf("fetch details for %s: %w", prID, err)
	}
	if len(results) == 1 && results[0] != nil && results[0].Error != nil {
		return fmt.Errorf("hydrate %s: %w", prID, results[0].Error)
	}

	writer := NewWriter(o.log, o.store).WithEmitter(o.emitter).WithClassifier(o.classifier).WithCurrentUser(o.currentUser).WithPostCommit(o.postCommit)
	if err := writer.WriteSyncResultsContext(ctx, results); err != nil {
		return fmt.Errorf("write hydrated PR %s: %w", prID, err)
	}
	log.Info("pr hydrated on demand")
	return nil
}

// DiscoverAndSyncInbox runs the local-first inbox discovery lane: GitHub search
// finds lightweight `involves:USERNAME` PR pointers, then provider repo PR
// endpoints hydrate canonical PR rows before SQLite is updated. The local-only
// high-water cursor advances only after detail hydration and DB writes succeed.
func (o *Orchestrator) DiscoverAndSyncInbox(ctx context.Context) (*SyncResult, error) {
	discovery, ok := o.provider.(provider.InboxDiscoveryProvider)
	if !ok {
		return &SyncResult{}, nil
	}
	if o.store == nil {
		return nil, errors.New("store not configured")
	}
	username, err := o.provider.GetUsername(ctx)
	if err != nil {
		return nil, fmt.Errorf("get username for inbox discovery: %w", err)
	}
	scope := githubProvider + ":inbox:" + username + ":search:involves"
	started := time.Now().UTC()
	options, err := o.inboxDiscoveryOptions(ctx, scope, username)
	if err != nil {
		return nil, err
	}
	discovered, err := discovery.DiscoverInboxPRs(ctx, options)
	if err != nil {
		return nil, o.recordInboxCursorError(ctx, scope, err)
	}
	if discovered == nil {
		discovered = &provider.InboxDiscoveryResult{}
	}
	result := &SyncResult{PRCount: len(discovered.PRs)}
	if len(discovered.PRs) > 0 {
		if err := o.writeDiscoveredPRs(ctx, scope, discovered.PRs, result); err != nil {
			return nil, err
		}
		if len(result.Errors) > 0 {
			o.recordInboxCursorError(ctx, scope,
				fmt.Errorf("inbox discovery had %d per-PR hydration errors", len(result.Errors)))
			return result, nil
		}
	}
	highWater := discovered.HighWater
	if highWater.IsZero() {
		highWater = started
	}
	if err := o.store.SaveSyncCursorSuccess(ctx, scope, store.FormatSyncCursorTime(highWater), discovered.ETag); err != nil {
		return nil, fmt.Errorf("save inbox cursor: %w", err)
	}
	return result, nil
}

func (o *Orchestrator) inboxDiscoveryOptions(ctx context.Context, scope, username string) (provider.InboxDiscoveryOptions, error) {
	options := provider.InboxDiscoveryOptions{Username: username, Limit: inboxDiscoveryLimit}
	cursor, err := o.store.GetSyncCursor(ctx, scope)
	if err != nil {
		return options, fmt.Errorf("load inbox cursor: %w", err)
	}
	if cursor == nil {
		return options, nil
	}
	options.ETag = cursor.ETag
	if cursor.CursorValue == "" {
		return options, nil
	}
	parsed, err := store.ParseSyncCursorTime(cursor.CursorValue)
	if err != nil {
		return options, fmt.Errorf("parse inbox cursor %q: %w", cursor.CursorValue, err)
	}
	lagged := parsed.Add(-inboxCursorLag)
	options.Since = &lagged
	return options, nil
}

func (o *Orchestrator) writeDiscoveredPRs(ctx context.Context, scope string, prs []*provider.PR, result *SyncResult) error {
	details, err := o.FetchPRDetails(ctx, prs, true, true, true)
	if err != nil {
		return o.recordInboxCursorError(ctx, scope, err)
	}
	for _, detail := range details {
		if detail == nil {
			continue
		}
		if detail.Error != nil {
			result.Errors = append(result.Errors, detail.Error.Error())
			continue
		}
		result.UpdatedPRs++
		result.CommentCount += len(detail.Comments)
	}
	writer := NewWriter(o.log, o.store).WithEmitter(o.emitter).WithClassifier(o.classifier).WithCurrentUser(o.currentUser).WithPostCommit(o.postCommit)
	if err := writer.WriteSyncResultsContext(ctx, details); err != nil {
		return o.recordInboxCursorError(ctx, scope, err)
	}
	return nil
}

// Cursor failure recording is best-effort: preserve the original sync failure
// while surfacing a failed local error write to the application log.
func (o *Orchestrator) recordInboxCursorError(ctx context.Context, scope string, original error) error {
	if err := o.store.MarkSyncCursorError(ctx, scope, original); err != nil {
		logging.FromContext(ctx).Warn("record inbox cursor error failed", "error", err)
	}
	return original
}

// syncPRList fetches and stores the PR list
func (o *Orchestrator) syncPRList(ctx context.Context, req *SyncRequest) ([]*provider.PR, error) {
	log := logging.FromContext(ctx)
	if o.provider == nil {
		return nil, errors.New("provider not configured")
	}

	updatedSince := time.Now().Add(-prListWindow)
	filters := provider.PRFilters{
		Owner:        req.Owner,
		Repo:         req.Repo,
		State:        req.State,
		UpdatedSince: &updatedSince,
		MinCount:     prListMinCount,
	}

	log.Debug("calling provider.ListPRs",
		"state", filters.State,
		"updated_since", updatedSince,
		"min_count", filters.MinCount,
	)
	start := time.Now()
	prs, err := o.provider.ListPRs(ctx, filters)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		if isRateLimitError(err) {
			state := &provider.RateLimitState{
				ResetAt: time.Now().Add(defaultRateLimitCooldown),
			}
			o.rateLimitMu.Lock()
			o.rateLimitState = state
			o.rateLimitMu.Unlock()
			log.Warn("rate limited",
				"reset_at", state.ResetAt,
				"duration_ms", dur,
			)
			o.emitter.Emit(&events.Event{
				Type: "rate_limited",
				Data: map[string]interface{}{
					"reset_in": state.ResetIn().Seconds(),
				},
				Timestamp: time.Now(),
			})
			return nil, err
		}
		return nil, fmt.Errorf("list PRs: %w", err)
	}

	log.Info("prs fetched", "count", len(prs), "duration_ms", dur)

	return prs, nil
}
