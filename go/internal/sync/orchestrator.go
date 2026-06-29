package sync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	"log/slog"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// Orchestrator coordinates sync operations
type Orchestrator struct {
	log            *slog.Logger
	provider       provider.Provider
	store          *store.DB
	git            *git.Client
	emitter        *events.EventEmitter
	rateLimitMu    gosync.RWMutex
	rateLimitState *provider.RateLimitState
	queue          *store.ActionQueue
	diffStore      *store.DiffStore
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

// resolveAnchors runs AnchorResolver.Resolve on every eligible review comment.
//
// Geometry rules:
//   - Current-position comments (Position != nil, > 0): use prBaseSHA as
//     DiffBaseSHA — correct for the current patchset.
//   - Original-position comments (Position == nil, OriginalPosition > 0):
//     require EXACT patchset identity via patchset_number:
//     1. Use c.PatchsetNumber if set (populated by a prior DB write).
//     2. Otherwise look up the stored patchset_number from the DB by comment ID.
//     3. Find the patchset row by exact number match; use its BaseSHA.
//     If patchset_number is zero/missing, or not found in patchsets list → SKIP.
//     Head-SHA-only matching is never used for original-position comments.
//
// All errors are logged at Debug/Warn level; affected comments are skipped.
func (o *Orchestrator) resolveAnchors(
	ctx context.Context,
	comments []*provider.Comment,
	threads []*provider.ReviewThread,
	prID, localPath, headSHA, prBaseSHA string,
	log *slog.Logger,
) []git.AnchorResult {
	var candidates []*provider.Comment
	for _, c := range comments {
		if c != nil && c.Kind == "review" && c.FilePath != "" && isAnchorable(c) {
			candidates = append(candidates, c)
		}
	}
	for _, th := range threads {
		if th == nil {
			continue
		}
		for _, c := range th.Comments {
			if c != nil && c.Kind == "review" && c.FilePath != "" && isAnchorable(c) {
				candidates = append(candidates, c)
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Load all known patchsets for this PR.
	var patchsets []store.Patchset
	if o.store != nil && prID != "" {
		var loadErr error
		patchsets, loadErr = o.store.ListPatchsets(ctx, prID)
		if loadErr != nil {
			log.Warn("resolveAnchors: could not load patchsets; original-position comments will be skipped",
				"pr_id", prID, "error", loadErr)
		}
	}

	// Find the current patchset number for (prBaseSHA, headSHA).
	// This is anchor_current_patchset_number — the state the anchor is valid against.
	// May be 0 for first sync (patchset not yet in DB); writeAnchorUpdates will fill
	// it in from currentPatchsetNum after recordPatchsetIfChanged runs.
	currentPsNumForAnchors := 0
	for _, ps := range patchsets {
		if ps.BaseSHA == prBaseSHA && ps.HeadSHA == headSHA {
			currentPsNumForAnchors = ps.Number
			break
		}
	}

	// For original-position candidates that lack c.PatchsetNumber, preload the
	// stored patchset_number from the DB (set during a prior writeComment call).
	storedPatchsetNums := map[string]int{}
	if o.store != nil && prID != "" {
		var needLookup []string
		for _, c := range candidates {
			if (c.PatchsetNumber == nil || *c.PatchsetNumber == 0) &&
				(c.Position == nil || *c.Position == 0) {
				needLookup = append(needLookup, c.ID)
			}
		}
		if len(needLookup) > 0 {
			loaded, err := o.store.GetCommentPatchsetNumbers(ctx, needLookup)
			if err != nil {
				log.Warn("resolveAnchors: could not load comment patchset_numbers; affected original-position comments will be skipped",
					"error", err)
			} else {
				storedPatchsetNums = loaded
			}
		}
	}

	results := make([]git.AnchorResult, 0, len(candidates))
	for _, c := range candidates {
		side := c.Side
		if side == "" {
			side = "RIGHT"
		}

		isCurrentPos := c.Position != nil && *c.Position > 0

		commitSHA := c.CommitSHA
		position := 0
		if isCurrentPos {
			position = *c.Position
		} else {
			if c.OriginalPosition != nil && *c.OriginalPosition > 0 {
				position = *c.OriginalPosition
			}
			if c.OriginalSHA != "" {
				commitSHA = c.OriginalSHA
			}
		}
		if commitSHA == "" || position == 0 {
			log.Debug("anchor skip: no usable commit SHA or position", "comment_id", c.ID)
			continue
		}

		// Resolve the patchset_number for this comment.
		patchsetNum := 0
		if c.PatchsetNumber != nil {
			patchsetNum = *c.PatchsetNumber
		} else if n, ok := storedPatchsetNums[c.ID]; ok {
			patchsetNum = n
		}

		diffBaseSHA, resolvedPatchsetNum, skip := selectDiffBaseSHA(patchsets, patchsetNum, isCurrentPos, prBaseSHA, headSHA)
		if skip {
			log.Debug("anchor skip: original-position comment, patchset identity unavailable or not found",
				"comment_id", c.ID, "patchset_num", patchsetNum)
			continue
		}

		req := git.ResolveRequest{
			LocalPath:   localPath,
			CommitSHA:   commitSHA,
			HeadSHA:     headSHA,
			DiffBaseSHA: diffBaseSHA,
			FilePath:    c.FilePath,
			Side:        side,
			Position:    position,
			CommentID:   c.ID,
			PatchsetNum: 0,  // unknown at fetch time; CommitSHA==HeadSHA fast path handles it
			CurrentNum:  -1, // prevents 0==0 patchset fast path
		}
		res, err := o.anchorResolver.Resolve(req)
		if err != nil {
			log.Debug("anchor resolve failed (best-effort, skipping)",
				"comment_id", c.ID, "error", err)
			continue
		}
		// PatchsetNumber = source patchset (geometry used for position-to-line mapping).
		// CurrentPatchsetNumber = current PR patchset at resolution time (the state the
		// anchor is valid against). Both are needed for full freshness validation.
		res.PatchsetNumber = resolvedPatchsetNum
		res.CurrentPatchsetNumber = currentPsNumForAnchors
		results = append(results, res)
	}
	return results
}

// selectDiffBaseSHA returns the correct DiffBaseSHA and resolved patchset number
// for a ResolveRequest, and whether the comment should be skipped.
//
//   - Current-position: prBaseSHA is the correct geometry.
//     Looks up the current patchset by (headSHA, prBaseSHA) to get its number.
//     Returns (prBaseSHA, patchset.Number, false), or (prBaseSHA, 0, false) if
//     the patchset row is not yet recorded (new PR, first sync).
//   - Original-position: requires exact patchset number identity.
//     patchsetNum > 0 → find patchset by exact number; return its BaseSHA and Number.
//     patchsetNum == 0 or not found → ("", 0, true) — skip, never approximate.
//
// Head-SHA-only matching is intentionally absent: the same head can appear
// under multiple bases (retarget/rebase), making head-only matching ambiguous.
func selectDiffBaseSHA(patchsets []store.Patchset, patchsetNum int, isCurrentPos bool, prBaseSHA, prHeadSHA string) (diffBase string, resolvedPatchsetNum int, skip bool) {
	if isCurrentPos {
		// Look up the current patchset to get its number for anchor identity.
		for _, ps := range patchsets {
			if ps.BaseSHA == prBaseSHA && ps.HeadSHA == prHeadSHA {
				return prBaseSHA, ps.Number, false
			}
		}
		// Patchset not yet recorded (new PR) — still valid, number unknown.
		return prBaseSHA, 0, false
	}
	// Original-position: require exact patchset_number identity.
	if patchsetNum <= 0 {
		return "", 0, true // no patchset_number recorded → skip
	}
	for _, ps := range patchsets {
		if ps.Number == patchsetNum {
			return ps.BaseSHA, ps.Number, false
		}
	}
	return "", 0, true // patchset number not found in DB → skip
}

// isAnchorable reports whether a review comment has enough information to
// attempt anchor resolution. It accepts both current-position comments and
// outdated/rebased comments that carry only original-position data.
func isAnchorable(c *provider.Comment) bool {
	hasCurrent := c.CommitSHA != "" && c.Position != nil && *c.Position > 0
	hasOriginal := c.OriginalPosition != nil && *c.OriginalPosition > 0
	return hasCurrent || hasOriginal
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

// validateThreadPreflight checks thread ownership and state_known before any
// provider mutation. Returns a non-nil *ActionResult on failure (caller must
// return it); returns nil when validation passes or is not applicable.
// Shared between the immediate ApplyAction path and the queue replay path so
// both enforce the same invariants.
func (o *Orchestrator) validateThreadPreflight(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	switch req.Action {
	case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
		if o.store == nil {
			return nil // no store — skip validation (test/noop path)
		}
		thread, err := o.store.GetReviewThread(ctx, req.PRID, req.Body)
		if err != nil {
			return &actions.ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     fmt.Sprintf("thread lookup failed: %v", err),
				Timestamp: time.Now(),
			}
		}
		if thread == nil {
			return &actions.ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     fmt.Sprintf("thread %s not found in pr %s", req.Body, req.PRID),
				Timestamp: time.Now(),
			}
		}
		if !thread.StateKnown {
			return &actions.ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     fmt.Sprintf("thread %s state is unknown; sync required before mutation", req.Body),
				Timestamp: time.Now(),
			}
		}
	}
	return nil
}

// prepareReplyCommentTarget resolves thread-oriented reply requests to a
// concrete GitHub review-comment ID before provider execution or queueing.
// The UI is thread-oriented, while GitHub's REST reply endpoint requires a
// review-comment database ID.
func (o *Orchestrator) prepareReplyCommentTarget(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	if req.Action != actions.ReplyCommentCommand || req.InReplyTo != "" || req.ThreadID == "" {
		return nil
	}
	if o.store == nil {
		return nil
	}

	targetID, err := o.store.ReplyTargetCommentIDForThread(ctx, req.PRID, req.ThreadID)
	if err != nil {
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("reply target lookup failed: %v", err),
			Timestamp: time.Now(),
		}
	}
	if targetID == "" {
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("thread %s has no cached review comment target in pr %s", req.ThreadID, req.PRID),
			Timestamp: time.Now(),
		}
	}
	req.InReplyTo = targetID
	return nil
}

