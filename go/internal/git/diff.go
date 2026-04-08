package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitlib "github.com/go-git/go-git/v5"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing"
)

// singleFilePatch wraps a FilePatch to implement the fdiff.Patch interface
type singleFilePatch struct {
	fp fdiff.FilePatch
}

func (s *singleFilePatch) FilePatches() []fdiff.FilePatch { return []fdiff.FilePatch{s.fp} }
func (s *singleFilePatch) Message() string                { return "" }

// filePatchToString renders a single FilePatch as a unified diff string
func filePatchToString(fp fdiff.FilePatch) (string, error) {
	buf := bytes.NewBuffer(nil)
	enc := fdiff.NewUnifiedEncoder(buf, fdiff.DefaultContextLines)
	if err := enc.Encode(&singleFilePatch{fp: fp}); err != nil {
		return "", fmt.Errorf("failed to encode file patch: %w", err)
	}
	return buf.String(), nil
}

// GenerateDiff creates per-file diffs between two commits
func (c *Client) GenerateDiff(localPath, baseSHA, headSHA string) (*DiffResult, error) {
	if localPath == "" || baseSHA == "" || headSHA == "" {
		return nil, fmt.Errorf("localPath, baseSHA, and headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	baseHash := plumbing.NewHash(baseSHA)
	headHash := plumbing.NewHash(headSHA)

	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get base commit: %w", err)
	}

	headCommit, err := repo.CommitObject(headHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get head commit: %w", err)
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get base tree: %w", err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get head tree: %w", err)
	}

	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return nil, fmt.Errorf("failed to create patch: %w", err)
	}

	result := &DiffResult{
		Files: make([]*DiffFile, 0),
		Size:  0,
	}

	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()

		var path, status string
		if from == nil {
			path = to.Path()
			status = "added"
		} else if to == nil {
			path = from.Path()
			status = "deleted"
		} else {
			path = to.Path()
			status = "modified"
		}

		patchStr, err := filePatchToString(filePatch)
		if err != nil {
			return nil, fmt.Errorf("failed to encode patch for %s: %w", path, err)
		}

		added, removed := countLines(patchStr)

		diffFile := &DiffFile{
			Path:         path,
			Status:       status,
			Patch:        patchStr,
			AddedLines:   added,
			RemovedLines: removed,
		}

		result.Files = append(result.Files, diffFile)
		result.Size += int64(len(patchStr))
	}

	return result, nil
}

// countLines counts added/removed lines in a patch string
func countLines(patch string) (added, removed int) {
	scanner := bufio.NewScanner(strings.NewReader(patch))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}

// WritePatchFiles writes per-file diffs to disk for offline viewing
func (c *Client) WritePatchFiles(diffResult *DiffResult, baseSHA, headSHA string) error {
	if diffResult == nil {
		return fmt.Errorf("diff result cannot be nil")
	}

	baseDir, err := getPatchDir()
	if err != nil {
		return fmt.Errorf("failed to get patch dir: %w", err)
	}
	patchDir := filepath.Join(baseDir, baseSHA+"_"+headSHA)
	if err := os.MkdirAll(patchDir, 0755); err != nil {
		return fmt.Errorf("failed to create patch directory: %w", err)
	}

	for _, file := range diffResult.Files {
		safePath := strings.ReplaceAll(file.Path, "/", "_")
		safePath = strings.ReplaceAll(safePath, "..", "_")
		patchFile := filepath.Clean(filepath.Join(patchDir, safePath+".patch"))

		// Verify the resolved path stays inside patchDir to prevent traversal
		if !strings.HasPrefix(patchFile, filepath.Clean(patchDir)+string(os.PathSeparator)) {
			return fmt.Errorf("path traversal detected for file: %s", file.Path)
		}

		if err := os.WriteFile(patchFile, []byte(file.Patch), 0644); err != nil {
			return fmt.Errorf("failed to write patch file %s: %w", patchFile, err)
		}
	}

	return nil
}
