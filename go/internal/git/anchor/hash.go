package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/logging"
)

const contextLines = 3

type anchorHashes struct {
	line    string
	context string
}

// FileLineHash returns (lineContentHash, contextHash) for the 1-based lineNo
// in filePath at commitSHA. Both are hex-encoded SHA-256 values.
func (r *Resolver) FileLineHash(localPath, commitSHA, filePath string, lineNo int) (lineHash, ctxHash string, err error) {
	repo, err := gogit.PlainOpen(localPath)
	if err != nil {
		return "", "", fmt.Errorf("open repo: %w", err)
	}
	return fileLineHash(repo, commitSHA, filePath, lineNo)
}

func fileLineHash(repo *gogit.Repository, commitSHA, filePath string, lineNo int) (lineHash, ctxHash string, err error) {
	commit, err := repo.CommitObject(plumbing.NewHash(commitSHA))
	if err != nil {
		return "", "", fmt.Errorf("commit %s: %w", commitSHA, err)
	}
	file, err := commit.File(filePath)
	if err != nil {
		return "", "", fmt.Errorf("file %s@%s: %w", filePath, logging.ShortSHA(commitSHA), err)
	}
	contents, err := file.Contents()
	if err != nil {
		return "", "", fmt.Errorf("read contents: %w", err)
	}
	lines := splitFileLines(contents)
	if lineNo < 1 || lineNo > len(lines) {
		return "", "", fmt.Errorf("lineNo %d out of range [1, %d]", lineNo, len(lines))
	}

	targetLine := strings.TrimSpace(lines[lineNo-1])
	lineHash = sha256hex(targetLine)

	lo, hi := contextBounds(len(lines), lineNo-1)
	ctxHash = contextHash(lines[lo:hi])
	return lineHash, ctxHash, nil
}

func contextHash(lines []string) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, strings.TrimSpace(line))
	}
	return sha256hex(strings.Join(parts, "\n"))
}

func contextBounds(n, i int) (lo, hi int) {
	lo = i - contextLines
	if lo < 0 {
		lo = 0
	}
	hi = i + contextLines + 1
	if hi > n {
		hi = n
	}
	return lo, hi
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