// ApplyAction executes an action immediately or queues it when rate limited.
func (o *Orchestrator) ApplyAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	log := logging.FromContext(ctx).With(
		"pr_id", req.PRID,
		"action", string(req.Action),
	)
	ctx = logging.WithLogger(ctx, log)

	// Reject immediately if the orchestrator's provider has been disconnected or
	// replaced. This handles the race where a handler captured this orchestrator
	// before disconnect and calls ApplyAction after.
	if o.liveness != nil {
		if err := o.liveness.Err(); err != nil {
			return &actions.ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     "provider disconnected: action rejected",
				Timestamp: time.Now(),
			}
		}
	}

	// Validate thread ownership and state before any provider mutation or queue.
	if result := o.validateThreadPreflight(ctx, req); result != nil {
		return result
	}
	if result := o.prepareReplyCommentTarget(ctx, req); result != nil {
		return result
	}

	if o.IsRateLimited() {
		log.Info("rate limited, queueing action")

		// Reject line-level post_comment before queueing: the queue payload only
		// stores Body, not Line, so replay would silently drop the line context
		// and post a general PR comment instead of the intended inline comment.
		// Return a clear error so the caller can retry after the rate limit clears.
		if req.Action == actions.PostCommentCommand && req.Line > 0 {
			return &actions.ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     "line-level post_comment cannot be queued for replay (Line context would be lost); retry after rate limit clears",
				Timestamp: time.Now(),
			}
		}

		payload := req.Body
		if req.Action == actions.ReplyCommentCommand {
			encoded, err := json.Marshal(actions.ReplyCommentPayload{Body: req.Body, InReplyTo: req.InReplyTo, ThreadID: req.ThreadID})
			if err != nil {
				return &actions.ActionResult{
					PRID:      req.PRID,
					Action:    req.Action,
					Success:   false,
					Error:     fmt.Sprintf("failed to encode reply_comment payload: %v", err),
					Timestamp: time.Now(),
				}
			}
			payload = string(encoded)
		}

		queuedAction := &store.QueuedAction{
			// Include Body in the ID so that multiple thread actions on the same
			// PR (e.g. resolve thread-A and resolve thread-B) get distinct rows
			// rather than overwriting each other. For actions without a body
			// (e.g. approve) the ID remains pr_id+action, preserving idempotence.
			ID:         queuedActionID(req),
			ActionType: string(req.Action),
			PRID:       req.PRID,
			Payload:    payload,
		}
		if o.queue != nil {
			if err := o.queue.Enqueue(ctx, queuedAction); err != nil {
				log.Error("failed to queue action", "error", err)
				return &actions.ActionResult{
					PRID:      req.PRID,
					Action:    req.Action,
					Success:   false,
					Error:     err.Error(),
					Timestamp: time.Now(),
				}
			}
			log.Debug("action queued", "queue_id", queuedAction.ID)
			return &actions.ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     "rate limited: action queued for later replay",
				Timestamp: time.Now(),
			}
		}
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     "rate limited: no action queue configured, action dropped",
			Timestamp: time.Now(),
		}
	}

	return o.executeAction(ctx, req)
}

