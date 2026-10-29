package shell

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const watchPoll = 10 * time.Millisecond
const testDeadline = 3 * time.Second

type settingsChange struct{ prev, next Settings }

func testStore(t *testing.T) *SettingsStore {
	t.Helper()
	store := NewSettingsStore(filepath.Join(t.TempDir(), "nested", "settings.json"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	return store
}

func assertSettings(t *testing.T, got, want Settings) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("settings mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestReloadAndSetNotifyInCommitOrder(t *testing.T) {
	store := testStore(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	changes := make(chan bool, 2)
	store.OnChange(func(_, next Settings) {
		_ = store.Get() // Consent getters must remain safe inside a listener.
		if next.Privacy.TelemetryEnabled {
			close(entered)
			<-release
		}
		changes <- next.Privacy.TelemetryEnabled
	})
	writeExternal(t, store.Path(), `{"privacy":{"telemetryEnabled":true}}`)
	reloaded := make(chan struct{})
	go func() { store.reloadExternal(); close(reloaded) }()
	select {
	case <-entered:
	case <-time.After(testDeadline):
		t.Fatal("reload listener did not start")
	}
	written := make(chan error, 1)
	next := store.Get()
	next.Privacy.TelemetryEnabled = false
	go func() { _, err := store.Set(next); written <- err }()
	// Allow the overlapping writer to reach the blocked notification.
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(watchPoll):
	}
	close(release)
	select {
	case <-reloaded:
	case <-time.After(testDeadline):
		t.Fatal("reload did not finish")
	}
	for _, want := range []bool{true, false} {
		select {
		case got := <-changes:
			if got != want {
				t.Fatalf("notification = %v, want %v", got, want)
			}
		case <-time.After(testDeadline):
			t.Fatal("missing settings notification")
		}
	}
	if store.Get().Privacy.TelemetryEnabled {
		t.Fatal("final consent should be off")
	}
}

func TestLoadDefaultsAndValidSiblings(t *testing.T) {
	for _, document := range []string{"", "null", "[]", "{not json", `{"appearance":{"theme":"purple","uiFontSize":16},"bogus":{}}`} {
		t.Run(document, func(t *testing.T) {
			store := testStore(t)
			want := DefaultSettings()
			if document != "" {
				writeExternal(t, store.Path(), document)
				if err := store.Load(); err != nil {
					t.Fatal(err)
				}
			}
			if strings.Contains(document, "uiFontSize") {
				want.Appearance.UIFontSize = 16
			}
			assertSettings(t, store.Get(), want)
		})
	}
}

func TestSetNormalisesAndNotifies(t *testing.T) {
	store := testStore(t)
	prev := store.Get()
	var changes []settingsChange
	store.OnChange(func(prev, next Settings) {
		// Callbacks must be able to read the store without deadlocking.
		assertSettings(t, store.Get(), next)
		changes = append(changes, settingsChange{prev, next})
	})
	next := store.Get()
	next.Navigation.Style = "nano"
	next.Appearance.Theme = "dark"
	next.Appearance.UIFontSize = math.NaN()
	next.Keybindings = map[string][]string{"list.top": {"g", "g"}, "list.down": {}}
	got, err := store.Set(next)
	if err != nil {
		t.Fatal(err)
	}
	want := prev
	want.Appearance.Theme = "dark"
	want.Keybindings = next.Keybindings
	assertSettings(t, got, want)
	if len(changes) != 1 {
		t.Fatalf("got %d notifications", len(changes))
	}
	assertSettings(t, changes[0].prev, prev)
	assertSettings(t, changes[0].next, want)
	if _, err := store.Set(got); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatal("unchanged Set notified")
	}
	encoded, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(encoded), "\n") || !strings.Contains(string(encoded), "\n  \"appearance\"") {
		t.Fatal("settings not pretty-printed with trailing newline")
	}
	reloaded := NewSettingsStore(store.Path(), nil)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	assertSettings(t, reloaded.Get(), want)
	reset, err := store.Reset()
	if err != nil {
		t.Fatal(err)
	}
	assertSettings(t, reset, DefaultSettings())
	if len(changes) != 2 {
		t.Fatal("Reset did not notify once")
	}
}

func TestWindowIsShellOwned(t *testing.T) {
	store := testStore(t)
	count := 0
	store.OnChange(func(Settings, Settings) { count++ })
	bounds := WindowBounds{X: 5, Y: 6, Width: 800, Height: 600, Maximised: true}
	if err := store.SetWindow(bounds); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("SetWindow notified")
	}
	next := store.Get()
	next.Window = WindowBounds{}
	next.Appearance.Theme = "dark"
	if _, err := store.Set(next); err != nil {
		t.Fatal(err)
	}
	if store.Get().Window != bounds {
		t.Fatal("renderer overwrote window bounds")
	}
	if err := store.SetWindow(WindowBounds{Width: 399}); err == nil {
		t.Fatal("invalid bounds accepted")
	}
	if store.Get().Window != bounds {
		t.Fatal("invalid bounds changed state")
	}
	reloaded := NewSettingsStore(store.Path(), nil)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if reloaded.Get().Window != bounds {
		t.Fatal("bounds not persisted")
	}
}

