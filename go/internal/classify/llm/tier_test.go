package llm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type mockClient struct {
	classify func(ctx context.Context, comment CommentInput) (string, int, error)
}

func (m mockClient) Classify(ctx context.Context, comment CommentInput) (string, int, error) {
	return m.classify(ctx, comment)
}

type capturedEvent struct {
	eventType string
	data      map[string]interface{}
}

func TestTierModeSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mode      string
		wantCalls int32
	}{
		{name: "rules no-op", mode: "rules", wantCalls: 0},
		{name: "server no-op", mode: "server", wantCalls: 0},
		{name: "rules+llm only unclassified", mode: "rules+llm", wantCalls: 1},
		{name: "llm-all all comments for PR", mode: "llm-all", wantCalls: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := setupTierTestDB(t)
			insertComment(t, db, "c1", "pr-1", "unclassified one", "info", "unclassified")
			insertComment(t, db, "c2", "pr-1", "already classified", "nit", "rule:coderabbit.yml:nit")
			insertComment(t, db, "c3", "pr-2", "other pr", "info", "unclassified")

			var calls atomic.Int32
			tier := NewTier(TierConfig{
				DB:       db,
				Client:   mockClient{classify: successfulClassify(&calls)},
				Mode:     func() string { return tt.mode },
				Settings: staticSettings("openai", "gpt-test"),
				Emit:     noopEmit,
			})

			tier.KickPR(context.Background(), "pr-1")
			tier.Wait()

			if calls.Load() != tt.wantCalls {
				t.Fatalf("client calls = %d, want %d", calls.Load(), tt.wantCalls)
			}
		})
	}
}

