package llm

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
)

const defaultBatchConcurrency = int32(4)

// SettingsSnapshot identifies the provider and model used for classification.
type SettingsSnapshot struct {
	Provider string
	Model    string
}

// Logger records LLM classification failures without logging comment bodies.
type Logger interface {
	Printf(format string, v ...any)
}

// TierConfig supplies local storage, client, and notifications for the LLM tier.
type TierConfig struct {
	DB            *sql.DB
	Client        ClassifierClient
	Mode          func() string
	Settings      func() SettingsSnapshot
	Emit          func(eventType string, data map[string]any)
	ServeComments func(prID string) (any, error)
	Log           Logger
	OnPanic       func(recovered any, stack []byte)
}

// Tier runs optional BYOK classification for unclassified PR comments.
type Tier struct {
	db            *sql.DB
	client        ClassifierClient
	mode          func() string
	settings      func() SettingsSnapshot
	emit          func(eventType string, data map[string]any)
	serveComments func(prID string) (any, error)
	log           Logger
	onPanic       func(any, []byte)

	authDisabled  atomic.Bool
	sessionTokens atomic.Int64
	wg            sync.WaitGroup
	// inflight coalesces concurrent KickPR calls per PR: overlapping syncs
	// and PR-open kicks would otherwise reclassify the same rows and
	// double-bill the user's key.
	inflightMu sync.Mutex
	inflight   map[string]bool
}

type commentForClassification struct {
	ID    string
	Input CommentInput
}

type cachedClassification struct {
	Severity string
	Provider string
	Model    string
}

// NewTier constructs the optional classifier with no-op notification defaults.
func NewTier(config TierConfig) *Tier {
	mode := config.Mode
	if mode == nil {
		mode = func() string { return "rules" }
	}
	settings := config.Settings
	if settings == nil {
		settings = func() SettingsSnapshot { return SettingsSnapshot{} }
	}
	emit := config.Emit
	if emit == nil {
		emit = func(string, map[string]any) {}
	}
	logger := config.Log
	if logger == nil {
		logger = log.Default()
	}

	return &Tier{
		db:            config.DB,
		client:        config.Client,
		mode:          mode,
		settings:      settings,
		emit:          emit,
		serveComments: config.ServeComments,
		log:           logger,
		onPanic:       config.OnPanic,
		inflight:      make(map[string]bool),
	}
}

// KickPR schedules one classification batch, coalescing concurrent calls per PR.
func (t *Tier) KickPR(ctx context.Context, prID string) {
	if t == nil || t.db == nil || t.client == nil || prID == "" {
		return
	}
	mode := t.mode()
	if mode != "rules+llm" && mode != "llm-all" {
		return
	}
	if t.authDisabled.Load() {
		return
	}

	// Coalesce: one live batch per PR. A kick arriving mid-batch is dropped —
	// the running batch already selects the current unclassified set, and the
	// next natural kick (sync or PR open) picks up anything new.
	t.inflightMu.Lock()
	if t.inflight[prID] {
		t.inflightMu.Unlock()
		return
	}
	t.inflight[prID] = true
	t.inflightMu.Unlock()

	prCtx, cancel := context.WithCancel(ctx)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer cancel()
		defer func() {
			t.inflightMu.Lock()
			delete(t.inflight, prID)
			t.inflightMu.Unlock()
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				t.reportPanic(recovered)
			}
		}()
		t.runBatch(prCtx, prID, mode)
	}()
}

// Wait blocks until all scheduled classification batches finish.
func (t *Tier) Wait() {
	if t == nil {
		return
	}
	t.wg.Wait()
}

// ResetAuth re-enables classification after credentials are updated.
func (t *Tier) ResetAuth() {
	if t == nil {
		return
	}
	t.authDisabled.Store(false)
}

