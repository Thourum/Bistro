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

// commitViewKeys returns keys aligned with commits. Missing diffs must be merges:
// callers request WithDiffs so ordinary commits always have a fingerprint.
func commitViewKeys(commits []diff.RangeCommit) []commitViewKey {
	totals := map[string]int{}
	for _, commit := range commits {
		if commit.Diff != nil {
			totals[commit.Diff.PatchID]++
		}
	}
	seen := map[string]int{}
	keys := make([]commitViewKey, len(commits))
	for i, commit := range commits {
		if commit.Diff == nil {
			keys[i] = commitViewKey{Key: "merge:" + commit.SHA, Occurrences: 1}
			continue
		}
		id := commit.Diff.PatchID
		keys[i] = commitViewKey{Key: fmt.Sprintf("%s:%d", id, seen[id]), Occurrences: totals[id]}
		seen[id]++
	}
	return keys
}
