package provider

import (
	"errors"
	"testing"
)

func TestProviderError_Error(t *testing.T) {
	pe := &ProviderError{Type: ErrorTypeNotFound, Message: "not found", Status: 404}
	if pe.Error() != "not found" {
		t.Errorf("Error(): want %q, got %q", "not found", pe.Error())
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("repo not found")
	if err.Type != ErrorTypeNotFound {
		t.Errorf("Type: want ErrorTypeNotFound, got %v", err.Type)
	}
	if err.Status != 404 {
		t.Errorf("Status: want 404, got %d", err.Status)
	}
}

func TestNewUnauthorizedError(t *testing.T) {
	err := NewUnauthorizedError("unauthorized")
	if err.Type != ErrorTypeUnauthorized {
		t.Errorf("Type: want ErrorTypeUnauthorized, got %v", err.Type)
	}
	if err.Status != 401 {
		t.Errorf("Status: want 401, got %d", err.Status)
	}
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError("rate limited")
	if err.Type != ErrorTypeRateLimit {
		t.Errorf("Type: want ErrorTypeRateLimit, got %v", err.Type)
	}
	if err.Status != 429 {
		t.Errorf("Status: want 429, got %d", err.Status)
	}
}

func TestNewNetworkError(t *testing.T) {
	err := NewNetworkError("connection refused")
	if err.Type != ErrorTypeNetworkError {
		t.Errorf("Type: want ErrorTypeNetworkError, got %v", err.Type)
	}
	if err.Status != 0 {
		t.Errorf("Status: want 0 for network error, got %d", err.Status)
	}
}

func TestIsRecoverable_RateLimit(t *testing.T) {
	err := NewRateLimitError("rate limited")
	if !IsRecoverable(err) {
		t.Error("rate limit error should be recoverable")
	}
}

func TestIsRecoverable_Network(t *testing.T) {
	err := NewNetworkError("timeout")
	if !IsRecoverable(err) {
		t.Error("network error should be recoverable")
	}
}

func TestIsRecoverable_NotFound(t *testing.T) {
	err := NewNotFoundError("not found")
	if IsRecoverable(err) {
		t.Error("not found error should not be recoverable")
	}
}

func TestIsRecoverable_Unauthorized(t *testing.T) {
	err := NewUnauthorizedError("unauthorized")
	if IsRecoverable(err) {
		t.Error("unauthorized error should not be recoverable")
	}
}

func TestIsRecoverable_NonProviderError(t *testing.T) {
	err := errors.New("generic error")
	if IsRecoverable(err) {
		t.Error("generic error should not be recoverable")
	}
}

func TestWrapHTTPError_401(t *testing.T) {
	err := WrapHTTPError(401, "failed: %s", "bad token")
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T", err)
	}
	if pe.Type != ErrorTypeUnauthorized {
		t.Errorf("Type: want ErrorTypeUnauthorized, got %v", pe.Type)
	}
}

func TestWrapHTTPError_404(t *testing.T) {
	err := WrapHTTPError(404, "repo not found")
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T", err)
	}
	if pe.Type != ErrorTypeNotFound {
		t.Errorf("Type: want ErrorTypeNotFound, got %v", pe.Type)
	}
}

func TestWrapHTTPError_429(t *testing.T) {
	err := WrapHTTPError(429, "rate limited")
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T", err)
	}
	if pe.Type != ErrorTypeRateLimit {
		t.Errorf("Type: want ErrorTypeRateLimit, got %v", pe.Type)
	}
}

func TestWrapHTTPError_500(t *testing.T) {
	err := WrapHTTPError(500, "server error")
	if _, ok := err.(*ProviderError); ok {
		t.Error("500 should not return a *ProviderError")
	}
}

// TestErrorTyping verifies all error types are distinct and correctly typed.
func TestErrorTyping(t *testing.T) {
	errs := []struct {
		name     string
		err      *ProviderError
		wantType ErrorType
	}{
		{"not found", NewNotFoundError("x"), ErrorTypeNotFound},
		{"unauthorized", NewUnauthorizedError("x"), ErrorTypeUnauthorized},
		{"rate limit", NewRateLimitError("x"), ErrorTypeRateLimit},
		{"network", NewNetworkError("x"), ErrorTypeNetworkError},
	}

	for _, tc := range errs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Type != tc.wantType {
				t.Errorf("Type: want %v, got %v", tc.wantType, tc.err.Type)
			}
		})
	}
}