func (t *Tier) runBatch(ctx context.Context, prID string, mode string) {
	comments, err := t.selectComments(ctx, prID, mode)
	if err != nil {
		t.log.Printf("select comments for llm classification pr=%s: %v", prID, err)
		return
	}
	if len(comments) == 0 {
		return
	}

	batchCtx, cancelBatch := context.WithCancel(ctx)
	defer cancelBatch()

	var batchTokens atomic.Int64
	var shouldAbandon atomic.Bool
	var workers sync.WaitGroup
	// A batch panic must still cancel and wait for its comment jobs.
	defer func() { cancelBatch(); workers.Wait() }()
	semaphore := make(chan struct{}, defaultBatchConcurrency)

	for _, comment := range comments {
		if shouldAbandon.Load() {
			break
		}
		select {
		case <-batchCtx.Done():
			shouldAbandon.Store(true)
		case semaphore <- struct{}{}:
		}
		if shouldAbandon.Load() {
			break
		}

		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			defer func() {
				if recovered := recover(); recovered != nil {
					shouldAbandon.Store(true)
					cancelBatch()
					t.reportPanic(recovered)
				}
			}()

			tokens, err := t.processComment(batchCtx, comment)
			if err == nil {
				batchTokens.Add(int64(tokens))
				return
			}
			if errors.Is(err, ErrAuth) {
				if t.authDisabled.CompareAndSwap(false, true) {
					t.emit("BYOK_AUTH_FAILED", map[string]any{})
				}
				cancelBatch()
				return
			}
			if errors.Is(err, ErrRateLimited) {
				shouldAbandon.Store(true)
				cancelBatch()
				return
			}
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.log.Printf("llm classification failed comment=%s: %v", comment.ID, err)
			}
		}()
	}

	workers.Wait()
	if shouldAbandon.Load() || (errors.Is(batchCtx.Err(), context.Canceled) && ctx.Err() != nil) || t.authDisabled.Load() {
		return
	}

	batchTokenCount := batchTokens.Load()
	if batchTokenCount > 0 {
		t.sessionTokens.Add(batchTokenCount)
	}
	t.emitCommentsReady(prID)
	t.emitUsage()
}

func (t *Tier) reportPanic(recovered any) {
	stack := debug.Stack()
	t.log.Printf("llm batch failed after recovered panic; stack=%s", stack)
	if t.onPanic != nil {
		func() {
			defer func() { _ = recover() }()
			t.onPanic(recovered, stack)
		}()
	}
}