func TestWatchExternalEditsKeepsCurrentOnInvalidInput(t *testing.T) {
	store := testStore(t)
	next := store.Get()
	next.Appearance.Theme = "dark"
	if _, err := store.Set(next); err != nil {
		t.Fatal(err)
	}
	changes := make(chan settingsChange, 8)
	store.OnChange(func(prev, next Settings) { changes <- settingsChange{prev, next} })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); store.Watch(ctx, watchPoll) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(testDeadline):
			t.Error("Watch did not stop")
		}
	})
	writeExternal(t, store.Path(), `{"appearance":{"uiFontSize":16}}`)
	select {
	case change := <-changes:
		if change.prev.Appearance.UIFontSize != 14 || change.next.Appearance.UIFontSize != 16 {
			t.Fatal("wrong prev/next")
		}
	case <-time.After(testDeadline):
		t.Fatal("Watch did not notify")
	}
	if store.Get().Appearance.UIFontSize != 16 {
		t.Fatal("external value not applied")
	}
	writeExternal(t, store.Path(), "{not json")
	waitForObservedEdit(t, store)
	select {
	case <-changes:
		t.Fatal("invalid JSON notified")
	default:
	}
	if store.Get().Appearance.Theme != "dark" {
		t.Fatal("invalid JSON reset theme")
	}
	writeExternal(t, store.Path(), `{"appearance":{"theme":"purple"}}`)
	waitForObservedEdit(t, store)
	select {
	case <-changes:
		t.Fatal("invalid-only edit notified")
	default:
	}
	if store.Get().Appearance.Theme != "dark" {
		t.Fatal("invalid theme reset to default")
	}
	writeExternal(t, store.Path(), `{"appearance":{"theme":"purple","codeFontSize":15}}`)
	select {
	case change := <-changes:
		if change.next.Appearance.Theme != "dark" || change.next.Appearance.CodeFontSize != 15 {
			t.Fatal("invalid leaf did not preserve current value")
		}
	case <-time.After(testDeadline):
		t.Fatal("valid sibling not applied")
	}
	// Store-originated writes must not echo through Watch.
	if err := store.SetWindow(WindowBounds{X: 5}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changes:
		t.Fatal("own window write notified")
	case <-time.After(5 * watchPoll):
	}
}

func writeExternal(t *testing.T, path, document string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForObservedEdit(t *testing.T, store *SettingsStore) {
	t.Helper()
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(testDeadline)
	defer deadline.Stop()
	ticker := time.NewTicker(watchPoll)
	defer ticker.Stop()
	for {
		store.mu.Lock()
		observed := store.mtime.Equal(info.ModTime())
		store.mu.Unlock()
		if observed {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("external edit was not observed")
		}
	}
}

func TestLeafValidation(t *testing.T) {
	store := testStore(t)
	base := store.Get()
	base.Appearance.Theme = "dark"
	base.Keybindings = map[string][]string{"list.down": {"j"}}
	folder := "/existing"
	base.Repositories.DevFolderPath = &folder
	base.Window = WindowBounds{X: 5, Width: 800, Height: 600}
	for _, document := range []string{
		`{"appearance":{"theme":null,"uiFont":null,"codeFont":2,"uiFontSize":0,"codeFontSize":-1,"highDensity":null,"onboardingComplete":"yes","tourComplete":1}}`,
		`{"sync":{"autoSync":null,"syncInterval":1e999,"notifications":1},"prList":{"view":"other"},"diff":{"style":"other"},"privacy":{"telemetryEnabled":null}}`,
		`{"classification":{"mode":"other","dismissalScope":"other","provider":"other","model":null,"endpoint":1},"navigation":{"style":"nano"},"repositories":{"devFolderPath":1}}`,
		`{"keybindings":{"list.down":["j",null]}}`,
		`{"keybindings":{"list.down":null}}`,
		`{"keybindings":null}`, `{"window":{"width":399}}`, `{"window":null}`, `{"window":{"maximised":null}}`,
	} {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(document), &raw); err != nil {
			t.Fatal(err)
		}
		assertSettings(t, normalise(raw, base), base)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"repositories":{"devFolderPath":null},"keybindings":{},"window":{"x":7},"sync":{"syncInterval":0.5}}`), &raw); err != nil {
		t.Fatal(err)
	}
	want := cloneSettings(base)
	want.Repositories.DevFolderPath = nil
	want.Keybindings = map[string][]string{}
	want.Window = WindowBounds{X: 7}
	want.Sync.SyncInterval = 0.5
	assertSettings(t, normalise(raw, base), want)
}

func TestSnapshotsDoNotAliasAndFailedWritePreservesState(t *testing.T) {
	store := testStore(t)
	next := store.Get()
	folder := "/folder"
	next.Repositories.DevFolderPath = &folder
	next.Keybindings["list.down"] = []string{"j"}
	store.OnChange(func(prev, next Settings) { next.Keybindings["list.down"][0] = "callback" })
	result, err := store.Set(next)
	if err != nil {
		t.Fatal(err)
	}
	next.Keybindings["list.down"][0] = "input"
	result.Keybindings["list.down"][0] = "result"
	*result.Repositories.DevFolderPath = "/changed"
	snapshot := store.Get()
	if snapshot.Keybindings["list.down"][0] != "j" || *snapshot.Repositories.DevFolderPath != "/folder" {
		t.Fatal("snapshot aliases state")
	}
	snapshot.Keybindings["list.down"][0] = "get"
	if store.Get().Keybindings["list.down"][0] != "j" {
		t.Fatal("Get aliases state")
	}
	if err := os.Remove(store.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	before := store.Get()
	if _, err := store.Reset(); err == nil {
		t.Fatal("expected persistence error")
	}
	assertSettings(t, store.Get(), before)
}
