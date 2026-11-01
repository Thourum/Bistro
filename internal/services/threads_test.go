package services

import (
	"context"
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/store"
)

type threadTestLister struct {
	threads []store.ReviewThreadRow
	err     error
}

func (s threadTestLister) ListReviewThreads(context.Context, string) ([]store.ReviewThreadRow, error) {
	return s.threads, s.err
}

type threadTestApplier struct {
	captured *actions.ActionRequest
	fail     bool
	queued   bool
}

func (s *threadTestApplier) ApplyAction(_ context.Context, req *actions.ActionRequest) *actions.ActionResult {
	s.captured = req
	return &actions.ActionResult{PRID: req.PRID, Action: req.Action, Success: !s.fail, Queued: s.queued, Error: "provider failed"}
}
func TestThreadsList(t *testing.T) {
	s := &Threads{DB: threadTestLister{threads: []store.ReviewThreadRow{{ID: "rt-1", StateKnown: true}, {ID: "rt-2", Resolved: true}}}}
	result, err := s.List(context.Background(), PRRef{PRID: "github:OWNER/REPO:1"})
	if err != nil || result.PRID != "github:owner/repo:1" || len(result.Threads) != 2 || !result.Threads[0].StateKnown || !result.Threads[1].Resolved {
		t.Fatalf("result = %+v, %v", result, err)
	}
	s.DB = threadTestLister{}
	result, err = s.List(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil || result.Threads == nil || len(result.Threads) != 0 {
		t.Fatalf("empty = %+v, %v", result, err)
	}
	s.DB = threadTestLister{err: errors.New("db failed")}
	_, err = s.List(context.Background(), PRRef{PRID: "pr-1"})
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInternal {
		t.Fatalf("db error = %v", err)
	}
}
func TestSubmitActionAliasesAndCommands(t *testing.T) {
	applier := &threadTestApplier{}
	s := &Threads{GetApplier: func() ActionApplier { return applier }}
	for _, req := range []ActionRequest{
		{Action: "approve"}, {Action: "request_changes", Body: "changes"}, {Action: "submit_review", ReviewEvent: "COMMENT"},
		{Action: "post_comment", Body: "comment"}, {Action: "reply_comment", Body: "reply", CommentID: "comment-id"},
		{Action: "reply_comment", Body: "reply", ThreadID: "thread-id"},
		{Action: "resolve_thread", Body: "old-thread", ThreadID: "thread-id"}, {Action: "unresolve_thread", ThreadID: "thread-id"},
	} {
		req.PRID = "github:OWNER/REPO:1"
		result, err := s.SubmitAction(context.Background(), req)
		if err != nil || result.Action != req.Action || result.PRID != "github:owner/repo:1" {
			t.Fatalf("action = %+v, %v", result, err)
		}
		captured := applier.captured
		if req.Action == "reply_comment" && (captured.Body != "reply" || captured.InReplyTo != req.CommentID || captured.ThreadID != req.ThreadID) {
			t.Fatalf("reply = %+v", captured)
		}
		if (req.Action == "resolve_thread" || req.Action == "unresolve_thread") && captured.Body != "thread-id" {
			t.Fatalf("thread alias = %+v", captured)
		}
	}
}
func TestSubmitActionFailuresAndLiveGetter(t *testing.T) {
	var current ActionApplier
	s := &Threads{GetApplier: func() ActionApplier { return current }}
	req := ActionRequest{PRID: "pr-1", Action: "resolve_thread", ThreadID: "thread-id"}
	_, err := s.SubmitAction(context.Background(), req)
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeUnauthenticated {
		t.Fatalf("signed out = %v", err)
	}
	applier := &threadTestApplier{}
	current = applier
	if _, err := s.SubmitAction(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	applier.fail = true
	_, err = s.SubmitAction(context.Background(), req)
	if !errors.As(err, &se) || se.Code != CodeInternal || se.Reason != "action_failed" {
		t.Fatalf("action error = %v", err)
	}
	applier.queued = true
	_, err = s.SubmitAction(context.Background(), req)
	if !errors.As(err, &se) || se.Code != CodeRateLimited || se.Reason != "action_queued" || se.Message != "provider failed" {
		t.Fatalf("queued action error = %v", err)
	}
	req.ThreadID = ""
	_, err = s.SubmitAction(context.Background(), req)
	if !errors.As(err, &se) || se.Code != CodeInvalid {
		t.Fatalf("validation = %v", err)
	}
}
