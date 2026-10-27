// go/internal/actions/handlers_test.go
package actions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
)

func TestApproveHandler(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &ApproveHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "Approved!",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Action != ApproveCommand {
		t.Errorf("expected action=%s, got %s", ApproveCommand, result.Action)
	}
}

func TestRequestChangesHandler(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &RequestChangesHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "Please update this section",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Action != RequestChangesCommand {
		t.Errorf("expected action=%s, got %s", RequestChangesCommand, result.Action)
	}
}

func TestPostCommentHandler(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &PostCommentHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "This needs attention",
		Line: 15,
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Action != PostCommentCommand {
		t.Errorf("expected action=%s, got %s", PostCommentCommand, result.Action)
	}
}

func TestHandlerErrorPropagation(t *testing.T) {
	mockProv := mock.NewMockProvider().WithError(errors.New("provider unavailable"))

	handler := &ApproveHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v (should return error in result)", err)
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestApproveHandlerWithoutBody(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &ApproveHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true for approve without body")
	}
}

func TestRequestChangesHandlerMissingBody(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &RequestChangesHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v (should return error in result)", err)
	}
	if result.Success {
		t.Error("expected failure for missing body")
	}
}

func TestPostCommentHandlerWithLineNumber(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &PostCommentHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "Line comment",
		Line: 42,
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
}

func TestPostCommentHandlerMissingBody(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &PostCommentHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Line: 5,
	})

	if err != nil {
		t.Fatalf("Handle() error = %v (should return error in result)", err)
	}
	if result.Success {
		t.Error("expected failure for missing body")
	}
}

