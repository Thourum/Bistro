package sync

import (
	"errors"
	"fmt"
	"testing"
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