// executeAction runs an action via the executor immediately.
func (o *Orchestrator) executeAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	log := logging.FromContext(ctx)
	log.Debug("executing action", "pr_id", req.PRID, "action", string(req.Action))

	// Derive an execution context that is cancelled if either the caller's ctx
	// OR the orchestrator's liveness context is cancelled. This ensures that a
	// provider disconnect mid-retry interrupts ExecuteWithRetry rather than
	// letting it complete with stale credentials.
	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()
	if o.liveness != nil {
		go func() {
			select {
			case <-o.liveness.Done():
				execCancel()
			case <-execCtx.Done():
				// caller cancelled first or action completed — nothing to do
			}
		}()
	}

	executor := actions.NewActionExecutor(o.log, o.provider, o.store, o.emitter).
		WithReviewTracking(o.reviewLogin, o.reviewSHAWriter)
	start := time.Now()
	result, err := executor.ExecuteWithRetry(execCtx, req, 3)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		log.Error("action execution error",
			"pr_id", req.PRID,
			"action", string(req.Action),
			"duration_ms", dur,
			"error", err,
		)
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		}
	}
	if result == nil {
		log.Error("action executor returned nil result",
			"pr_id", req.PRID,
			"action", string(req.Action),
			"duration_ms", dur,
		)
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     "executor returned nil result",
			Timestamp: time.Now(),
		}
	}
	log.Debug("action executed",
		"pr_id", req.PRID,
		"action", string(req.Action),
		"duration_ms", dur,
		"success", result.Success,
	)

	// Post-action local state update and event emission for thread commands.
	if result.Success {
		switch req.Action {
		case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
			o.handleThreadActionSuccess(ctx, req, log)
		case actions.ReplyCommentCommand:
			o.handleReplyCommentSuccess(req)
		}
	}

	return result
}

