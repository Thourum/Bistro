package expertise

import (
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

type Excluder struct {
	globs []string
}

func New(userGlobs []string) Excluder {
	globs := make([]string, 0, len(builtInExcludeGlobs)+len(userGlobs))
	globs = append(globs, builtInExcludeGlobs...)
	globs = append(globs, userGlobs...)

	return Excluder{globs: globs}
}

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
