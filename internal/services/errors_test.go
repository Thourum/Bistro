package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestToServiceError(t *testing.T) {
	domainError := WithReason(CodeInvalid, "x", "m")
	tests := []struct {
		name string
		err  error
		want *ServiceError
	}{
		{name: "nil"},
		{name: "wrapped service error", err: fmt.Errorf("wrap: %w", domainError), want: domainError},
		{name: "service error wins over context", err: errors.Join(context.Canceled, domainError), want: domainError},
		{name: "wrapped cancellation", err: fmt.Errorf("wrap: %w", context.Canceled), want: &ServiceError{Code: CodeCancelled, Message: "cancelled"}},
		{name: "deadline", err: context.DeadlineExceeded, want: &ServiceError{Code: CodeTimeout, Message: "timed out"}},
		{name: "plain error", err: errors.New("boom"), want: &ServiceError{Code: CodeInternal, Message: "boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toServiceError(tt.err)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			se, ok := got.(*ServiceError)
			if !ok {
				t.Fatalf("got %T, want *ServiceError", got)
			}
			if *se != *tt.want {
				t.Fatalf("got %+v, want %+v", se, tt.want)
			}
			if tt.want == domainError && se != domainError {
				t.Fatal("existing service error was replaced")
			}
		})
	}
}
