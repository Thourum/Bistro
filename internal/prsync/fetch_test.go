package prsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

func TestJoinFetchErrorsPreservesMessageAndCauses(t *testing.T) {
	commentError := errors.New("unavailable")
	reviewError := errors.New("rate limited")

	err := joinFetchErrors([]error{
		fmt.Errorf("comments: %w", commentError),
		fmt.Errorf("reviews: %w", reviewError),
	})

	if got, want := err.Error(), "comments: unavailable; reviews: rate limited"; got != want {
		t.Errorf("error message = %q, want %q", got, want)
	}
	if !errors.Is(err, commentError) || !errors.Is(err, reviewError) {
		t.Errorf("error chain must preserve both causes, got %v", err)
	}
	if got := joinFetchErrors(nil); got != nil {
		t.Errorf("no fetch errors should return nil, got %v", got)
	}
}

// The mock supplies unrelated provider methods; detail fetching itself is real.
type panicCommentsProvider struct{ *mock.MockProvider }

func (p panicCommentsProvider) GetComments(context.Context, string) ([]*provider.Comment, error) {
	panic("detail fetch panic")
}

func TestFetchPRDetailsReturnsAndReportsWorkerPanic(t *testing.T) {
	reports := 0
	orchestrator := NewOrchestrator(silentLog(), panicCommentsProvider{mock.NewMockProvider()}, nil, nil).
		WithWorkerPanicReporter(func(recovered any, stack []byte) {
			reports++
			if recovered != "detail fetch panic" || len(stack) == 0 {
				t.Errorf("panic report = %v", recovered)
			}
		})
	_, err := orchestrator.FetchPRDetails(context.Background(), []*provider.PR{{ID: "github:owner/repo:1", Number: 1}}, true, false, false)
	if err == nil || !strings.Contains(err.Error(), "panicked") || reports != 1 {
		t.Fatalf("fetch error=%v reports=%d", err, reports)
	}
}
