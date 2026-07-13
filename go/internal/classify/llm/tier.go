package llm

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
)

const defaultBatchConcurrency = int32(4)

type SettingsSnapshot struct {
	Provider string
	Model    string
}

type Logger interface {
	Printf(format string, v ...interface{})
}

type TierConfig struct {
	DB            *sql.DB
	Client        ClassifierClient
	Mode          func() string
	Settings      func() SettingsSnapshot
	Emit          func(eventType string, data map[string]interface{})
	ServeComments func(prID string) (interface{}, error)
	Log           Logger
}

type Tier struct {
	db            *sql.DB
	client        ClassifierClient
	mode          func() string
	settings      func() SettingsSnapshot
	emit          func(eventType string, data map[string]interface{})
	serveComments func(prID string) (interface{}, error)
	log           Logger

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
	ID   string
	Body string
}

type cachedClassification struct {
	Severity string
	Category string
	Provider string
	Model    string
}

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
		emit = func(string, map[string]interface{}) {}
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
		inflight:      make(map[string]bool),
	}
}

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
		t.runBatch(prCtx, prID, mode)
	}()
}

func (t *Tier) Wait() {
	if t == nil {
		return
	}
	t.wg.Wait()
}

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
	semaphore := make(chan struct{}, defaultBatchConcurrency)

	for _, comment := range comments {
		if shouldAbandon.Load() {
			break
		}
		select {
		case <-batchCtx.Done():
			shouldAbandon.Store(true)
			break
		case semaphore <- struct{}{}:
		}
		if shouldAbandon.Load() {
			break
		}

		comment := comment
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()

			tokens, err := t.processComment(batchCtx, comment)
			if err == nil {
				batchTokens.Add(int64(tokens))
				return
			}
			if errors.Is(err, ErrAuth) {
				if t.authDisabled.CompareAndSwap(false, true) {
					t.emit("BYOK_AUTH_FAILED", map[string]interface{}{})
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
	if shouldAbandon.Load() || errors.Is(batchCtx.Err(), context.Canceled) && ctx.Err() != nil || t.authDisabled.Load() {
		return
	}

	batchTokenCount := batchTokens.Load()
	if batchTokenCount > 0 {
		t.sessionTokens.Add(batchTokenCount)
	}
	t.emitCommentsReady(prID)
	t.emitUsage()
}

func (t *Tier) selectComments(ctx context.Context, prID string, mode string) ([]commentForClassification, error) {
	// llm-all still excludes comments the LLM already judged (byok: source):
	// re-sending them buys nothing (the cache would answer anyway) and, worse,
	// re-emitting COMMENTS_READY for a no-op batch loops forever with the
	// renderer's refetch-on-push. Switching models affects new text only, by
	// spec. An empty selection returns early and emits nothing.
	query := `SELECT id, body FROM comments WHERE pr_id = ?
		AND (classification_source IS NULL OR classification_source NOT LIKE 'byok:%') ORDER BY id`
	args := []interface{}{prID}
	if mode == "rules+llm" {
		query = `SELECT id, body FROM comments WHERE pr_id = ? AND classification_source = 'unclassified' ORDER BY id`
	}

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []commentForClassification
	for rows.Next() {
		var comment commentForClassification
		if err := rows.Scan(&comment.ID, &comment.Body); err != nil {
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

	bodyHash := normalizedBodyHash(comment.Body)
	cached, hit, err := t.lookupCache(ctx, bodyHash)
	if err != nil {
		t.log.Printf("llm cache lookup failed comment=%s: %v", comment.ID, err)
	}
	if hit {
		return 0, t.updateComment(ctx, comment.ID, cached.Severity, cached.Category, classificationSource(cached.Provider, cached.Model))
	}

	settings := t.settings()
	severity, category, usageTokens, err := t.client.Classify(ctx, comment.Body)
	if err != nil {
		return 0, err
	}
	if !isValidSeverity(severity) || !isValidCategory(category) {
		return 0, fmt.Errorf("invalid llm classification %q/%q", severity, category)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	source := classificationSource(settings.Provider, settings.Model)
	if err := t.updateComment(ctx, comment.ID, severity, category, source); err != nil {
		return 0, err
	}
	if err := t.upsertCache(ctx, bodyHash, severity, category, settings.Provider, settings.Model); err != nil {
		t.log.Printf("llm cache upsert failed comment=%s: %v", comment.ID, err)
	}
	return usageTokens, nil
}

func (t *Tier) lookupCache(ctx context.Context, bodyHash string) (cachedClassification, bool, error) {
	var cached cachedClassification
	err := t.db.QueryRowContext(
		ctx,
		`SELECT severity, category, provider, model FROM llm_classification_cache WHERE body_hash = ?`,
		bodyHash,
	).Scan(&cached.Severity, &cached.Category, &cached.Provider, &cached.Model)
	if errors.Is(err, sql.ErrNoRows) {
		return cachedClassification{}, false, nil
	}
	if err != nil {
		return cachedClassification{}, false, err
	}
	if !isValidSeverity(cached.Severity) || !isValidCategory(cached.Category) {
		return cachedClassification{}, false, fmt.Errorf("invalid cached classification %q/%q", cached.Severity, cached.Category)
	}
	return cached, true, nil
}

func (t *Tier) updateComment(ctx context.Context, commentID string, severity string, category string, source string) error {
	_, err := t.db.ExecContext(
		ctx,
		`UPDATE comments SET severity = ?, category = ?, classification_source = ? WHERE id = ?`,
		severity,
		category,
		source,
		commentID,
	)
	return err
}

func (t *Tier) upsertCache(ctx context.Context, bodyHash string, severity string, category string, provider string, model string) error {
	_, err := t.db.ExecContext(
		ctx,
		`INSERT OR REPLACE INTO llm_classification_cache (body_hash, severity, category, provider, model, created_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		bodyHash,
		severity,
		category,
		provider,
		model,
	)
	return err
}

func (t *Tier) emitCommentsReady(prID string) {
	data := map[string]interface{}{"pr_id": prID}
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
	t.emit("BYOK_USAGE", map[string]interface{}{
		"session_tokens": sessionTokens,
		"est_cost":       estimateCostUSD(sessionTokens, t.settings()),
	})
}

func normalizedBodyHash(body string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(body)), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func classificationSource(provider string, model string) string {
	return "byok:" + provider + ":" + model
}

func estimateCostUSD(tokens int64, settings SettingsSnapshot) float64 {
	return float64(tokens) * estimatedCostPerTokenUSD(settings)
}

func estimatedCostPerTokenUSD(settings SettingsSnapshot) float64 {
	// Rough blended estimates for UI-only session counters. Actual billing varies by model and provider.
	switch strings.ToLower(settings.Provider) {
	case "anthropic":
		return 0.000003
	case "openrouter":
		return 0.0000015
	case "ollama":
		return 0
	default:
		return 0.000002
	}
}