func TestReplyCommentHandler_Success(t *testing.T) {
	prov := mock.NewMockProvider()
	h := NewReplyCommentHandler(prov)
	result, err := h.Handle(context.Background(), &ActionRequest{
		PRID:      "github:owner/repo:42",
		Action:    ReplyCommentCommand,
		Body:      "Reply body",
		InReplyTo: "github-review-comment-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}
	if len(prov.ReplyCalls) != 1 {
		t.Fatalf("expected one reply call, got %d", len(prov.ReplyCalls))
	}
	call := prov.ReplyCalls[0]
	if call.PRID != "github:owner/repo:42" || call.InReplyTo != "github-review-comment-123" || call.Body != "Reply body" {
		t.Errorf("unexpected reply call: %+v", call)
	}
}

func TestReplyCommentHandlerMissingTarget(t *testing.T) {
	h := NewReplyCommentHandler(mock.NewMockProvider())
	result, err := h.Handle(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ReplyCommentCommand,
		Body:   "Reply body",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v (should return error in result)", err)
	}
	if result.Success {
		t.Error("expected failure for missing InReplyTo")
	}
}

func TestResolveThreadHandler_Success(t *testing.T) {
	prov := mock.NewMockProvider()
	h := NewResolveThreadHandler(prov)
	result, err := h.Handle(context.Background(), &ActionRequest{
		PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}
	if len(prov.ResolvedCalls) != 1 || prov.ResolvedCalls[0] != "PRRT_abc123" {
		t.Errorf("ResolveThread not called correctly: %v", prov.ResolvedCalls)
	}
}

func TestUnresolveThreadHandler_Success(t *testing.T) {
	prov := mock.NewMockProvider()
	h := NewUnresolveThreadHandler(prov)
	result, err := h.Handle(context.Background(), &ActionRequest{
		PRID: "pr1", Action: UnresolveThreadCommand, Body: "PRRT_xyz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}
	if len(prov.UnresolvedCalls) != 1 || prov.UnresolvedCalls[0] != "PRRT_xyz" {
		t.Errorf("UnresolveThread not called correctly: %v", prov.UnresolvedCalls)
	}
}

func TestResolveThreadHandler_NoCapability_PermanentError(t *testing.T) {
	type onlyProvider struct{ provider.Provider }
	prov := mock.NewMockProvider()
	h := NewResolveThreadHandler(onlyProvider{prov})
	result, err := h.Handle(context.Background(), &ActionRequest{
		PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure: provider lacks ThreadResolver")
	}
	if result.IsRecoverable() {
		t.Error("no-capability must be permanent (not recoverable)")
	}
}

func TestResolveThreadHandler_NotFound_PermanentError(t *testing.T) {
	prov := mock.NewMockProvider()
	prov.SetResolveError(errors.New("could not resolve to a node: 404 not found"))
	h := NewResolveThreadHandler(prov)
	result, err := h.Handle(context.Background(), &ActionRequest{
		PRID: "pr1", Action: ResolveThreadCommand, Body: "MISSING",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure")
	}
	if result.IsRecoverable() {
		t.Error("not-found must not be recoverable")
	}
}

type reviewSubmitterProvider struct {
	*mock.MockProvider
	submissions []provider.ReviewSubmission
	submitError error
}

func (p *reviewSubmitterProvider) SubmitReview(_ context.Context, _ string, sub provider.ReviewSubmission) error {
	p.submissions = append(p.submissions, sub)
	return p.submitError
}

func TestExecute_SubmitReviewSendsTheReviewSubmission(t *testing.T) {
	prov := &reviewSubmitterProvider{MockProvider: mock.NewMockProvider()}
	comments := []provider.ReviewComment{{Path: "a.go", Side: "LEFT", StartLine: 2, Line: 4, Body: "range"}}
	result, err := NewActionExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)), prov, nil, nil).Execute(context.Background(), &ActionRequest{
		PRID: "github:o/r:1", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove,
		Body: "LGTM", CommitID: "abc", ReviewComments: comments,
	})
	if err != nil || !result.Success {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.Action != SubmitReviewCommand || result.PRID != "github:o/r:1" || result.Timestamp.IsZero() {
		t.Fatalf("result = %+v", result)
	}
	if len(prov.submissions) != 1 {
		t.Fatalf("submissions = %d, want 1", len(prov.submissions))
	}
	got := prov.submissions[0]
	if got.Event != ReviewEventApprove || got.Body != "LGTM" || got.CommitID != "abc" || len(got.Comments) != 1 || got.Comments[0] != comments[0] {
		t.Fatalf("submission = %+v", got)
	}
}

func TestExecute_SubmitReviewWithoutCapabilityFailsPermanently(t *testing.T) {
	result, err := NewActionExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)), mock.NewMockProvider(), nil, nil).Execute(context.Background(), &ActionRequest{
		PRID: "github:o/r:1", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove,
	})
	if err != nil || result.Success || result.IsRecoverable() || !strings.Contains(result.Error, "permanent") {
		t.Fatalf("result = %+v, err = %v, want a permanent failure", result, err)
	}
}

func TestExecute_SubmitReviewLocalSideEffects(t *testing.T) {
	cases := []struct {
		name           string
		event          string
		wantStatus     string
		wantEventTypes []string
	}{
		{"approve", ReviewEventApprove, "approved", []string{"action_complete", "pr_approved"}},
		{"request changes", ReviewEventRequestChanges, "changes_requested", []string{"action_complete", "changes_requested"}},
		{"comment", ReviewEventComment, "pending", []string{"action_complete"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.OpenDB(":memory:")
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			defer db.Close()
			if err := store.NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			const prID = "github:o/r:1"
			if _, err := db.ExecContext(ctx, `INSERT INTO prs
				(id, provider, owner, repo, number, title, author, state,
				 created_at, updated_at, base_branch, head_branch, base_sha, head_sha, review_status)
				VALUES (?, 'github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-head','pending')`, prID); err != nil {
				t.Fatalf("insert pr: %v", err)
			}
			prov := &reviewSubmitterProvider{MockProvider: mock.NewMockProvider()}
			emitter := events.NewEventEmitter()
			subscriber := emitter.Subscribe()
			defer emitter.Unsubscribe(subscriber)
			shaWriter := &stubSHAWriter{}
			executor := NewActionExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)), prov, db, emitter).
				WithReviewTracking(&stubLoginGetter{login: "alice", found: true}, shaWriter)
			result, err := executor.Execute(ctx, &ActionRequest{
				PRID: prID, Action: SubmitReviewCommand, ReviewEvent: tc.event, Body: "Review body", CommitID: "sha-head",
			})
			if err != nil || !result.Success {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			var status string
			if err := db.QueryRowContext(ctx, `SELECT review_status FROM prs WHERE id = ?`, prID).Scan(&status); err != nil {
				t.Fatalf("read review status: %v", err)
			}
			if status != tc.wantStatus {
				t.Errorf("review_status = %q, want %q", status, tc.wantStatus)
			}
			if len(shaWriter.calls) != 1 || (shaWriter.calls[0].PRID != prID || shaWriter.calls[0].Login != "alice" || shaWriter.calls[0].HeadSHA != "sha-head") {
				t.Errorf("last-reviewed writes = %+v", shaWriter.calls)
			}
			var eventTypes []string
			for len(subscriber) > 0 {
				event := <-subscriber
				eventTypes = append(eventTypes, event.Type)
				if event.Type == "action_complete" && (event.Data["action"] != SubmitReviewCommand || event.Data["success"] != true) {
					t.Errorf("action_complete = %+v", event.Data)
				}
			}
			if !slices.Equal(eventTypes, tc.wantEventTypes) {
				t.Errorf("event types = %v, want %v", eventTypes, tc.wantEventTypes)
			}
		})
	}
}

