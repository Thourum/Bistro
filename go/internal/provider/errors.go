package provider

import "fmt"

// ErrorType classifies provider errors for retry and recovery logic.
type ErrorType int

const (
	ErrorTypeUnknown    ErrorType = iota
	ErrorTypeNotFound             // HTTP 404
	ErrorTypeUnauthorized         // HTTP 401
	ErrorTypeRateLimit            // HTTP 429
	ErrorTypeNetworkError
)

// ProviderError is a structured error returned by provider operations.
// It carries the error type and HTTP status code (when applicable) so
// callers can implement retry or recovery logic without string parsing.
type ProviderError struct {
	Type    ErrorType
	Message string
	Status  int // HTTP status code if applicable, 0 otherwise
}

func (pe *ProviderError) Error() string {
	return pe.Message
}

// NewNotFoundError returns a ProviderError for HTTP 404 conditions.
func NewNotFoundError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeNotFound, Message: msg, Status: 404}
}

// NewUnauthorizedError returns a ProviderError for HTTP 401 conditions.
func NewUnauthorizedError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeUnauthorized, Message: msg, Status: 401}
}

// NewRateLimitError returns a ProviderError for HTTP 429 conditions.
func NewRateLimitError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeRateLimit, Message: msg, Status: 429}
}

// NewNetworkError returns a ProviderError for network-level failures.
func NewNetworkError(msg string) *ProviderError {
	return &ProviderError{Type: ErrorTypeNetworkError, Message: msg}
}

// IsRecoverable reports whether the error is worth retrying.
// Rate-limit and network errors are recoverable; auth and not-found errors are not.
func IsRecoverable(err error) bool {
	pe, ok := err.(*ProviderError)
	if !ok {
		return false
	}
	return pe.Type == ErrorTypeRateLimit || pe.Type == ErrorTypeNetworkError
}

// WrapHTTPError converts an HTTP status code into the appropriate typed ProviderError.
// If the status code is not recognised the original error is wrapped as-is.
func WrapHTTPError(status int, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	switch status {
	case 401:
		return NewUnauthorizedError(msg)
	case 404:
		return NewNotFoundError(msg)
	case 429:
		return NewRateLimitError(msg)
	default:
		return fmt.Errorf("%s", msg)
	}
}