func (t *Tier) selectComments(ctx context.Context, prID string, mode string) ([]commentForClassification, error) {
	// llm-all still excludes comments the LLM already judged (byok: source):
	// re-sending them buys nothing (the cache would answer anyway) and, worse,
	// re-emitting COMMENTS_READY for a no-op batch loops forever with the
	// renderer's refetch-on-push. Switching models affects new text only, by
	// spec. An empty selection returns early and emits nothing.
	const selectColumns = `SELECT id, body, COALESCE(author, ''), COALESCE(file_path, ''),
		COALESCE(line_number, 0), COALESCE(diff_hunk, '') FROM comments WHERE pr_id = ?`
	query := selectColumns + `
		AND (classification_source IS NULL OR classification_source NOT LIKE 'byok:%') ORDER BY id`
	if mode == "rules+llm" {
		query = selectColumns + ` AND classification_source = 'unclassified' ORDER BY id`
	}

	rows, err := t.db.QueryContext(ctx, query, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []commentForClassification
	for rows.Next() {
		var comment commentForClassification
		if err := rows.Scan(&comment.ID, &comment.Input.Body, &comment.Input.Author,
			&comment.Input.FilePath, &comment.Input.LineNumber, &comment.Input.DiffHunk); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return comments, nil
}

func (t *Tier) processComment(ctx context.Context, comment commentForClassification) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	bodyHash := classificationCacheKey(comment.Input)
	cached, hit, err := t.lookupCache(ctx, bodyHash)
	if err != nil {
		t.log.Printf("llm cache lookup failed comment=%s: %v", comment.ID, err)
	}
	if hit {
		return 0, t.updateComment(ctx, comment.ID, cached.Severity, classificationSource(cached.Provider, cached.Model))
	}

	settings := t.settings()
	severity, usageTokens, err := t.client.Classify(ctx, comment.Input)
	if err != nil {
		return 0, err
	}
	if !isValidSeverity(severity) {
		return 0, fmt.Errorf("invalid llm classification %q", severity)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	source := classificationSource(settings.Provider, settings.Model)
	if err := t.updateComment(ctx, comment.ID, severity, source); err != nil {
		return 0, err
	}
	if err := t.upsertCache(ctx, bodyHash, severity, settings.Provider, settings.Model); err != nil {
		t.log.Printf("llm cache upsert failed comment=%s: %v", comment.ID, err)
	}
	return usageTokens, nil
}

func (t *Tier) lookupCache(ctx context.Context, bodyHash string) (cachedClassification, bool, error) {
	var cached cachedClassification
	err := t.db.QueryRowContext(
		ctx,
		`SELECT severity, provider, model FROM llm_classification_cache WHERE body_hash = ?`,
		bodyHash,
	).Scan(&cached.Severity, &cached.Provider, &cached.Model)
	if errors.Is(err, sql.ErrNoRows) {
		return cachedClassification{}, false, nil
	}
	if err != nil {
		return cachedClassification{}, false, err
	}
	if !isValidSeverity(cached.Severity) {
		return cachedClassification{}, false, fmt.Errorf("invalid cached classification %q", cached.Severity)
	}
	return cached, true, nil
}

func (t *Tier) updateComment(ctx context.Context, commentID string, severity string, source string) error {
	_, err := t.db.ExecContext(
		ctx,
		`UPDATE comments SET severity = ?, classification_source = ? WHERE id = ?`,
		severity,
		source,
		commentID,
	)
	return err
}

func (t *Tier) upsertCache(ctx context.Context, bodyHash string, severity string, provider string, model string) error {
	_, err := t.db.ExecContext(
		ctx,
		`INSERT OR REPLACE INTO llm_classification_cache (body_hash, severity, provider, model, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		bodyHash,
		severity,
		provider,
		model,
	)
	return err
}

func (t *Tier) emitCommentsReady(prID string) {
	data := map[string]any{"pr_id": prID}
	if t.serveComments != nil {
		comments, err := t.serveComments(prID)
		if err != nil {
			t.log.Printf("serve comments after llm classification pr=%s: %v", prID, err)
		} else {
			data["comments"] = comments
		}
	}
	t.emit("COMMENTS_READY", data)
}

func (t *Tier) emitUsage() {
	sessionTokens := t.sessionTokens.Load()
	t.emit("BYOK_USAGE", map[string]any{
		"session_tokens": sessionTokens,
		"est_cost":       estimateCostUSD(sessionTokens, t.settings()),
	})
}

// classificationCacheKey hashes what the model actually judges: the
// vendor-noise-stripped body plus the diff hunk. Stripping first means a
// bot's per-comment hidden fingerprints (<!-- cr-comment:... -->) don't
// defeat the cache; including the hunk means identical bodies on different
// code are classified separately instead of reusing the first verdict.
func classificationCacheKey(input CommentInput) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(stripVendorNoise(input.Body))), " ")
	sum := sha256.Sum256([]byte(normalized + "\x00" + input.DiffHunk))
	return hex.EncodeToString(sum[:])
}

func classificationSource(provider string, model string) string {
	return "byok:" + provider + ":" + model
}

func estimateCostUSD(tokens int64, settings SettingsSnapshot) float64 {
	return float64(tokens) * estimatedCostPerTokenUSD(settings)
}

func estimatedCostPerTokenUSD(settings SettingsSnapshot) float64 {
	// Rough blended USD/token estimates for UI-only counters, not billing rates.
	const (
		anthropicUSDPerToken  = 0.000003
		openRouterUSDPerToken = 0.0000015
		defaultUSDPerToken    = 0.000002
	)
	costs := map[string]float64{
		"anthropic":  anthropicUSDPerToken,
		"openrouter": openRouterUSDPerToken,
		"ollama":     0,
	}
	if cost, ok := costs[strings.ToLower(settings.Provider)]; ok {
		return cost
	}
	return defaultUSDPerToken
}
