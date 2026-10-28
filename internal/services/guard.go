package services

import "runtime/debug"

// PanicReporter receives panics recovered from service calls (Sentry in production).
type PanicReporter interface {
	ReportPanic(op string, recovered any, stack []byte)
}

// guard runs the body of service method op. A panic becomes an internal
// ServiceError and is reported, so one broken call cannot take the app down.
func guard[T any](rep PanicReporter, op string, fn func() (T, error)) (out T, err error) {
	defer func() {
		if r := recover(); r != nil {
			if rep != nil {
				rep.ReportPanic(op, r, debug.Stack())
			}
			var zero T
			out, err = zero, &ServiceError{Code: CodeInternal, Message: "internal error"}
		}
	}()
	out, err = fn()
	return out, toServiceError(err)
}

func guardErr(rep PanicReporter, op string, fn func() error) error {
	_, err := guard(rep, op, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}
