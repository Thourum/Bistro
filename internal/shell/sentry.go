package shell

import (
	"errors"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// Bound shutdown flushing so crash reporting cannot stall application exit.
const sentryFlushTimeout = 2 * time.Second

// InitSentry always returns a callable flush, even when reporting is unavailable.
func InitSentry(dsn, version string, consent func() bool) (flush func(), err error) {
	flush = func() {}
	if override, ok := os.LookupEnv("SENTRY_DSN"); ok {
		dsn = override
	}
	if dsn == "" {
		return flush, nil
	}
	environment := os.Getenv("SENTRY_ENVIRONMENT")
	if environment == "" {
		environment = "production"
		if version == "dev" {
			environment = "development"
		}
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn: dsn, Release: "bistro@" + version, Environment: environment,
		TracesSampleRate: 0, BeforeSend: sentryBeforeSend(consent),
	}); err != nil {
		// The SDK's error may contain the DSN. Never expose it to logging callers.
		return flush, errors.New("crash reporting initialization failed")
	}
	return func() { sentry.Flush(sentryFlushTimeout) }, nil
}

func sentryBeforeSend(consent func() bool) func(*sentry.Event, *sentry.EventHint) *sentry.Event {
	return func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		if event == nil || consent == nil || !consent() {
			return nil
		}
		redacted := *event
		event = &redacted
		event.Exception = append([]sentry.Exception(nil), event.Exception...)
		event.Threads = append([]sentry.Thread(nil), event.Threads...)
		event.User = sentry.User{}
		event.Request = nil
		event.Breadcrumbs = nil
		event.Contexts = nil
		event.Tags = nil
		// sentry-go v0.50 removed Extra; its migration uses tags and contexts.
		event.Message = "[redacted]"
		// The SDK defaults ServerName to the machine hostname.
		event.ServerName = ""
		for i := range event.Exception {
			exception := &event.Exception[i]
			exception.Value = "[redacted]"
			exception.Type = "Error"
			exception.Stacktrace = scrubStacktrace(exception.Stacktrace)
		}
		for i := range event.Threads {
			event.Threads[i].Stacktrace = scrubStacktrace(event.Threads[i].Stacktrace)
		}
		return event
	}
}

func scrubStacktrace(stack *sentry.Stacktrace) *sentry.Stacktrace {
	if stack == nil {
		return nil
	}
	redacted := *stack
	stack = &redacted
	stack.Frames = append([]sentry.Frame(nil), stack.Frames...)
	for i := range stack.Frames {
		frame := &stack.Frames[i]
		if frame.Filename != "" {
			frame.Filename = path.Base(strings.ReplaceAll(frame.Filename, `\`, "/"))
		}
		if frame.AbsPath != "" {
			frame.AbsPath = path.Base(strings.ReplaceAll(frame.AbsPath, `\`, "/"))
		}
		frame.PreContext, frame.PostContext = nil, nil
		frame.ContextLine = ""
		frame.Vars = nil
	}
	return stack
}

// SentryReporter structurally implements the engine's panic reporter contract.
type SentryReporter struct{ Log *slog.Logger }

func (r SentryReporter) ReportPanic(op string, recovered any, _ []byte) {
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	// Panic values and raw stacks can contain tokens, personal data and paths.
	log.Error("service panic", "operation", op)
	sentry.CurrentHub().Recover(recovered)
}