// handleReplyCommentSuccess emits a refresh hint after a successful reply. The
// created GitHub comment is not synchronously written to SQLite, so consumers
// should re-request/sync comments to observe the new reply.
func (o *Orchestrator) handleReplyCommentSuccess(req *actions.ActionRequest) {
	if o.emitter == nil {
		return
	}
	o.emitter.Emit(&events.Event{
		Type: "COMMENT_POSTED",
		Data: map[string]interface{}{
			"pr_id":       req.PRID,
			"thread_id":   req.ThreadID,
			"in_reply_to": req.InReplyTo,
		},
		Timestamp: time.Now(),
	})
}

// handleThreadActionSuccess updates local thread state and emits a WS event
// after a successful resolve/unresolve action. Best-effort: failures are
// logged but do not affect the returned ActionResult.
func (o *Orchestrator) handleThreadActionSuccess(ctx context.Context, req *actions.ActionRequest, log *slog.Logger) {
	// If the provider was disconnected/replaced during or after execution,
	// skip local SQLite writes and event emission. The next sync cycle after
	// re-auth will reconcile state from GitHub; a stale THREAD_RESOLVED event
	// broadcast to a new session would be misleading.
	if o.liveness != nil && o.liveness.Err() != nil {
		log.Debug("handleThreadActionSuccess: liveness cancelled, skipping local write/event",
			"thread_id", req.Body)
		return
	}

	resolved := req.Action == actions.ResolveThreadCommand
	threadID := req.Body

	// Update local SQLite cache so REVIEW_THREADS_READY reflects the new state
	// without waiting for the next full sync cycle.
	if o.store != nil {
		if err := o.store.SetThreadResolved(ctx, req.PRID, threadID, resolved); err != nil {
			log.Warn("handleThreadActionSuccess: SetThreadResolved failed",
				"thread_id", threadID, "resolved", resolved, "error", err)
		}
	}

	// Emit event for WS broadcast.
	if o.emitter != nil {
		eventType := "THREAD_RESOLVED"
		if !resolved {
			eventType = "THREAD_REOPENED"
		}
		o.emitter.Emit(&events.Event{
			Type: eventType,
			Data: map[string]interface{}{
				"thread_id": threadID,
				"pr_id":     req.PRID,
				"resolved":  resolved,
			},
			Timestamp: time.Now(),
		})
	}
}