func TestTierCacheHitSkipsClientAndUsesCacheProvenance(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	body := "  Please Rename This   Variable  "
	insertComment(t, db, "c1", "pr-1", body, "info", "unclassified")
	insertCache(t, db, classificationCacheKey(CommentInput{Body: body}), "nit", "anthropic", "claude-cache")

	var calls atomic.Int32
	tier := NewTier(TierConfig{
		DB:       db,
		Client:   mockClient{classify: successfulClassify(&calls)},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-current"),
		Emit:     noopEmit,
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()

	if calls.Load() != 0 {
		t.Fatalf("client calls = %d, want 0", calls.Load())
	}
	severity, source := readCommentClassification(t, db, "c1")
	if severity != "nit" || source != "byok:anthropic:claude-cache" {
		t.Fatalf("classification = %q/%q, want nit/byok:anthropic:claude-cache", severity, source)
	}
}

func TestTierCacheMissCallsClientUpdatesCommentAndInsertsCache(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	body := "This looks like SQL injection"
	insertComment(t, db, "c1", "pr-1", body, "info", "unclassified")

	var calls atomic.Int32
	tier := NewTier(TierConfig{
		DB:     db,
		Client: mockClient{classify: successfulClassify(&calls)},
		Mode:   func() string { return "rules+llm" },
		Settings: func() SettingsSnapshot {
			return SettingsSnapshot{Provider: "openrouter", Model: "deepseek-test"}
		},
		Emit: noopEmit,
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()

	if calls.Load() != 1 {
		t.Fatalf("client calls = %d, want 1", calls.Load())
	}
	severity, source := readCommentClassification(t, db, "c1")
	if severity != "warning" || source != "byok:openrouter:deepseek-test" {
		t.Fatalf("classification = %q/%q, want warning/byok:openrouter:deepseek-test", severity, source)
	}
	cacheSeverity, cacheProvider, cacheModel := readCache(t, db, classificationCacheKey(CommentInput{Body: body}))
	if cacheSeverity != "warning" || cacheProvider != "openrouter" || cacheModel != "deepseek-test" {
		t.Fatalf("cache = %q/%q/%q, want warning/openrouter/deepseek-test", cacheSeverity, cacheProvider, cacheModel)
	}
}

func TestTierFanoutIsBounded(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	for i := 0; i < 10; i++ {
		insertComment(t, db, fmt.Sprintf("c%d", i), "pr-1", fmt.Sprintf("body %d", i), "info", "unclassified")
	}

	var active atomic.Int32
	var maxActive atomic.Int32
	var calls atomic.Int32
	tier := NewTier(TierConfig{
		DB: db,
		Client: mockClient{classify: func(ctx context.Context, comment CommentInput) (string, int, error) {
			current := active.Add(1)
			for {
				maxSeen := maxActive.Load()
				if current <= maxSeen || maxActive.CompareAndSwap(maxSeen, current) {
					break
				}
			}
			defer active.Add(-1)
			calls.Add(1)
			select {
			case <-ctx.Done():
				return "", 0, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				return "nit", 1, nil
			}
		}},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-test"),
		Emit:     noopEmit,
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()

	if calls.Load() != 10 {
		t.Fatalf("client calls = %d, want 10", calls.Load())
	}
	if maxActive.Load() > defaultBatchConcurrency {
		t.Fatalf("max concurrent calls = %d, want <= %d", maxActive.Load(), defaultBatchConcurrency)
	}
}

func TestTierErrAuthDisablesAndEmitsOnceUntilReset(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	insertComment(t, db, "c1", "pr-1", "body 1", "info", "unclassified")
	insertComment(t, db, "c2", "pr-2", "body 2", "info", "unclassified")
	insertComment(t, db, "c3", "pr-3", "body 3", "info", "unclassified")

	var calls atomic.Int32
	events, emit := captureEvents()
	tier := NewTier(TierConfig{
		DB: db,
		Client: mockClient{classify: func(ctx context.Context, comment CommentInput) (string, int, error) {
			calls.Add(1)
			return "", 0, ErrAuth
		}},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-test"),
		Emit:     emit,
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()
	tier.KickPR(context.Background(), "pr-2")
	tier.Wait()

	if calls.Load() != 1 {
		t.Fatalf("client calls before reset = %d, want 1", calls.Load())
	}
	if countEvents(events(), "BYOK_AUTH_FAILED") != 1 {
		t.Fatalf("BYOK_AUTH_FAILED events = %d, want 1", countEvents(events(), "BYOK_AUTH_FAILED"))
	}

	tier.ResetAuth()
	tier.KickPR(context.Background(), "pr-3")
	tier.Wait()

	if calls.Load() != 2 {
		t.Fatalf("client calls after reset = %d, want 2", calls.Load())
	}
	if countEvents(events(), "BYOK_AUTH_FAILED") != 2 {
		t.Fatalf("BYOK_AUTH_FAILED events after reset = %d, want 2", countEvents(events(), "BYOK_AUTH_FAILED"))
	}
}

func TestTierErrRateLimitedAbandonsSilently(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	insertComment(t, db, "c1", "pr-1", "body", "info", "unclassified")
	events, emit := captureEvents()
	tier := NewTier(TierConfig{
		DB: db,
		Client: mockClient{classify: func(ctx context.Context, comment CommentInput) (string, int, error) {
			return "", 0, ErrRateLimited
		}},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-test"),
		Emit:     emit,
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()

	if len(events()) != 0 {
		t.Fatalf("events = %#v, want none", events())
	}
	severity, source := readCommentClassification(t, db, "c1")
	if severity != "info" || source != "unclassified" {
		t.Fatalf("classification changed to %q/%q after rate limit", severity, source)
	}
}

func TestTierSingleCommentFailureDoesNotBlockOthers(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	insertComment(t, db, "c1", "pr-1", "fail me", "info", "unclassified")
	insertComment(t, db, "c2", "pr-1", "classify me", "info", "unclassified")

	tier := NewTier(TierConfig{
		DB: db,
		Client: mockClient{classify: func(ctx context.Context, comment CommentInput) (string, int, error) {
			if comment.Body == "fail me" {
				return "", 0, errors.New("temporary failure")
			}
			return "suggestion", 9, nil
		}},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-test"),
		Emit:     noopEmit,
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()

	severity, source := readCommentClassification(t, db, "c1")
	if severity != "info" || source != "unclassified" {
		t.Fatalf("failed comment classification = %q/%q, want deterministic result unchanged", severity, source)
	}
	severity, source = readCommentClassification(t, db, "c2")
	if severity != "suggestion" || source != "byok:openai:gpt-test" {
		t.Fatalf("successful comment classification = %q/%q", severity, source)
	}
}

func TestTierCancellationDrainsCleanly(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	for i := 0; i < 8; i++ {
		insertComment(t, db, fmt.Sprintf("c%d", i), "pr-1", fmt.Sprintf("body %d", i), "info", "unclassified")
	}

	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	tier := NewTier(TierConfig{
		DB: db,
		Client: mockClient{classify: func(ctx context.Context, comment CommentInput) (string, int, error) {
			once.Do(func() { close(started) })
			<-ctx.Done()
			return "", 0, ctx.Err()
		}},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-test"),
		Emit:     noopEmit,
	})

	tier.KickPR(ctx, "pr-1")
	<-started
	cancel()
	done := make(chan struct{})
	go func() {
		tier.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tier did not drain after cancellation")
	}
}

func TestTierEmitsCommentsReadyAndUsageAfterBatch(t *testing.T) {
	t.Parallel()

	db := setupTierTestDB(t)
	insertComment(t, db, "c1", "pr-1", "body", "info", "unclassified")
	events, emit := captureEvents()
	tier := NewTier(TierConfig{
		DB: db,
		Client: mockClient{classify: func(ctx context.Context, comment CommentInput) (string, int, error) {
			return "nit", 123, nil
		}},
		Mode:     func() string { return "rules+llm" },
		Settings: staticSettings("openai", "gpt-test"),
		Emit:     emit,
		ServeComments: func(prID string) (interface{}, error) {
			if prID != "pr-1" {
				t.Fatalf("ServeComments prID = %q, want pr-1", prID)
			}
			return []map[string]string{{"id": "c1"}}, nil
		},
	})

	tier.KickPR(context.Background(), "pr-1")
	tier.Wait()

	gotEvents := events()
	if countEvents(gotEvents, "COMMENTS_READY") != 1 {
		t.Fatalf("COMMENTS_READY events = %d, want 1 (%#v)", countEvents(gotEvents, "COMMENTS_READY"), gotEvents)
	}
	if countEvents(gotEvents, "BYOK_USAGE") != 1 {
		t.Fatalf("BYOK_USAGE events = %d, want 1 (%#v)", countEvents(gotEvents, "BYOK_USAGE"), gotEvents)
	}
	ready := findEvent(gotEvents, "COMMENTS_READY")
	if ready.data["pr_id"] != "pr-1" {
		t.Fatalf("COMMENTS_READY pr_id = %#v, want pr-1", ready.data["pr_id"])
	}
	if ready.data["comments"] == nil {
		t.Fatal("COMMENTS_READY missing comments")
	}
	usage := findEvent(gotEvents, "BYOK_USAGE")
	if usage.data["session_tokens"] != int64(123) {
		t.Fatalf("session_tokens = %#v, want int64(123)", usage.data["session_tokens"])
	}
	if _, ok := usage.data["est_cost"].(float64); !ok {
		t.Fatalf("est_cost = %#v, want float64", usage.data["est_cost"])
	}
}

func setupTierTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	statements := []string{
		`CREATE TABLE comments (
			id TEXT PRIMARY KEY,
			pr_id TEXT NOT NULL,
			author TEXT,
			body TEXT NOT NULL,
			line_number INTEGER,
			file_path TEXT,
			diff_hunk TEXT,
			severity TEXT,
			classification_source TEXT
		)`,
		`CREATE TABLE llm_classification_cache (
			body_hash TEXT PRIMARY KEY,
			severity TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	return db
}

func insertComment(t *testing.T, db *sql.DB, id string, prID string, body string, severity string, source string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO comments (id, pr_id, body, severity, classification_source) VALUES (?, ?, ?, ?, ?)`, id, prID, body, severity, source)
	if err != nil {
		t.Fatalf("insert comment: %v", err)
	}
}

func insertCache(t *testing.T, db *sql.DB, hash string, severity string, provider string, model string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO llm_classification_cache (body_hash, severity, provider, model) VALUES (?, ?, ?, ?)`, hash, severity, provider, model)
	if err != nil {
		t.Fatalf("insert cache: %v", err)
	}
}

func readCommentClassification(t *testing.T, db *sql.DB, id string) (string, string) {
	t.Helper()
	var severity string
	var source string
	if err := db.QueryRow(`SELECT severity, classification_source FROM comments WHERE id = ?`, id).Scan(&severity, &source); err != nil {
		t.Fatalf("read comment classification: %v", err)
	}
	return severity, source
}

func readCache(t *testing.T, db *sql.DB, hash string) (string, string, string) {
	t.Helper()
	var severity string
	var provider string
	var model string
	if err := db.QueryRow(`SELECT severity, provider, model FROM llm_classification_cache WHERE body_hash = ?`, hash).Scan(&severity, &provider, &model); err != nil {
		t.Fatalf("read cache: %v", err)
	}
	return severity, provider, model
}

func successfulClassify(calls *atomic.Int32) func(context.Context, CommentInput) (string, int, error) {
	return func(ctx context.Context, comment CommentInput) (string, int, error) {
		calls.Add(1)
		return "warning", 11, nil
	}
}

func staticSettings(provider string, model string) func() SettingsSnapshot {
	return func() SettingsSnapshot {
		return SettingsSnapshot{Provider: provider, Model: model}
	}
}

func noopEmit(eventType string, data map[string]interface{}) {}

func captureEvents() (func() []capturedEvent, func(string, map[string]interface{})) {
	var mu sync.Mutex
	var events []capturedEvent
	return func() []capturedEvent {
			mu.Lock()
			defer mu.Unlock()
			return append([]capturedEvent(nil), events...)
		}, func(eventType string, data map[string]interface{}) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, capturedEvent{eventType: eventType, data: data})
		}
}

func countEvents(events []capturedEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.eventType == eventType {
			count++
		}
	}
	return count
}

func findEvent(events []capturedEvent, eventType string) capturedEvent {
	for _, event := range events {
		if event.eventType == eventType {
			return event
		}
	}
	return capturedEvent{}
}
