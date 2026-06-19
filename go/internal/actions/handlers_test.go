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
