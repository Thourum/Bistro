package services

import (
	"errors"
	"testing"
)

type panicRecorder struct {
	op        string
	recovered any
	stack     []byte
}

func (r *panicRecorder) ReportPanic(op string, recovered any, stack []byte) {
	r.op, r.recovered, r.stack = op, recovered, stack
}

func TestGuardRecoversAndReportsPanic(t *testing.T) {
	reporter := &panicRecorder{}
	got, err := guard(reporter, "X.Y", func() (string, error) {
		panic("boom")
	})
	if got != "" {
		t.Fatalf("got %q, want zero value", got)
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != CodeInternal || se.Message != "internal error" {
		t.Fatalf("got %v, want internal ServiceError", err)
	}
	if reporter.op != "X.Y" || reporter.recovered != "boom" || len(reporter.stack) == 0 {
		t.Fatalf("panic report: %+v", reporter)
	}
}

func TestGuardReturnsValueAndNormalizesErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "plain error", err: errors.New("boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := guard(nil, "X.Y", func() (string, error) { return "value", tt.err })
			if got != "value" {
				t.Fatalf("got %q, want value", got)
			}
			if tt.err == nil {
				if err != nil {
					t.Fatalf("got %v, want nil", err)
				}
				return
			}
			se, ok := err.(*ServiceError)
			if !ok || se.Code != CodeInternal || se.Message != "boom" {
				t.Fatalf("got %v, want internal ServiceError", err)
			}
		})
	}
}

func TestGuardErrRecoversWithoutReporter(t *testing.T) {
	err := guardErr(nil, "X.Y", func() error { panic("boom") })
	se, ok := err.(*ServiceError)
	if !ok || se.Code != CodeInternal {
		t.Fatalf("got %v, want internal ServiceError", err)
	}
}