func TestExecute_SubmitReviewOlderCommitDoesNotAdvanceLastReviewedSHA(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const prID = "github:o/r:1"
	const cachedHeadSHA = "H2"
	const submittedCommitSHA = "H1"
	if _, err := db.ExecContext(ctx, `INSERT INTO prs
		(id, provider, owner, repo, number, title, author, state,
		 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b',?)`, prID, cachedHeadSHA); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	prov := &reviewSubmitterProvider{MockProvider: mock.NewMockProvider()}
	shaWriter := &stubSHAWriter{}
	executor := NewActionExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)), prov, db, nil).
		WithReviewTracking(&stubLoginGetter{login: "alice", found: true}, shaWriter)
	result, err := executor.Execute(ctx, &ActionRequest{
		PRID: prID, Action: SubmitReviewCommand, ReviewEvent: ReviewEventComment, CommitID: submittedCommitSHA,
		ReviewComments: []provider.ReviewComment{{Path: "a.go", Side: "RIGHT", Line: 1, Body: "Older draft"}},
	})
	if err != nil || !result.Success {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if len(shaWriter.calls) != 0 {
		t.Fatalf("last-reviewed writes = %+v, want none for a review on %s with cached head %s", shaWriter.calls, submittedCommitSHA, cachedHeadSHA)
	}
}

func TestExecute_SubmitReviewFailuresAndAmbiguousTransport(t *testing.T) {
	cases := []struct {
		name          string
		providerError string
		wantAmbiguous bool
		wantRateLimit bool
	}{
		{"permanent rejection", "422 validation failed", false, false},
		{"transport may have succeeded", "connection reset", true, false},
		{"rate limited", "429 rate limit", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := &reviewSubmitterProvider{MockProvider: mock.NewMockProvider(), submitError: errors.New(tc.providerError)}
			result, err := NewActionExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)), prov, nil, nil).Execute(context.Background(), &ActionRequest{
				PRID: "github:o/r:1", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove,
			})
			if err != nil || result.Success || result.IsAmbiguousRemoteSuccess() != tc.wantAmbiguous || result.IsRateLimit() != tc.wantRateLimit {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if !tc.wantAmbiguous && !strings.Contains(result.Error, "failed to submit review: "+tc.providerError) {
				t.Errorf("error = %q, want provider error with context", result.Error)
			}
			if len(prov.submissions) != 1 {
				t.Errorf("submissions = %d, want 1", len(prov.submissions))
			}
		})
	}
}

func TestIsPermanentThreadError(t *testing.T) {
	cases := []struct {
		msg  string
		perm bool
	}{
		{"404 not found", true},
		{"could not resolve to a node", true},
		{"not resolve", true},
		{"500 internal server error", false},
		{"timeout", false},
		{"", false},
	}
	for _, tc := range cases {
		var err error
		if tc.msg != "" {
			err = errors.New(tc.msg)
		}
		if got := isPermanentThreadError(err); got != tc.perm {
			t.Errorf("isPermanentThreadError(%q) = %v, want %v", tc.msg, got, tc.perm)
		}
	}
}
