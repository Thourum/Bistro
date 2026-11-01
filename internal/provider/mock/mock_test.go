package mock_test

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

// Compile-time: MockProvider must implement ThreadProvider.
var _ provider.ThreadProvider = (*mock.MockProvider)(nil)

func TestMockProvider_GetReviewThreads(t *testing.T) {
	m := mock.NewMockProvider()
	want := []*provider.ReviewThread{
		{ID: "t1", PRID: "p1", StateKnown: true},
	}
	m.SetReviewThreads(want)

	threads, err := m.GetReviewThreads(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != "t1" {
		t.Errorf("got %+v", threads)
	}
}

func TestMockProvider_GetReviewThreads_Error(t *testing.T) {
	m := mock.NewMockProvider()
	m.SetReviewThreadsError(context.DeadlineExceeded)

	_, err := m.GetReviewThreads(context.Background(), "p1")
	if err == nil {
		t.Error("expected error from SetReviewThreadsError, got nil")
	}
}
