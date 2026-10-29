// Package shell owns desktop settings independently of engine services.
package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"
)

// Settings is the renderer settings snapshot plus shell-owned window bounds.
type Settings struct {
	Schema         int                    `json:"$schema"`
	Appearance     AppearanceSettings     `json:"appearance"`
	Sync           SyncSettings           `json:"sync"`
	PRList         PRListSettings         `json:"prList"`
	Diff           DiffSettings           `json:"diff"`
	Privacy        PrivacySettings        `json:"privacy"`
	Repositories   RepositorySettings     `json:"repositories"`
	Classification ClassificationSettings `json:"classification"`
	Navigation     NavigationSettings     `json:"navigation"`
	Keybindings    map[string][]string    `json:"keybindings"`
	Window         WindowBounds           `json:"window"`
}

type AppearanceSettings struct {
	Theme              string  `json:"theme"`
	UIFont             string  `json:"uiFont"`
	CodeFont           string  `json:"codeFont"`
	UIFontSize         float64 `json:"uiFontSize"`
	CodeFontSize       float64 `json:"codeFontSize"`
	HighDensity        bool    `json:"highDensity"`
	OnboardingComplete bool    `json:"onboardingComplete"`
	TourComplete       bool    `json:"tourComplete"`
}

type SyncSettings struct {
	AutoSync      bool    `json:"autoSync"`
	SyncInterval  float64 `json:"syncInterval"`
	Notifications bool    `json:"notifications"`
}

