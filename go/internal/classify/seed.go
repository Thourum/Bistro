package classify

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed builtin/*.yml
var builtinFS embed.FS

// SeedAndUpdate materializes embedded rule files into dir.
// Fresh dir: write everything. Existing dir: overwrite only built-ins that
// are still PRESENT (upgrade path) — a deleted built-in stays deleted, and
// user-created files are never touched.
func SeedAndUpdate(dir string) error {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return fmt.Errorf("read embedded builtins: %w", err)
	}
	fresh := false
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create rules dir: %w", err)
		}
		fresh = true
	}
	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if !fresh {
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				continue // deleted by the user — respect it
			}
		}
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("seed %s: %w", e.Name(), err)
		}
	}
	return nil
}

// Builtins parses the embedded rule files directly — the in-memory fallback
// when the rules dir is unusable. Embedded files are compile-time trusted;
// a parse error here is a programmer bug surfaced in tests.
func Builtins() ([]*RuleFile, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	var out []*RuleFile
	for _, e := range entries { // ReadDir is already sorted by name
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}
		rf, err := Parse(data, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, rf)
	}
	return out, nil
}
