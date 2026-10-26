package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
)

// ---- review tracking tests ----

// stubLoginGetter implements ReviewLoginGetter for tests.
type stubLoginGetter struct {
	login string
	found bool
}

func (s *stubLoginGetter) GetCurrentUser(_ context.Context, _ string) (string, bool, error) {
	return s.login, s.found, nil
}

// stubSHAWriter implements ReviewSHAWriter and captures calls for assertions.
type stubSHAWriter struct {
	calls []store.ReviewEvent
}

func (s *stubSHAWriter) RecordLastReviewed(_ context.Context, event store.ReviewEvent) error {
	s.calls = append(s.calls, event)
	return nil
}

func TestExecutor_ReviewTracking_ApproveRecordsLastReviewedSHA(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()

	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert a PR with a known head_sha.
	const prID = "github:o/r:1"
	_, err = db.ExecContext(ctx, `INSERT INTO prs
		(id, provider, owner, repo, number, title, author, state,
		 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-head')`,
		prID)
	if err != nil {
		t.Fatalf("insert pr: %v", err)
	}

	loginGetter := &stubLoginGetter{login: "alice", found: true}
	shaWriter := &stubSHAWriter{}

	emitter := events.NewEventEmitter()
	executor := NewActionExecutor(log, mockProv, db, emitter).
		WithReviewTracking(loginGetter, shaWriter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   prID,
		Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	if len(shaWriter.calls) != 1 {
		t.Fatalf("expected 1 RecordLastReviewed call, got %d", len(shaWriter.calls))
	}
	call := shaWriter.calls[0]
	if call.PRID != prID {
		t.Errorf("prID: got %q want %q", call.PRID, prID)
	}
	if call.Login != "alice" {
		t.Errorf("login: got %q want alice", call.Login)
	}
	if call.HeadSHA != "sha-head" {
		t.Errorf("sha: got %q want sha-head", call.HeadSHA)
	}
	// patchsetNum may be 0 here (patchset row not yet recorded in test DB) — that is correct.
}

func TestExecutor_ReviewTracking_NoUserSkips(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()
	emitter := events.NewEventEmitter()

	// No current user → should not record a review baseline.
	loginGetter := &stubLoginGetter{found: false}
	shaWriter := &stubSHAWriter{}

	executor := NewActionExecutor(log, mockProv, nil, emitter).
		WithReviewTracking(loginGetter, shaWriter)

	_, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:o/r:1", Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(shaWriter.calls) != 0 {
		t.Errorf("expected 0 SHA writes when no user, got %d", len(shaWriter.calls))
	}
}

func TestExecutor_ReviewTracking_NilTrackerNoOp(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()
	emitter := events.NewEventEmitter()

	// No review tracking attached → must not panic.
	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:o/r:1", Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success without review tracking, got: %s", result.Error)
	}
}

func TestExecutor_ReviewTracking_RecordsPatchsetNumber(t *testing.T) {
	// The newest patchset at the current head wins even after the base tip moves.
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()

	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const prID = "github:o/r:99"
	_, err = db.ExecContext(ctx, `INSERT INTO prs
		(id, provider, owner, repo, number, title, author, state,
		 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github','o','r',99,'T','a','open','2026-01-01','2026-01-01','main','f','BASE-SHA','HEAD-SHA')`,
		prID)
	if err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	// A moving base tip must not hide the latest patchset at the same head.
	_, err = db.ExecContext(ctx, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES (?, 1, 'HEAD-SHA', 'BASE-SHA', CURRENT_TIMESTAMP),
		       (?, 2, 'HEAD-SHA', 'OLD-BASE-SHA', CURRENT_TIMESTAMP)`, prID, prID)
	if err != nil {
		t.Fatalf("insert patchset: %v", err)
	}

	loginGetter := &stubLoginGetter{login: "alice", found: true}
	shaWriter := &stubSHAWriter{}
	emitter := events.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, db, emitter).
		WithReviewTracking(loginGetter, shaWriter)

	_, err = executor.Execute(ctx, &ActionRequest{PRID: prID, Action: ApproveCommand})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(shaWriter.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(shaWriter.calls))
	}
	call := shaWriter.calls[0]
	if call.PatchsetNum != 2 {
		t.Errorf("patchsetNum: got %d want 2 (must record exact patchset number)", call.PatchsetNum)
	}
	if call.HeadSHA != "HEAD-SHA" {
		t.Errorf("sha: got %q want HEAD-SHA", call.HeadSHA)
	}
	if call.Source != store.ReviewSourceBistroAction {
		t.Errorf("source: got %q want bistro_action", call.Source)
	}
	if call.ReviewedAt.IsZero() {
		t.Error("reviewedAt must record the action time")
	}
}
