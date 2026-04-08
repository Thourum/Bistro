package mock

import (
	"context"
	"fmt"
	"testing"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

func TestNewMockProvider(t *testing.T) {
	mp := NewMockProvider()
	if mp == nil {
		t.Error("expected non-nil mock provider")
	}
}

func TestMockProviderImplementsInterface(t *testing.T) {
	var _ provider.Provider = (*MockProvider)(nil)
}

func TestMockListPRs(t *testing.T) {
	mp := NewMockProvider().WithPRs([]*provider.PR{
		{
			ID:     "github:org/repo:123",
			Number: 123,
			Title:  "Test PR",
			Author: "alice",
		},
	})

	ctx := context.Background()
	prs, err := mp.ListPRs(ctx, provider.PRFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Errorf("expected 1 PR, got %d", len(prs))
	}
}

func TestMockGetPR(t *testing.T) {
	pr := &provider.PR{
		ID:     "github:org/repo:123",
		Number: 123,
		Title:  "Test PR",
	}

	mp := NewMockProvider().WithPRs([]*provider.PR{pr})

	ctx := context.Background()
	result, err := mp.GetPR(ctx, pr.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != pr.ID {
		t.Errorf("expected ID %s, got %s", pr.ID, result.ID)
	}
}

func TestMockGetPRNotFound(t *testing.T) {
	mp := NewMockProvider()

	ctx := context.Background()
	_, err := mp.GetPR(ctx, "github:org/repo:999")
	if err == nil {
		t.Error("expected error for missing PR, got nil")
	}
}

func TestMockGetComments(t *testing.T) {
	comments := []*provider.Comment{
		{
			ID:     "comment-1",
			Author: "bob",
			Body:   "Great work!",
		},
	}

	mp := NewMockProvider().WithComments(comments)

	ctx := context.Background()
	result, err := mp.GetComments(ctx, "github:org/repo:123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 comment, got %d", len(result))
	}
}

func TestMockGetCIStatus(t *testing.T) {
	status := &provider.CIStatus{
		Status: "success",
		Checks: []*provider.Check{
			{Name: "Tests", Status: "success"},
		},
	}

	mp := NewMockProvider().WithCIStatus(status)

	ctx := context.Background()
	result, err := mp.GetCIStatus(ctx, "github:org/repo:123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("expected status success, got %s", result.Status)
	}
}

func TestMockApprove(t *testing.T) {
	mp := NewMockProvider()

	ctx := context.Background()
	err := mp.Approve(ctx, "github:org/repo:123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockRequestChanges(t *testing.T) {
	mp := NewMockProvider()

	ctx := context.Background()
	err := mp.RequestChanges(ctx, "github:org/repo:123", "Please fix this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockPostComment(t *testing.T) {
	mp := NewMockProvider()

	ctx := context.Background()
	err := mp.PostComment(ctx, "github:org/repo:123", 0, "Great work!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockAuthenticate(t *testing.T) {
	mp := NewMockProvider()

	ctx := context.Background()
	token, err := mp.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "mock-token" {
		t.Errorf("expected mock-token, got %s", token)
	}
}

func TestMockWithError(t *testing.T) {
	customErr := fmt.Errorf("mock error")
	mp := NewMockProvider().WithError(customErr)

	ctx := context.Background()

	_, err := mp.ListPRs(ctx, provider.PRFilters{})
	if err != customErr {
		t.Errorf("expected custom error from ListPRs, got %v", err)
	}

	_, err = mp.GetPR(ctx, "id")
	if err != customErr {
		t.Errorf("expected custom error from GetPR, got %v", err)
	}

	_, err = mp.GetComments(ctx, "id")
	if err != customErr {
		t.Errorf("expected custom error from GetComments, got %v", err)
	}

	_, err = mp.GetCIStatus(ctx, "id")
	if err != customErr {
		t.Errorf("expected custom error from GetCIStatus, got %v", err)
	}

	err = mp.Approve(ctx, "id")
	if err != customErr {
		t.Errorf("expected custom error from Approve, got %v", err)
	}

	err = mp.RequestChanges(ctx, "id", "body")
	if err != customErr {
		t.Errorf("expected custom error from RequestChanges, got %v", err)
	}

	err = mp.PostComment(ctx, "id", 0, "body")
	if err != customErr {
		t.Errorf("expected custom error from PostComment, got %v", err)
	}
}
