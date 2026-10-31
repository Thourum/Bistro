package services

import (
	"context"
	"errors"
	"fmt"
)

// Code is the closed set of error categories the renderer maps onto tagged faults.
type Code string

const (
	CodeNotFound        Code = "not_found"
	CodeUnauthenticated Code = "unauthenticated"
	CodeRateLimited     Code = "rate_limited"
	CodeInvalid         Code = "invalid"
	CodeCancelled       Code = "cancelled"
	CodeTimeout         Code = "timeout"
	CodeInternal        Code = "internal"
)

// ServiceError is the only error a service method returns. Wails marshals it
// as the rejection cause. Reason is an optional domain code the UI may branch on.
type ServiceError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

func (e *ServiceError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Reason, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Errorf formats a message in the renderer's closed error category set.
func Errorf(code Code, format string, args ...any) *ServiceError {
	return &ServiceError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithReason preserves a domain sub-code without extending Code.
func WithReason(code Code, reason, message string) *ServiceError {
	return &ServiceError{Code: code, Reason: reason, Message: message}
}

// toServiceError returns nil for nil, otherwise a *ServiceError: one already in
// the chain wins, then context errors, then internal. Returning error (not
// *ServiceError) avoids a typed-nil interface.
func toServiceError(err error) error {
	if err == nil {
		return nil
	}
	var se *ServiceError
	if errors.As(err, &se) {
		return se
	}
	switch {
	case errors.Is(err, context.Canceled):
		return &ServiceError{Code: CodeCancelled, Message: "cancelled"}
	case errors.Is(err, context.DeadlineExceeded):
		return &ServiceError{Code: CodeTimeout, Message: "timed out"}
	}
	return &ServiceError{Code: CodeInternal, Message: err.Error()}
}
