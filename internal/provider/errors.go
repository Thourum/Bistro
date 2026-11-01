package provider

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorType classifies provider errors for retry and recovery logic.
type ErrorType int

const (
	// ErrorTypeUnknown identifies an unclassified provider error.
	ErrorTypeUnknown ErrorType = iota
	// ErrorTypeNotFound identifies an HTTP 404 response.
	ErrorTypeNotFound
	// ErrorTypeUnauthorized identifies an HTTP 401 response.
	ErrorTypeUnauthorized
	// ErrorTypeRateLimit identifies an HTTP 429 response.
	ErrorTypeRateLimit
	// ErrorTypeNetworkError identifies a transport failure.
	ErrorTypeNetworkError
)

// ProviderError is a structured error returned by provider operations.
// It carries the error type and HTTP status code (when applicable) so
// callers can implement retry or recovery logic without string parsing.
type ProviderError struct {
	Type    ErrorType
	Message string
	Status  int // HTTP status code if applicable, 0 otherwise
	Cause   error
}

// Error returns the provider's error message.
func (pe *ProviderError) Error() string {
	return pe.Message
}

// Unwrap preserves the underlying provider client's error for errors.Is and errors.As.
func (pe *ProviderError) Unwrap() error {
	return pe.Cause
}

// NewNotFoundError returns a ProviderError for HTTP 404 conditions.
func NewNotFoundError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeNotFound, Message: msg, Status: http.StatusNotFound}
}

// NewUnauthorizedError returns a ProviderError for HTTP 401 conditions.
func NewUnauthorizedError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeUnauthorized, Message: msg, Status: http.StatusUnauthorized}
}

// NewRateLimitError returns a ProviderError for HTTP 429 conditions.
func NewRateLimitError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeRateLimit, Message: msg, Status: http.StatusTooManyRequests}
}

// NewNetworkError returns a ProviderError for network-level failures.
func NewNetworkError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeNetworkError, Message: msg}
}

// IsRecoverable reports whether the error is worth retrying.
// Rate-limit and network errors are recoverable; auth and not-found errors are not.
func IsRecoverable(err error) bool {
	var pe *ProviderError
	if !errors.As(err, &pe) {
		return false
	}
	return pe.Type == ErrorTypeRateLimit || pe.Type == ErrorTypeNetworkError
}

// WrapHTTPError converts an HTTP status code into the appropriate typed ProviderError.
// Unrecognised statuses return the formatted message as a plain error.
func WrapHTTPError(status int, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	switch status {
	case http.StatusUnauthorized:
		return NewUnauthorizedError(msg)
	case http.StatusNotFound:
		return NewNotFoundError(msg)
	case http.StatusTooManyRequests:
		return NewRateLimitError(msg)
	default:
		return errors.New(msg)
	}
}
