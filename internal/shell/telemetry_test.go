package shell

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
)

func TestResolveTelemetryEndpoint(t *testing.T) {
	for _, test := range []struct {
		endpoint string
		accepted bool
	}{
		{"https://example.com", true},
		{"http://127.0.0.1:9", true},
		{"http://localhost:9", true},
		{"http://[::1]:9", true},
		{"http://example.com", false},
		{"https://u:p@x", false},
		{"https://", false},
		{"file:///tmp/events", false},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			if got := resolveTelemetryEndpoint(test.endpoint); (got != "") != test.accepted {
				t.Fatalf("resolved %q, accepted=%v", got, test.accepted)
			}
		})
	}
}

func telemetryRecorder(t *testing.T) (*Telemetry, *[]posthog.Capture) {
	t.Helper()
	var events []posthog.Capture
	telemetry := &Telemetry{
		log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		installIDPath: filepath.Join(t.TempDir(), "telemetry", "anonymous-install-id"),
		capture: func(event posthog.Capture) error {
			events = append(events, event)
			return nil
		},
	}
	telemetry.SetEnabled(true)
	return telemetry, &events
}

func TestTrackSettingsChangedIgnoresWindow(t *testing.T) {
	telemetry, events := telemetryRecorder(t)
	prev := DefaultSettings()
	next := prev
	next.Navigation.Style = "emacs"
	next.Window.Width = 800
	telemetry.TrackSettingsChanged(prev, next)
	if len(*events) != 1 {
		t.Fatalf("got %d events, want 1", len(*events))
	}
	event := (*events)[0]
	want := posthog.Properties{"section": "navigation", "style": "emacs", "telemetrySchemaVersion": 1}
	if event.Event != "settings_updated" || !reflect.DeepEqual(event.Properties, want) {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestDisabledTelemetryEnqueuesNothing(t *testing.T) {
	telemetry, events := telemetryRecorder(t)
	telemetry.SetEnabled(false)
	telemetry.TrackAppStarted("1.2.3")
	prev, next := DefaultSettings(), DefaultSettings()
	next.Navigation.Style = "emacs"
	telemetry.TrackSettingsChanged(prev, next)
	if len(*events) != 0 {
		t.Fatalf("disabled telemetry sent %d events", len(*events))
	}
	if _, err := os.Stat(telemetry.installIDPath); !os.IsNotExist(err) {
		t.Fatalf("disabled telemetry wrote install id: %v", err)
	}
}

func TestAppStartedVersionAndInstallID(t *testing.T) {
	telemetry, events := telemetryRecorder(t)
	telemetry.TrackAppStarted("1.2.3-beta+build")
	telemetry.TrackAppStarted("private version /Users/name")
	if len(*events) != 2 {
		t.Fatalf("got %d events, want 2", len(*events))
	}
	first, second := (*events)[0], (*events)[1]
	if first.Properties["appVersion"] != "1.2.3-beta+build" {
		t.Fatalf("valid version omitted: %+v", first.Properties)
	}
	if _, exists := second.Properties["appVersion"]; exists {
		t.Fatal("invalid version was sent")
	}
	if _, err := uuid.Parse(first.DistinctId); err != nil || first.DistinctId != second.DistinctId {
		t.Fatal("expected stable UUID distinct id")
	}
	persisted, err := loadTelemetryInstallID(telemetry.installIDPath)
	if err != nil || persisted != first.DistinctId {
		t.Fatalf("persisted id differs: %v", err)
	}
	for _, event := range *events {
		if event.Properties["telemetrySchemaVersion"] != 1 || event.Properties["platform"] == "" {
			t.Fatalf("missing common properties: %+v", event.Properties)
		}
	}
}

func TestTrackSettingsSectionsNeverIncludeValues(t *testing.T) {
	telemetry, events := telemetryRecorder(t)
	prev, next := DefaultSettings(), DefaultSettings()
	next.Appearance.Theme = "dark"
	next.Keybindings = map[string][]string{"private action": {"private shortcut"}}
	next.PRList.View = "grouped"
	next.Privacy.TelemetryEnabled = true
	next.Sync.AutoSync = false
	next.Diff.Style = "split"
	privatePath := "/Users/private/repo"
	next.Repositories.DevFolderPath = &privatePath
	next.Classification.Endpoint = "https://private.example"
	telemetry.TrackSettingsChanged(prev, next)
	sections := map[string]int{}
	for _, event := range *events {
		sections[event.Properties["section"].(string)]++
		for key := range event.Properties {
			if key != "section" && key != "telemetrySchemaVersion" && key != "telemetryEnabled" {
				t.Fatalf("unexpected property %q", key)
			}
		}
	}
	want := map[string]int{"appearance": 1, "keyboard": 1, "prList": 1, "privacy": 1, "sync": 1, "advanced": 3}
	if !reflect.DeepEqual(sections, want) {
		t.Fatalf("got sections %v, want %v", sections, want)
	}
}
