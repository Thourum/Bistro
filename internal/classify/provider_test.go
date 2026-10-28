package classify

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const providerTestRule = `name: TestVendor
vendor: testvendor
match:
  author: ["test[bot]"]
classifiers:
  - name: always_nit
    body_pattern: 'nit'
    severity: nit
`

func writeRule(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write rule: %v", err)
	}
}

func TestProviderCurrentLoadsDir(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "a.yml", providerTestRule)
	p := NewEngineProvider(dir, func() string { return "rules" }, nil)

	eng := p.Current()

	got := eng.Classify("test[bot]", "nit: rename this", "")
	if got.Severity != "nit" || got.BotVendor != "testvendor" {
		t.Errorf("dir rules not active: %+v", got)
	}
}

func TestProviderReflectsEditOnNextCall(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "a.yml", providerTestRule)
	p := NewEngineProvider(dir, func() string { return "rules" }, nil)
	first := p.Current()

	writeRule(t, dir, "a.yml", strings.Replace(providerTestRule, "severity: nit", "severity: warning", 1))
	second := p.Current()

	if first.Fingerprint() == second.Fingerprint() {
		t.Fatalf("fingerprint unchanged after edit")
	}
	if got := second.Classify("test[bot]", "nit: rename this", ""); got.Severity != "warning" {
		t.Errorf("edit not picked up: %+v", got)
	}
}

func TestProviderCachesWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "a.yml", providerTestRule)
	p := NewEngineProvider(dir, func() string { return "rules" }, nil)

	first := p.Current()
	second := p.Current()
	if first != second {
		t.Errorf("unchanged dir should return the cached *Engine instance")
	}
}

func TestProviderFallsBackToBuiltins(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{"empty dir path", ""},
		{"nonexistent dir", filepath.Join(t.TempDir(), "missing")},
		{"dir with no yaml", t.TempDir()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewEngineProvider(tt.dir, func() string { return "rules" }, nil)

			eng := p.Current()

			if eng.Fingerprint() != "builtin:rules" {
				t.Errorf("fingerprint = %q, want builtin:rules", eng.Fingerprint())
			}
			// Built-in coderabbit rules must be active.
			got := eng.Classify("coderabbitai[bot]", "💡 tidy this up", "")
			if got.Severity != "nit" {
				t.Errorf("builtins not active: %+v", got)
			}
		})
	}
}

func TestProviderModeChangeInvalidates(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "a.yml", providerTestRule)
	mode := "rules"
	var mu sync.Mutex
	getMode := func() string { mu.Lock(); defer mu.Unlock(); return mode }
	p := NewEngineProvider(dir, getMode, nil)
	first := p.Current()

	mu.Lock()
	mode = "rules+llm"
	mu.Unlock()
	second := p.Current()

	if first.Fingerprint() == second.Fingerprint() {
		t.Errorf("mode change must produce a new fingerprint")
	}
}

func TestProviderConcurrentCurrent(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "a.yml", providerTestRule)
	p := NewEngineProvider(dir, func() string { return "rules" }, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eng := p.Current()
			_ = eng.Classify("test[bot]", "nit: x", "")
		}()
	}
	wg.Wait()
}
