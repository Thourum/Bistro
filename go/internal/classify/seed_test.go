package classify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var builtinNames = []string{"coderabbit.yml", "greptile.yml", "zz-fallback.yml"}

func TestSeedFreshDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("SeedAndUpdate: %v", err)
	}

	for _, name := range builtinNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("builtin %s not seeded: %v", name, err)
		}
	}
}

func TestSeedRespectsDeletion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	deleted := filepath.Join(dir, "coderabbit.yml")
	if err := os.Remove(deleted); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Errorf("deleted built-in was restored; want it to stay deleted")
	}
}

func TestSeedOverwritesPresent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	target := filepath.Join(dir, "greptile.yml")
	if err := os.WriteFile(target, []byte("corrupted: [broken"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want, err := builtinFS.ReadFile("builtin/greptile.yml")
	if err != nil {
		t.Fatalf("read embedded: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("present built-in not restored to embedded content")
	}
}

func TestSeedNeverTouchesUserFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	userFile := filepath.Join(dir, "my-rules.yml")
	userContent := "name: Mine\nclassifiers:\n  - name: x\n    body_pattern: 'x'\n    severity: nit\n"
	if err := os.WriteFile(userFile, []byte(userContent), 0o644); err != nil {
		t.Fatalf("write user file: %v", err)
	}

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	got, err := os.ReadFile(userFile)
	if err != nil {
		t.Fatalf("read user file: %v", err)
	}
	if string(got) != userContent {
		t.Errorf("user file modified by seeding")
	}
}

func TestSeedUnwritableDirFallsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; permissions not enforced")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	dir := filepath.Join(parent, "rules")

	if err := SeedAndUpdate(dir); err == nil {
		t.Errorf("SeedAndUpdate on unwritable parent: want error, got nil")
	}

	// In-memory fallback still works.
	files, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	if len(files) != len(builtinNames) {
		t.Errorf("Builtins parsed %d files, want %d", len(files), len(builtinNames))
	}
}

func TestBuiltinsParseAndOrder(t *testing.T) {
	files, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d builtins, want 3", len(files))
	}
	// Alphabetical: coderabbit, greptile, zz-fallback (heuristics last).
	if files[0].Vendor != "coderabbit" || files[1].Vendor != "greptile" || files[2].Name != "Heuristics" {
		t.Errorf("unexpected builtin order/content: %s, %s, %s", files[0].FileName, files[1].FileName, files[2].FileName)
	}
}

func TestSeedNewBuiltinArrivesOnUpgrade(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	// Simulate a pre-upgrade install that never shipped greptile.yml: remove
	// the file AND its manifest entry (as if the old binary's embed lacked it).
	if err := os.Remove(filepath.Join(dir, "greptile.yml")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	manifest := filepath.Join(dir, shippedManifestName)
	if err := os.WriteFile(manifest, []byte("coderabbit.yml\nzz-fallback.yml\n"), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("upgrade seed: %v", err)
	}

	// Never-shipped builtin arrives; and it is now recorded as shipped.
	if _, err := os.Stat(filepath.Join(dir, "greptile.yml")); err != nil {
		t.Errorf("new builtin not seeded on upgrade: %v", err)
	}
	data, _ := os.ReadFile(manifest)
	if !strings.Contains(string(data), "greptile.yml") {
		t.Errorf("manifest not updated: %q", data)
	}
}

func TestSeedDeletionStillRespectedWithManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	deleted := filepath.Join(dir, "coderabbit.yml")
	if err := os.Remove(deleted); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Errorf("user-deleted builtin resurrected despite manifest")
	}
}

func TestSeedPreManifestDirRespectsDeletions(t *testing.T) {
	// Legacy install: rule files exist but no manifest. A deleted builtin
	// must NOT be resurrected just because the manifest is missing.
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, shippedManifestName)); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}
	deleted := filepath.Join(dir, "coderabbit.yml")
	if err := os.Remove(deleted); err != nil {
		t.Fatalf("remove builtin: %v", err)
	}

	if err := SeedAndUpdate(dir); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Errorf("pre-manifest deletion resurrected")
	}
}
