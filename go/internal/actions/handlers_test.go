// go/internal/actions/handlers_test.go
package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
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
