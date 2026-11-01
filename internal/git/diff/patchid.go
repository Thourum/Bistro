package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// PatchID fingerprints a per-file patch so a rebase that only moves the change
// keeps the same ID, in the spirit of `git patch-id`. Hunk header ranges and
// the `index` blob-hash line are dropped; hunk boundaries and every changed or
// context line still count.
func PatchID(patch string) string {
	var normalized strings.Builder
	for _, line := range strings.SplitAfter(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "index "):
			continue
		case strings.HasPrefix(line, "@@"):
			normalized.WriteString("@@\n")
		default:
			normalized.WriteString(line)
		}
	}
	sum := sha256.Sum256([]byte(normalized.String()))
	return hex.EncodeToString(sum[:])
}