// queuedActionID returns a stable unique ID for a queued action.
// For thread actions (resolve/unresolve), Body is the thread node ID — short
// and stable — so it is included directly.
// For all other actions, if Body is non-empty it is hashed (sha256 prefix) so
// that arbitrary user text does not appear raw in the queue row ID.
// Actions without a body keep their deduplication behaviour (pr_id+action).
func queuedActionID(req *actions.ActionRequest) string {
	if req.Body == "" {
		return req.PRID + "-" + string(req.Action)
	}
	switch req.Action {
	case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
		// Body is a thread node ID: stable, safe to use directly.
		return req.PRID + "-" + string(req.Action) + "-" + req.Body
	case actions.ReplyCommentCommand:
		// Include both the target and body without leaking arbitrary user text.
		h := sha256.Sum256([]byte(req.InReplyTo + "\x00" + req.Body))
		return req.PRID + "-" + string(req.Action) + "-" + hex.EncodeToString(h[:8])
	default:
		// Hash arbitrary body text to keep the ID compact and free of raw user content.
		h := sha256.Sum256([]byte(req.Body))
		return req.PRID + "-" + string(req.Action) + "-" + hex.EncodeToString(h[:8])
	}
}

// ReplayQueuedActions replays pending actions after a rate limit clears.
func (o *Orchestrator) ReplayQueuedActions(ctx context.Context) error {
	if o.queue == nil {
		o.log.Debug("skipping queue replay: no queue configured")
		return nil
	}

	batchID := shortID()
	log := logging.FromContext(ctx).With("replay_batch", batchID)
	ctx = logging.WithLogger(ctx, log)

	pending, err := o.queue.GetPending(ctx, 10)
	if err != nil {
		log.Error("failed to load pending queue", "error", err)
		return err
	}

	log.Info("replaying queued actions", "count", len(pending))

	for _, action := range pending {
		log.Info("replaying queued action",
			"id", action.ID,
			"pr_id", action.PRID,
			"action", action.ActionType,
			"retry_count", action.RetryCount,
		)

		req := &actions.ActionRequest{
			PRID:   action.PRID,
			Action: actions.ActionCommand(action.ActionType),
			Body:   action.Payload,
		}
		if req.Action == actions.ReplyCommentCommand {
			var payload actions.ReplyCommentPayload
			if err := json.Unmarshal([]byte(action.Payload), &payload); err != nil {
				log.Warn("replay: invalid reply_comment payload, marking failed",
					"id", action.ID, "error", err)
				if markErr := o.queue.MarkFailed(ctx, action.ID, err.Error(), store.MaxRetryCount); markErr != nil {
					log.Error("failed to mark action failed after payload decode", "id", action.ID, "error", markErr)
				}
				continue
			}
			req.Body = payload.Body
			req.InReplyTo = payload.InReplyTo
			req.ThreadID = payload.ThreadID
		}

		// Re-validate thread ownership and state_known before executing.
		// Queued actions bypass the live ApplyAction path, so we must re-run
		// the same preflight checks here to prevent stale/wrong-PR mutations.
		if preflightErr := o.validateThreadPreflight(ctx, req); preflightErr != nil {
			log.Warn("replay: thread preflight failed, marking failed",
				"id", action.ID, "reason", preflightErr.Error)
			if err := o.queue.MarkFailed(ctx, action.ID, preflightErr.Error, store.MaxRetryCount); err != nil {
				log.Error("failed to mark action failed after preflight", "id", action.ID, "error", err)
			}
			continue
		}

		result := o.executeAction(ctx, req)
		if result.Success {
			if err := o.queue.MarkCompleted(ctx, action.ID); err != nil {
				log.Error("failed to mark action completed", "id", action.ID, "error", err)
			}
		} else {
			errMsg := result.Error
			if errMsg == "" {
				errMsg = "unknown error"
			}
			// If the result carries the ambiguous-remote-success sentinel, the
			// provider may have already mutated GitHub state. Mark the row
			// permanently failed (MaxRetryCount) so it is never replayed again —
			// retrying would risk duplicate mutations for non-idempotent actions
			// like post_comment and request_changes.
			//
			// Use a fresh context for the terminal write: the execution context
			// (ctx) may already be cancelled (that is precisely how the ambiguous
			// state arises), and an ExecContext call on a cancelled context fails,
			// which would leave the row pending and allow a duplicate mutation on
			// the next replay pass.
			retryCount := action.RetryCount + 1
			markCtx := ctx
			if result.IsAmbiguousRemoteSuccess() {
				log.Warn("replay: ambiguous remote success, marking terminal to prevent duplicate mutation",
					"id", action.ID, "action", action.ActionType)
				retryCount = store.MaxRetryCount
				markCtx = context.Background()
			}
			if err := o.queue.MarkFailed(markCtx, action.ID, errMsg, retryCount); err != nil {
				log.Error("failed to mark action failed", "id", action.ID, "error", err)
			}
		}
	}

	return nil
}

