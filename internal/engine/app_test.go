package engine

import (
	"context"
	"github.com/itterno/bistro/internal/services"
	"testing"
	"time"
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
