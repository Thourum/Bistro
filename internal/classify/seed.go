package classify

import (
	"embed"
	"errors"
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
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat rules dir: %w", err)
		}
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
		empty, err := isDirEmptyOfYAML(dir)
		if err != nil {
			return fmt.Errorf("inspect rules dir: %w", err)
		}
		if !empty {
			shipped = allNames
		}
	}

	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dst); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("stat rule %s: %w", e.Name(), err)
			}
			if slices.Contains(shipped, e.Name()) {
				continue // previously shipped, now absent: user deleted it
			}
			// Never shipped here before — a new builtin: fall through and seed.
		}
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return err
		}
		if err := writeSeedFile(dst, data); err != nil {
			return fmt.Errorf("seed %s: %w", e.Name(), err)
		}
	}

	manifest := strings.Join(allNames, "\n") + "\n"
	if err := writeSeedFile(filepath.Join(dir, shippedManifestName), []byte(manifest)); err != nil {
		return fmt.Errorf("write shipped manifest: %w", err)
	}
	return nil
}

// writeSeedFile replaces a rule or manifest only after its complete contents
// have been written, so a failed write cannot truncate the previous file.
func writeSeedFile(path string, data []byte) (writeErr error) {
	// Preserve links to user-managed rule files when replacing their contents.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve seed destination: %w", err)
	} else if _, linkErr := os.Lstat(path); linkErr == nil {
		return fmt.Errorf("resolve seed destination: %w", err)
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat seed destination: %w", err)
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".bistro-seed-*")
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			writeErr = errors.Join(writeErr, fmt.Errorf("close seed temp file: %w", err))
		}
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeErr = errors.Join(writeErr, fmt.Errorf("remove seed temp file: %w", err))
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
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