// CleanupQueue removes old completed and failed actions from the queue.
func (o *Orchestrator) CleanupQueue(ctx context.Context) error {
	if o.queue == nil {
		return nil
	}

	log := logging.FromContext(ctx)
	deleted, err := o.queue.DeleteCompleted(ctx, 7*24*time.Hour)
	if err != nil {
		log.Error("queue cleanup (completed) failed", "error", err)
		return err
	}
	log.Debug("cleaned up completed queue entries", "deleted", deleted)

	if err := o.queue.DeleteFailed(ctx, 24*time.Hour); err != nil {
		log.Error("queue cleanup (failed) failed", "error", err)
		return err
	}
	return nil
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

// Sync executes a full sync operation. All log records emitted downstream are
// tagged with a per-cycle sync_id plus the repo slug so a full sync can be
// grep'd out of interleaved output.
func (o *Orchestrator) Sync(ctx context.Context, req *SyncRequest) (*SyncResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync request cannot be nil")
	}

	if req.Provider == "" || req.Owner == "" || req.Repo == "" {
		return nil, fmt.Errorf("provider, owner, and repo are required")
	}

	syncID := shortID()
	repoSlug := req.Owner + "/" + req.Repo
	log := logging.FromContext(ctx).With(
		"sync_id", syncID,
		"repo", repoSlug,
		"provider", req.Provider,
	)
	ctx = logging.WithLogger(ctx, log)

	startTime := time.Now()
	result := &SyncResult{
		Errors: make([]string, 0),
	}

	log.Info("sync started")
	o.emitter.Emit(&events.Event{
		Type: "sync_started",
		Data: map[string]interface{}{
			"provider": req.Provider,
			"owner":    req.Owner,
			"repo":     req.Repo,
			"sync_id":  syncID,
		},
		Timestamp: time.Now(),
	})

	// Phase 1: Fetch PR list
	var fetchedPRs []*PRFetchResult
	if req.IncludePRs {
		log.Debug("phase list-prs start")
		phaseStart := time.Now()
		o.emitter.Emit(events.NewProgressEvent("fetching_prs", 0, 0))

		prs, err := o.syncPRList(ctx, req)
		phaseDur := time.Since(phaseStart).Milliseconds()
		if err != nil {
			errMsg := fmt.Sprintf("failed to sync PRs: %v", err)
			result.Errors = append(result.Errors, errMsg)
			log.Error("phase list-prs failed", "duration_ms", phaseDur, "error", err)
			o.emitter.Emit(events.NewErrorEvent(errMsg))
		} else {
			result.PRCount = len(prs)
			fetchedPRs = make([]*PRFetchResult, len(prs))
			for i, pr := range prs {
				fetchedPRs[i] = &PRFetchResult{PR: pr}
			}
			log.Info("phase list-prs done",
				"count", len(prs),
				"duration_ms", phaseDur,
			)
		}
	} else {
		log.Debug("phase list-prs skipped (include_prs=false)")
	}

	// Phase 2: Fetch details concurrently (comments, CI, diffs)
	if len(fetchedPRs) > 0 {
		log.Debug("phase fetch-details start",
			"pr_count", len(fetchedPRs),
			"include_comments", req.IncludeComments,
			"include_ci", req.IncludeCI,
			"include_diff", req.IncludeDiff,
		)
		phaseStart := time.Now()
		o.emitter.Emit(events.NewProgressEvent("fetching_details", 0, len(fetchedPRs)))

		// Extract the PR slice from the Phase 1 results to avoid a second ListPRs call.
		prSlice := make([]*provider.PR, len(fetchedPRs))
		for i, r := range fetchedPRs {
			prSlice[i] = r.PR
		}
		detailResults, err := o.FetchPRDetails(ctx, prSlice, req.IncludeComments, req.IncludeCI, req.IncludeDiff)
		phaseDur := time.Since(phaseStart).Milliseconds()
		if err != nil {
			errMsg := fmt.Sprintf("failed to fetch details: %v", err)
			result.Errors = append(result.Errors, errMsg)
			log.Error("phase fetch-details failed", "duration_ms", phaseDur, "error", err)
			o.emitter.Emit(events.NewErrorEvent(errMsg))
		} else {
			fetchedPRs = detailResults
			for _, r := range detailResults {
				if r.Error == nil {
					result.UpdatedPRs++
					result.CommentCount += len(r.Comments)
				} else {
					result.Errors = append(result.Errors, r.Error.Error())
				}
			}
			log.Info("phase fetch-details done",
				"duration_ms", phaseDur,
				"pr_ok", result.UpdatedPRs,
				"pr_errors", len(detailResults)-result.UpdatedPRs,
			)
		}
	}

	// Phase 3: Write to SQLite (batched transactions)
	if len(fetchedPRs) > 0 {
		log.Debug("phase write-db start", "pr_count", len(fetchedPRs))
		phaseStart := time.Now()
		o.emitter.Emit(events.NewProgressEvent("writing_database", 0, len(fetchedPRs)))

		writer := NewWriter(o.log, o.store)
		if err := writer.WriteSyncResultsContext(ctx, fetchedPRs); err != nil {
			errMsg := fmt.Sprintf("failed to write results: %v", err)
			result.Errors = append(result.Errors, errMsg)
			log.Error("phase write-db failed",
				"duration_ms", time.Since(phaseStart).Milliseconds(),
				"error", err,
			)
			o.emitter.Emit(events.NewErrorEvent(errMsg))
		} else {
			log.Info("phase write-db done",
				"duration_ms", time.Since(phaseStart).Milliseconds(),
			)
		}
	}

	result.Duration = time.Since(startTime)
	log.Info("sync complete",
		"duration_ms", result.Duration.Milliseconds(),
		"prs_total", result.PRCount,
		"prs_updated", result.UpdatedPRs,
		"comments", result.CommentCount,
		"errors", len(result.Errors),
	)

	o.emitter.Emit(events.NewCompleteEvent(
		result.PRCount,
		result.NewPRs,
		result.UpdatedPRs,
		len(result.Errors),
		result.Duration,
	))

	return result, nil
}

// syncPRList fetches and stores the PR list
func (o *Orchestrator) syncPRList(ctx context.Context, req *SyncRequest) ([]*provider.PR, error) {
	log := logging.FromContext(ctx)
	if o.provider == nil {
		log.Error("syncPRList: provider not configured")
		return nil, fmt.Errorf("provider not configured")
	}

	filters := provider.PRFilters{
		Owner: req.Owner,
		Repo:  req.Repo,
		State: req.State,
		Limit: 100,
	}

	log.Debug("calling provider.ListPRs",
		"state", filters.State,
		"limit", filters.Limit,
	)
	start := time.Now()
	prs, err := o.provider.ListPRs(ctx, filters)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		if isRateLimitError(err) {
			state := &provider.RateLimitState{
				ResetAt: time.Now().Add(60 * time.Second),
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
		log.Error("provider.ListPRs failed",
			"duration_ms", dur,
			"state", filters.State,
			"error", err,
		)
		return nil, fmt.Errorf("failed to list PRs: %w", err)
	}

	log.Info("prs fetched", "count", len(prs), "duration_ms", dur)

	return prs, nil
}
