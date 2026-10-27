package clonecache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git/remote"
)

// pinnedHeadRef keeps every PR head the sync diffed reachable, so a force-push
// or gc cannot drop the head an earlier patchset recorded.
func pinnedHeadRef(repoURL string, prNumber int, headSHA string) plumbing.ReferenceName {
	return plumbing.ReferenceName(fmt.Sprintf("refs/bistro/%s/pull/%d/heads/%s", refSafeID(repoURL), prNumber, headSHA))
}

// refSafeID namespaces pins by repo. URLs without a GitHub identity (tests,
// BISTRO_REPO_DIR setups) hash the raw URL instead.
func refSafeID(repoURL string) string {
	if identity, err := remote.ParseURL(repoURL); err == nil {
		return remote.SafeRepoID(identity)
	}
	sum := sha256.Sum256([]byte(repoURL))
	return hex.EncodeToString(sum[:])[:remote.SafeIDLength]
}

// PinPRHead writes the pin for headSHA. Best-effort: a missing pin only makes
// a later interdiff unavailable, so it never fails the sync.
func PinPRHead(log *slog.Logger, localPath, repoURL string, prNumber int, headSHA string) {
	if prNumber <= 0 {
		return
	}
	repo, err := gitlib.PlainOpen(localPath)
	if err == nil {
		ref := plumbing.NewHashReference(pinnedHeadRef(repoURL, prNumber, headSHA), plumbing.NewHash(headSHA))
		err = repo.Storer.SetReference(ref)
	}
	if err != nil {
		log.Warn("pin PR head failed", "local_path", localPath, "pr_number", prNumber, "error", err)
	}
}