type PRListSettings struct {
	View string `json:"view"`
}
type DiffSettings struct {
	Style string `json:"style"`
}
type PrivacySettings struct {
	TelemetryEnabled bool `json:"telemetryEnabled"`
}
type RepositorySettings struct {
	DevFolderPath *string `json:"devFolderPath"`
}
type ClassificationSettings struct {
	Mode           string `json:"mode"`
	DismissalScope string `json:"dismissalScope"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Endpoint       string `json:"endpoint"`
}
type NavigationSettings struct {
	Style string `json:"style"`
}

// WindowBounds is shell-owned; zero Width means centre at the default size.
type WindowBounds struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

// DefaultSettings matches electron/src/types/settings.ts, with zero window bounds.
func DefaultSettings() Settings {
	return Settings{
		Schema: 1,
		Appearance: AppearanceSettings{
			Theme:      "light",
			UIFont:     `"Geist Variable", "Geist", "Inter", system-ui, sans-serif`,
			CodeFont:   `"JetBrains Mono", "Geist Mono Variable", "Fira Code", "IBM Plex Mono", monospace`,
			UIFontSize: 14, CodeFontSize: 13,
		},
		Sync:           SyncSettings{AutoSync: true, SyncInterval: 60, Notifications: true},
		PRList:         PRListSettings{View: "flat"},
		Diff:           DiffSettings{Style: "unified"},
		Classification: ClassificationSettings{Mode: "rules", DismissalScope: "local", Provider: "anthropic"},
		Navigation:     NavigationSettings{Style: "vim"},
		Keybindings:    map[string][]string{},
	}
}

// SettingsStore serializes settings persistence and polling. Snapshots are copies.
type SettingsStore struct {
	mu        sync.Mutex
	path      string
	log       *slog.Logger
	current   Settings
	mtime     time.Time
	listeners []func(prev, next Settings)
}

// NewSettingsStore constructs a store; Load reads its initial disk state.
func NewSettingsStore(path string, log *slog.Logger) *SettingsStore {
	if log == nil {
		log = slog.Default()
	}
	return &SettingsStore{path: path, log: log, current: DefaultSettings()}
}

// Load uses defaults for missing or invalid JSON, without notifying listeners.
func (s *SettingsStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, mtime, err := readSettings(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.current = normalise(raw, DefaultSettings())
	s.mtime = mtime
	return nil
}

// Get returns an independent snapshot.
func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSettings(s.current)
}

// Path returns the settings file location.
func (s *SettingsStore) Path() string { return s.path }

// OnChange registers a listener called outside the store lock with copies.
func (s *SettingsStore) OnChange(listener func(prev, next Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, listener)
}

// Set retains invalid leaves and shell-owned window bounds.
func (s *SettingsStore) Set(next Settings) (Settings, error) {
	s.mu.Lock()
	// JSON cannot represent non-finite numbers; treat them as invalid leaves.
	if math.IsNaN(next.Appearance.UIFontSize) || math.IsInf(next.Appearance.UIFontSize, 0) {
		next.Appearance.UIFontSize = s.current.Appearance.UIFontSize
	}
	if math.IsNaN(next.Appearance.CodeFontSize) || math.IsInf(next.Appearance.CodeFontSize, 0) {
		next.Appearance.CodeFontSize = s.current.Appearance.CodeFontSize
	}
	if math.IsNaN(next.Sync.SyncInterval) || math.IsInf(next.Sync.SyncInterval, 0) {
		next.Sync.SyncInterval = s.current.Sync.SyncInterval
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		s.mu.Unlock()
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		s.mu.Unlock()
		return Settings{}, fmt.Errorf("decode settings snapshot: %w", err)
	}
	next = normalise(raw, s.current)
	next.Window = s.current.Window
	return s.commit(next)
}

// Reset persists defaults, including zero window bounds.
func (s *SettingsStore) Reset() (Settings, error) {
	s.mu.Lock()
	return s.commit(DefaultSettings())
}

// commit consumes the lock acquired by Set or Reset; failed writes leave state intact.
func (s *SettingsStore) commit(next Settings) (Settings, error) {
	if err := s.write(next); err != nil {
		s.mu.Unlock()
		return Settings{}, err
	}
	prev := s.current
	s.current = next
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	notifySettings(listeners, prev, next)
	return cloneSettings(next), nil
}

// SetWindow persists valid bounds without notifying renderer settings listeners.
func (s *SettingsStore) SetWindow(bounds WindowBounds) error {
	if !validWindow(bounds) {
		return fmt.Errorf("window width and height must be zero or at least %d", minimumWindowSize)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneSettings(s.current)
	next.Window = bounds
	if err := s.write(next); err != nil {
		return err
	}
	s.current = next
	return nil
}

func (s *SettingsStore) write(next Settings) error {
	encoded, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return fmt.Errorf("create settings temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write settings: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync settings: %w", err)
	}
	info, err := file.Stat()
	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("stat settings temporary file: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close settings temporary file: %w", closeErr)
	}
	if err := os.Rename(file.Name(), s.path); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}
	s.mtime = info.ModTime()
	return nil
}

// Watch blocks until cancellation, polling for external edits.
func (s *SettingsStore) Watch(ctx context.Context, every time.Duration) {
	if every <= 0 {
		s.log.Warn("Settings watch interval must be positive")
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reloadExternal()
		}
	}
}

func (s *SettingsStore) reloadExternal() {
	s.mu.Lock()
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.mu.Unlock()
		s.log.Warn("Cannot stat settings file")
		return
	}
	if info.ModTime().Equal(s.mtime) {
		s.mu.Unlock()
		return
	}
	raw, mtime, err := readSettings(s.path)
	if err != nil {
		s.mu.Unlock()
		s.log.Warn("Cannot read settings file")
		return
	}
	s.mtime = mtime
	if raw == nil {
		s.mu.Unlock()
		s.log.Warn("Settings file is not a JSON object")
		return
	}
	prev := s.current
	next := normalise(raw, prev)
	s.current = next
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	notifySettings(listeners, prev, next)
}

func readSettings(path string) (map[string]json.RawMessage, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("open settings: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("stat settings: %w", err)
	}
	// Read the whole document so trailing garbage is rejected as well.
	encoded, err := io.ReadAll(file)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read settings: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return nil, info.ModTime(), nil
	}
	return raw, info.ModTime(), nil
}

func cloneSettings(settings Settings) Settings {
	settings.Keybindings = maps.Clone(settings.Keybindings)
	for action, keys := range settings.Keybindings {
		settings.Keybindings[action] = slices.Clone(keys)
	}
	if settings.Repositories.DevFolderPath != nil {
		path := *settings.Repositories.DevFolderPath
		settings.Repositories.DevFolderPath = &path
	}
	return settings
}

func notifySettings(listeners []func(prev, next Settings), prev, next Settings) {
	if reflect.DeepEqual(prev, next) {
		return
	}
	for _, listener := range listeners {
		listener(cloneSettings(prev), cloneSettings(next))
	}
}
