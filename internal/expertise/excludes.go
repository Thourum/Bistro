package expertise

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

var builtInExcludeGlobs = []string{
	"vendor/**",
	"**/vendor/**",
	"dist/**",
	"**/dist/**",
	"node_modules/**",
	"**/node_modules/**",
	"**/*.pb.go",
	"testdata/**",
	"**/testdata/**",
	"*.lock",
	"**/*.lock",
	"pnpm-lock.yaml",
	"**/pnpm-lock.yaml",
	"package-lock.json",
	"**/package-lock.json",
	"go.sum",
	"**/go.sum",
	"yarn.lock",
	"**/yarn.lock",
}

// Excluder matches repository-relative paths against built-in and user globs.
type Excluder struct {
	globs []string
}

// New combines built-in exclusions with user-defined globs.
func New(userGlobs []string) Excluder {
	globs := make([]string, 0, len(builtInExcludeGlobs)+len(userGlobs))
	globs = append(globs, builtInExcludeGlobs...)
	globs = append(globs, userGlobs...)

	return Excluder{globs: globs}
}

func loadExcluder(ctx context.Context, store *Store, repoID string) (Excluder, error) {
	var userGlobs []string
	if err := store.Setting(ctx, excludesSettingKey(repoID), &userGlobs); err != nil && !errors.Is(err, ErrSettingNotFound) {
		return Excluder{}, fmt.Errorf("load expertise excludes for repo %s: %w", repoID, err)
	}
	return New(userGlobs), nil
}

// Excluded reports whether any configured glob matches filePath.
func (excluder Excluder) Excluded(filePath string) bool {
	normalizedPath := filepath.ToSlash(filePath)
	for _, glob := range excluder.globs {
		matched, err := doublestar.Match(filepath.ToSlash(glob), normalizedPath)
		if err == nil && matched {
			return true
		}
	}

	return false
}
