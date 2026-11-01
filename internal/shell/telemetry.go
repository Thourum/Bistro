package shell

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
)

const (
	defaultTelemetryEndpoint = "https://us.i.posthog.com"
	telemetrySchemaVersion   = 1
	telemetryShutdownTimeout = 3 * time.Second
)

// Telemetry sends only allowlisted properties while consent is enabled.
type Telemetry struct {
	mu            sync.Mutex
	enabled       bool
	installID     string
	installIDPath string
	log           *slog.Logger
	capture       func(posthog.Capture) error
	close         func() error
}

func NewTelemetry(dataDir string, logger *slog.Logger) *Telemetry {
	if logger == nil {
		logger = slog.Default()
	}
	telemetry := &Telemetry{
		log:           logger,
		installIDPath: filepath.Join(dataDir, "telemetry", "anonymous-install-id"),
	}
	key := firstTelemetryEnv("BISTRO_POSTHOG_API_KEY", "POSTHOG_API_KEY")
	if key == "" {
		return telemetry
	}
	endpoint := defaultTelemetryEndpoint
	for _, name := range []string{"BISTRO_TELEMETRY_ENDPOINT", "BISTRO_POSTHOG_HOST", "POSTHOG_HOST"} {
		if candidate := resolveTelemetryEndpoint(os.Getenv(name)); candidate != "" {
			endpoint = candidate
			break
		}
	}
	client, err := posthog.NewWithConfig(key, posthog.Config{
		Endpoint: endpoint, ShutdownTimeout: telemetryShutdownTimeout,
		// SDK diagnostics can include request bodies and credentials.
		Logger:   posthog.StdLogger(log.New(io.Discard, "", 0), false),
		IsServer: posthog.Ptr(false),
	})
	if err != nil {
		logger.Warn("telemetry initialization failed")
		return telemetry
	}
	telemetry.capture = func(event posthog.Capture) error { return client.Enqueue(event) }
	telemetry.close = client.Close
	return telemetry
}

func firstTelemetryEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func resolveTelemetryEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Opaque != "" {
		return ""
	}
	if endpoint.Scheme == "https" {
		return raw
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme == "http" && (endpoint.Hostname() == "localhost" || ip != nil && ip.IsLoopback()) {
		return raw
	}
	return ""
}

func (telemetry *Telemetry) SetEnabled(enabled bool) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	telemetry.enabled = enabled
}

func (telemetry *Telemetry) TrackAppStarted(version string) {
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}
	properties := posthog.Properties{"platform": platform}
	if regexp.MustCompile(`^[0-9A-Za-z.+-]{1,32}$`).MatchString(version) {
		properties["appVersion"] = version
	}
	telemetry.track("app_started", properties)
}

func (telemetry *Telemetry) TrackSettingsChanged(prev, next Settings) {
	sections := []struct {
		name       string
		prev, next any
		properties posthog.Properties
	}{
		{"appearance", prev.Appearance, next.Appearance, nil},
		{"keyboard", prev.Keybindings, next.Keybindings, nil},
		{"prList", prev.PRList, next.PRList, nil},
		{"navigation", prev.Navigation, next.Navigation, posthog.Properties{"style": next.Navigation.Style}},
		{"privacy", prev.Privacy, next.Privacy, posthog.Properties{"telemetryEnabled": next.Privacy.TelemetryEnabled}},
		{"sync", prev.Sync, next.Sync, nil},
		{"advanced", prev.Diff, next.Diff, nil},
		{"advanced", prev.Repositories, next.Repositories, nil},
		{"advanced", prev.Classification, next.Classification, nil},
	}
	for _, section := range sections {
		if reflect.DeepEqual(section.prev, section.next) {
			continue
		}
		properties := posthog.Properties{"section": section.name}
		for name, value := range section.properties {
			properties[name] = value
		}
		// Navigation settings may come from callers other than SettingsStore.
		if style, ok := properties["style"].(string); ok && !validTelemetryStyle(style) {
			delete(properties, "style")
		}
		telemetry.track("settings_updated", properties)
	}
}

func validTelemetryStyle(style string) bool {
	return style == "vim" || style == "emacs" || style == "standard"
}

func (telemetry *Telemetry) track(name string, properties posthog.Properties) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	if !telemetry.enabled || telemetry.capture == nil {
		return
	}
	if telemetry.installID == "" {
		installID, err := loadTelemetryInstallID(telemetry.installIDPath)
		if err != nil {
			telemetry.log.Warn("telemetry install id unavailable")
			return
		}
		telemetry.installID = installID
	}
	properties["telemetrySchemaVersion"] = telemetrySchemaVersion
	if err := telemetry.capture(posthog.Capture{
		DistinctId: telemetry.installID, Event: name, Properties: properties,
	}); err != nil {
		telemetry.log.Warn("telemetry enqueue failed")
	}
}

func loadTelemetryInstallID(filename string) (string, error) {
	stored, err := os.ReadFile(filename)
	if err == nil {
		id := strings.TrimSpace(string(stored))
		if parsed, parseErr := uuid.Parse(id); parseErr != nil || parsed.String() != id {
			return "", fmt.Errorf("invalid anonymous install id")
		}
		return id, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return "", err
	}
	installID, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	id := installID.String()
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return loadTelemetryInstallID(filename)
	}
	if err != nil {
		return "", err
	}
	_, writeErr := file.WriteString(id + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	return id, closeErr
}

func (telemetry *Telemetry) Close() {
	telemetry.mu.Lock()
	telemetry.enabled = false
	closeClient := telemetry.close
	telemetry.close, telemetry.capture = nil, nil
	telemetry.mu.Unlock()
	if closeClient != nil {
		if err := closeClient(); err != nil {
			telemetry.log.Warn("telemetry shutdown failed")
		}
	}
}
