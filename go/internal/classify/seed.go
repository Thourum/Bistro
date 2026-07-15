package classify

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed builtin/*.yml
var builtinFS embed.FS

// shippedManifestName records which built-ins were ever seeded into a rules
// dir. It is what distinguishes "the user deleted coderabbit.yml" (name in
// manifest, file absent → respect the deletion) from "this app upgrade ships
// a brand-new vendor file" (name NOT in manifest → seed it).
const shippedManifestName = ".seeded-builtins"

func readShippedManifest(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, shippedManifestName))
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// SeedAndUpdate materializes embedded rule files into dir.
// Fresh dir: write everything. Existing dir: overwrite built-ins still
// PRESENT (upgrade path), seed built-ins never shipped before (new vendors),
// respect user deletions of previously shipped ones, and never touch
// user-created files. The shipped set is tracked in .seeded-builtins.
func SeedAndUpdate(dir string) error {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return fmt.Errorf("read embedded builtins: %w", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create rules dir: %w", err)
		}
	}
	// Pre-manifest installs (or a hand-deleted manifest): treat every current
	// builtin name as previously shipped, preserving the old presence-gated
	// behavior rather than resurrecting files the user may have deleted.
	shipped := readShippedManifest(dir)
	manifestExisted := shipped != nil
	var allNames []string
	for _, e := range entries {
		allNames = append(allNames, e.Name())
	}
	if !manifestExisted {
		if empty, _ := isDirEmptyOfYAML(dir); !empty {
			shipped = allNames
		}
	}

	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if slices.Contains(shipped, e.Name()) {
				continue // previously shipped, now absent: user deleted it
			}
			// Never shipped here before — a new builtin: fall through and seed.
		}
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("seed %s: %w", e.Name(), err)
		}
	}

	manifest := strings.Join(allNames, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, shippedManifestName), []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("write shipped manifest: %w", err)
	}
	return nil
}

// isDirEmptyOfYAML reports whether dir contains no rule files yet.
func isDirEmptyOfYAML(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, err
	}
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yml" || ext == ".yaml" {
			return false, nil
		}
	}
	return true, nil
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
