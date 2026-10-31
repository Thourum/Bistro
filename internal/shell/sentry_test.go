package shell

import (
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestSentryBeforeSendConsentAndRedaction(t *testing.T) {
	consent := false
	beforeSend := sentryBeforeSend(func() bool { return consent })
	event := &sentry.Event{
		Message: "secret panic", ServerName: "private hostname",
		User:        sentry.User{Email: "private@example.com"},
		Request:     &sentry.Request{URL: "https://private.example"},
		Breadcrumbs: []*sentry.Breadcrumb{{Message: "secret"}},
		Contexts:    map[string]sentry.Context{"private": {"secret": true}},
		Tags:        map[string]string{"private": "secret"},
		Exception: []sentry.Exception{{Type: "PrivateError", Value: "secret value", Stacktrace: &sentry.Stacktrace{
			Frames: []sentry.Frame{{Filename: "/Users/name/src/file.go", AbsPath: `C:\Users\name\src\file.go`, ContextLine: "secret source", Vars: map[string]any{"token": "secret"}}},
		}}},
		Threads: []sentry.Thread{{Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{{Filename: "/Users/name/thread.go"}}}}},
	}
	if got := beforeSend(event, nil); got != nil {
		t.Fatal("event sent without consent")
	}
	consent = true
	got := beforeSend(event, nil)
	if got == nil || got.Message != "[redacted]" || got.ServerName != "" || !reflect.DeepEqual(got.User, sentry.User{}) {
		t.Fatalf("message or identity not redacted: %+v", got)
	}
	if got.Request != nil || len(got.Breadcrumbs) != 0 || got.Contexts != nil || got.Tags != nil {
		t.Fatal("private event metadata retained")
	}
	exception := got.Exception[0]
	if exception.Type != "Error" || exception.Value != "[redacted]" {
		t.Fatal("exception not redacted")
	}
	frame := exception.Stacktrace.Frames[0]
	if frame.Filename != "file.go" || frame.AbsPath != "file.go" || frame.ContextLine != "" || frame.Vars != nil {
		t.Fatalf("frame not redacted: %+v", frame)
	}
	if got.Threads[0].Stacktrace.Frames[0].Filename != "thread.go" {
		t.Fatal("thread frame path retained")
	}
	if event.Message != "secret panic" || event.Exception[0].Stacktrace.Frames[0].Filename != "/Users/name/src/file.go" {
		t.Fatal("redaction mutated caller event")
	}
}

func TestInitSentryAlwaysReturnsFlush(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	flush, err := InitSentry("ignored by empty override", "dev", func() bool { return false })
	if err != nil || flush == nil {
		t.Fatalf("empty DSN: callable flush required, err=%v", err)
	}
	flush()
	t.Setenv("SENTRY_DSN", "invalid secret DSN")
	flush, err = InitSentry("", "dev", func() bool { return false })
	if err == nil || flush == nil || err.Error() != "crash reporting initialization failed" {
		t.Fatalf("invalid DSN: safe error and callable flush required")
	}
	flush()
}
