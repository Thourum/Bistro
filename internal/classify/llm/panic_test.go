package llm

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
)

type panicTestLogger struct{ messages []string }

func (logger *panicTestLogger) Printf(format string, _ ...any) {
	logger.messages = append(logger.messages, format)
}

func TestTierJobsRecoverAndReportPanics(t *testing.T) {
	for _, phase := range []string{"comment", "batch"} {
		t.Run(phase, func(t *testing.T) {
			db := setupTierTestDB(t)
			insertComment(t, db, "c1", "pr-1", "body", "info", "unclassified")
			var reports atomic.Int32
			var shouldPanic atomic.Bool
			shouldPanic.Store(true)
			logger := &panicTestLogger{}
			ready := 0
			tier := NewTier(TierConfig{
				DB: db, Mode: func() string { return "rules+llm" },
				Client: mockClient{classify: func(context.Context, CommentInput) (string, int, error) {
					if phase == "comment" && shouldPanic.Load() {
						panic("job panic")
					}
					return "warning", 1, nil
				}},
				// This runs in the batch before COMMENTS_READY is emitted.
				ServeComments: func(string) (any, error) {
					if phase == "batch" && shouldPanic.Load() {
						panic("job panic")
					}
					return nil, nil
				},
				Emit: func(name string, _ map[string]any) {
					if name == "COMMENTS_READY" {
						ready++
					}
				},
				Log: logger,
				OnPanic: func(recovered any, stack []byte) {
					reports.Add(1)
					if recovered != "job panic" || len(stack) == 0 {
						t.Errorf("panic report = %v", recovered)
					}
				},
			})
			tier.KickPR(context.Background(), "pr-1")
			tier.Wait()
			if reports.Load() != 1 || ready != 0 || len(logger.messages) != 1 || !strings.Contains(logger.messages[0], "batch failed") {
				t.Fatalf("reports=%d ready=%d failures=%v", reports.Load(), ready, logger.messages)
			}
			shouldPanic.Store(false)
			if _, err := db.Exec(`UPDATE comments SET classification_source = 'unclassified' WHERE id = 'c1'`); err != nil {
				t.Fatal(err)
			}
			tier.KickPR(context.Background(), "pr-1")
			tier.Wait()
			severity, source := readCommentClassification(t, db, "c1")
			if ready != 1 || severity != "warning" || !strings.HasPrefix(source, "byok:") || reports.Load() != 1 {
				t.Fatalf("retry ready=%d classification=%s/%s reports=%d", ready, severity, source, reports.Load())
			}
		})
	}
}
