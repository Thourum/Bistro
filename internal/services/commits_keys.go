package services

import (
	"fmt"

	"github.com/itterno/bistro/internal/git/diff"
)

// commitViewKey identifies a commit's viewed mark; see the spec's Viewed state.
type commitViewKey struct {
	Key         string
	Occurrences int
}

// commitViewKeys returns keys aligned with commits; non-merge fingerprints
// come from patchIDs (cached or freshly computed stats).
func commitViewKeys(commits []diff.RangeCommit, patchIDs map[string]string) []commitViewKey {
	totals := map[string]int{}
	for _, commit := range commits {
		if !commit.IsMerge {
			totals[patchIDs[commit.SHA]]++
		}
	}
	seen := map[string]int{}
	keys := make([]commitViewKey, len(commits))
	for i, commit := range commits {
		if commit.IsMerge {
			keys[i] = commitViewKey{Key: "merge:" + commit.SHA, Occurrences: 1}
			continue
		}
		id := patchIDs[commit.SHA]
		keys[i] = commitViewKey{Key: fmt.Sprintf("%s:%d", id, seen[id]), Occurrences: totals[id]}
		seen[id]++
	}
	return keys
}
