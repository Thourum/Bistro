package engine

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/prsync"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/services/servicestest"
	"github.com/itterno/bistro/internal/store/actionqueue"
	"github.com/itterno/bistro/internal/store/currentuser"
)

func TestApplyClassificationSettingsSharesServiceState(t *testing.T) {
	app := newTestApp()
	app.classificationState = services.NewClassificationState()
	req := services.ClassificationSettings{Mode: "rules+llm", DismissalScope: "local+provider", Provider: "openai", Model: "gpt-5", Endpoint: "http://localhost"}
	if err := app.ApplyClassificationSettings(req); err != nil {
		t.Fatal(err)
	}
	if got := app.classificationState.Snapshot(); got != req {
		t.Fatalf("App state = %+v, want %+v", got, req)
	}
	service := &services.Classification{State: app.classificationState}
	next := services.ClassificationSettings{Mode: "server", DismissalScope: "local", Provider: "anthropic"}
	got, err := service.SetSettings(context.Background(), next)
	if err != nil || got != next || app.classificationState.Snapshot() != next {
		t.Fatalf("service state = %+v, error = %v", got, err)
	}
	if err := app.ApplyClassificationSettings(services.ClassificationSettings{Mode: "invalid"}); err == nil {
		t.Fatal("invalid App preferences accepted")
	}
	if app.classificationState.Snapshot() != next {
		t.Fatal("invalid App preferences changed shared state")
	}
}

func TestApplySyncSettingsClampsInterval(t *testing.T) {
	for _, test := range []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{"below minimum", -time.Second, 30 * time.Second},
		{"minimum", 30 * time.Second, 30 * time.Second},
		{"within range", 5 * time.Minute, 5 * time.Minute},
		{"maximum", time.Hour, time.Hour},
		{"above maximum", 2 * time.Hour, time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := newTestApp()
			app.ApplySyncSettings(false, test.interval)
			settings := <-app.syncSettings
			if settings.AutoSync || settings.Interval != test.want {
				t.Fatalf("settings = %+v, want autoSync false, interval %s", settings, test.want)
			}
		})
	}
}

func TestStartStopOwnsDatabaseAndLoops(t *testing.T) {
	app := New(Options{DBPath: filepath.Join(t.TempDir(), "nested", "sync.db"), RulesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	// Avoid reading the real keychain; provider injection precedes startup.
	app.SetProvider(mock.NewMockProvider().WithInboxDiscovery(&provider.InboxDiscoveryResult{}, nil))
	app.SetEmitter(&servicestest.Recorder{})
	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if app.Services() == nil || app.db == nil {
		t.Fatal("Start did not prepare services and database")
	}
	if err := app.Start(ctx); err == nil {
		t.Fatal("duplicate Start accepted")
	}
	db := app.db
	finished := make(chan struct{})
	started := make(chan struct{})
	app.startLoop(app.runCtx, "shutdown-test", func(ctx context.Context) { close(started); <-ctx.Done(); close(finished) })
	<-started
	app.Stop()
	select {
	case <-finished:
	default:
		t.Fatal("Stop did not wait for background loop")
	}
	if app.Services() != nil || app.db != nil || db.Ping() == nil {
		t.Fatal("Stop did not close database and clear services")
	}
	app.Stop()
}

type blockedReplayProvider struct {
	*mock.MockProvider
	started, cancelled, release chan struct{}
}

func (p *blockedReplayProvider) PostComment(ctx context.Context, _ string, _ int, _ string) error {
	close(p.started)
	<-ctx.Done()
	close(p.cancelled)
	<-p.release
	return ctx.Err()
}

func TestStartReturnsDuringReplayAndStopWaits(t *testing.T) {
	app := New(Options{DBPath: filepath.Join(t.TempDir(), "sync.db"), RulesDir: t.TempDir(), ReplayQueue: true, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.openDatabase(ctx); err != nil {
		t.Fatal(err)
	}
	if err := currentuser.New(app.db).Set(ctx, provider.GitHub, "reviewer"); err != nil {
		t.Fatal(err)
	}
	if err := actionqueue.New(app.db.DB).Enqueue(ctx, &actionqueue.Action{
		ID: "startup-replay", ActionType: "post_comment", PRID: "github:owner/repo:42", Payload: "review nit",
		AccountProvider: provider.GitHub, AccountLogin: "reviewer",
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.db.Close(); err != nil {
		t.Fatal(err)
	}
	app.db = nil
	prov := &blockedReplayProvider{MockProvider: mock.NewMockProvider(), started: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	app.SetProvider(prov)
	startDone := make(chan error, 1)
	go func() { startDone <- app.Start(ctx) }()
	startReturned := false
	var stopDone chan struct{}
	defer func() {
		cancel()
		close(prov.release)
		if !startReturned {
			<-startDone
		}
		if stopDone != nil {
			<-stopDone
			return
		}
		app.Stop()
	}()
	select {
	case <-prov.started:
	case <-ctx.Done():
		t.Fatal("startup replay did not reach provider")
	}
	select {
	case err := <-startDone:
		startReturned = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Start blocked on replay")
	}
	if app.Services() == nil {
		t.Fatal("Start returned without services")
	}
	db := app.db
	stopDone = make(chan struct{})
	go func() { app.Stop(); close(stopDone) }()
	select {
	case <-prov.cancelled:
	case <-ctx.Done():
		t.Fatal("Stop did not cancel replay")
	}
	select {
	case <-stopDone:
		t.Fatal("Stop returned before replay finished")
	default:
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("SQLite closed before replay finished: %v", err)
	}
	prov.release <- struct{}{}
	select {
	case <-stopDone:
	case <-ctx.Done():
		t.Fatal("Stop did not finish after replay exited")
	}
	if db.Ping() == nil {
		t.Fatal("Stop did not close SQLite")
	}
}

func TestSyncPartialFailuresEmitError(t *testing.T) {
	app := newTestApp()
	app.reportSyncResult("repo", &prsync.SyncResult{Errors: []string{"worker task panicked: boom"}})
	payloads := app.emitter.(*servicestest.Recorder).Named(services.EventSyncError)
	if len(payloads) != 1 {
		t.Fatalf("sync errors = %v", payloads)
	}
	failure, ok := payloads[0].(services.SyncError)
	if !ok || failure.ErrorCode != "SYNC_FAILED" || !strings.Contains(failure.Message, "panicked") {
		t.Fatalf("sync error = %#v", payloads[0])
	}
}
