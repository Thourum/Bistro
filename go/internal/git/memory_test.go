package git

import (
	"testing"

	gogit "github.com/go-git/go-git/v5"
)

// TestVerifyRequiredCommits exercises the required-commit gate for the targeted path.
func TestVerifyRequiredCommits(t *testing.T) {
	remoteDir := t.TempDir()
	_, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, 1)
	repo, err := gogit.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}

	// All present (base+head, plus base again and an empty entry to be tolerated).
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, &CloneOptions{RequiredSHAs: []string{baseSHA, ""}}); missing != "" {
		t.Errorf("expected all present, got missing=%s", missing)
	}
	// Missing required SHA reported.
	const bogus = "2222222222222222222222222222222222222222"
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, &CloneOptions{RequiredSHAs: []string{bogus}}); missing != bogus {
		t.Errorf("expected missing=%s, got %s", bogus, missing)
	}
	// Missing base reported.
	if missing := verifyRequiredCommits(repo, bogus, headSHA, nil); missing != bogus {
		t.Errorf("expected missing base=%s, got %s", bogus, missing)
	}
}
