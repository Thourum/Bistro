package classify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirAlphabetical(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, dir, "b.yml", validRuleYAML("Rule B", "b_rule", "warning"))
	writeRuleFile(t, dir, "a.yml", validRuleYAML("Rule A", "a_rule", "nit"))

	loaded, errs := LoadDir(dir, "rules")

	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	assertLoadedFileNames(t, loaded, []string{"a.yml", "b.yml"})
}

func TestLoadDirSkipsBadFile(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, dir, "a.yml", validRuleYAML("Rule A", "a_rule", "nit"))
	writeRuleFile(t, dir, "b.yml", "name: [broken")

	loaded, errs := LoadDir(dir, "rules")

	if len(errs) != 1 {
		t.Fatalf("LoadDir error count=%d, want 1: %v", len(errs), errs)
	}
	assertLoadedFileNames(t, loaded, []string{"a.yml"})
}

func TestLoadDirIgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, dir, "README.md", "# Not a rule file\n")
	writeRuleFile(t, dir, "a.yaml", validRuleYAML("Rule A", "a_rule", "nit"))

	loaded, errs := LoadDir(dir, "rules")

	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	assertLoadedFileNames(t, loaded, []string{"a.yaml"})
}

func TestFingerprintChangesOnEdit(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, dir, "a.yml", validRuleYAML("Rule A", "a_rule", "nit"))

	first, errs := LoadDir(dir, "rules")
	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	second, errs := LoadDir(dir, "rules")
	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprint unstable: first=%q second=%q", first.Fingerprint, second.Fingerprint)
	}

	writeRuleFile(t, dir, "a.yml", validRuleYAML("Rule A", "a_rule", "warning"))
	third, errs := LoadDir(dir, "rules")
	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	if first.Fingerprint == third.Fingerprint {
		t.Fatalf("fingerprint did not change after edit: %q", first.Fingerprint)
	}
}

func TestFingerprintIncludesMode(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, dir, "a.yml", validRuleYAML("Rule A", "a_rule", "nit"))

	rulesOnly, errs := LoadDir(dir, "rules")
	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	rulesWithLLM, errs := LoadDir(dir, "rules+llm")
	if len(errs) != 0 {
		t.Fatalf("LoadDir returned errors: %v", errs)
	}
	if rulesOnly.Fingerprint == rulesWithLLM.Fingerprint {
		t.Fatalf("fingerprint did not include mode: %q", rulesOnly.Fingerprint)
	}
}

func writeRuleFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func validRuleYAML(name, classifierName, severity string) string {
	return "name: " + name + "\n" +
		"vendor: test\n" +
		"match:\n" +
		"  author: [\"test[bot]\"]\n" +
		"classifiers:\n" +
		"  - name: " + classifierName + "\n" +
		"    body_pattern: '.*'\n" +
		"    severity: " + severity + "\n"
}

func assertLoadedFileNames(t *testing.T, loaded *Loaded, want []string) {
	t.Helper()
	if loaded == nil {
		t.Fatal("LoadDir returned nil Loaded")
	}
	if len(loaded.Files) != len(want) {
		t.Fatalf("loaded file count=%d, want %d", len(loaded.Files), len(want))
	}
	for index, fileName := range want {
		if loaded.Files[index].FileName != fileName {
			t.Fatalf("loaded file %d=%q, want %q", index, loaded.Files[index].FileName, fileName)
		}
	}
}
