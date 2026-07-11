package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
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

// GenerateDiff creates per-file diffs between two commits in the on-disk
// repository at localPath. Retained as the stable, disk-backed API; it opens
// the repo and delegates to generateDiffFromRepo so the patch-conversion logic
// is shared with the in-memory path.
func (c *Client) GenerateDiff(localPath, baseSHA, headSHA string, opts DiffOptions) (*DiffResult, error) {
	if localPath == "" || baseSHA == "" || headSHA == "" {
		return nil, fmt.Errorf("localPath, baseSHA, and headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	return generateDiffFromRepo(repo, baseSHA, headSHA, opts)
}

// generateDiffFromRepo computes per-file diffs between baseSHA and headSHA using
// an already-open *gitlib.Repository. It works identically against a disk-backed
// repo (PlainOpen) and an in-memory repo (memory.Storage), since both expose the
// same object store. This is the shared core behind GenerateDiff (disk) and the
// in-memory PR-specific diff prototype.
func generateDiffFromRepo(repo *gitlib.Repository, baseSHA, headSHA string, opts DiffOptions) (*DiffResult, error) {
	totalStart := time.Now()

	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}
	if baseSHA == "" || headSHA == "" {
		return nil, fmt.Errorf("baseSHA and headSHA cannot be empty")
	}

	baseHash := plumbing.NewHash(baseSHA)
	headHash := plumbing.NewHash(headSHA)

	commitStart := time.Now()
	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get base commit: %w", err)
	}

	headCommit, err := repo.CommitObject(headHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get head commit: %w", err)
	}
	commitDuration := time.Since(commitStart)

	treeStart := time.Now()
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get base tree: %w", err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get head tree: %w", err)
	}
	treeDuration := time.Since(treeStart)

	patchStart := time.Now()
	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return nil, fmt.Errorf("failed to create patch: %w", err)
	}
	patchDuration := time.Since(patchStart)

	result := &DiffResult{
		Files: make([]*DiffFile, 0),
		Size:  0,
		Profile: DiffProfile{
			LoadCommits: commitDuration,
			LoadTrees:   treeDuration,
			BuildPatch:  patchDuration,
		},
	}

	renderStart := time.Now()
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()

		var path, oldPath, status string
		if from == nil {
			path = to.Path()
			status = "added"
		} else if to == nil {
			path = from.Path()
			oldPath = from.Path()
			status = "deleted"
		} else if from.Path() != to.Path() {
			path = to.Path()
			oldPath = from.Path()
			status = "renamed"
		} else {
			path = to.Path()
			status = "modified"
		}
		isBinary := filePatch.IsBinary()
		if isBinary {
			status = "binary"
			result.Profile.BinaryFileCount++
		}

		patchStr, err := filePatchToString(filePatch)
		if err != nil {
			return nil, fmt.Errorf("failed to encode patch for %s: %w", path, err)
		}

		patchBytes := len(patchStr)
		hunks := countHunks(patchStr)

		diffStatus := DiffStatusOK
		patchToStore := patchStr
		if isBinary {
			patchToStore = ""
		} else if opts.MaxFilePatchBytes > 0 && int64(patchBytes) > opts.MaxFilePatchBytes {
			diffStatus = DiffStatusTruncated
			patchToStore = ""
			result.Profile.TruncatedFiles++
		} else if opts.MaxTotalPatchBytes > 0 && result.Size+int64(patchBytes) > opts.MaxTotalPatchBytes {
			diffStatus = DiffStatusTooLarge
			patchToStore = ""
			result.Profile.TooLargeFiles++
		}

		// Count from the full patch so truncated/binary files still report
		// accurate line stats even though patchToStore is cleared.
		added, removed := countLines(patchStr)

		diffFile := &DiffFile{
			Path:         path,
			Status:       status,
			DiffStatus:   diffStatus,
			OldPath:      oldPath,
			Binary:       isBinary,
			Truncated:    diffStatus == DiffStatusTruncated || diffStatus == DiffStatusTooLarge,
			PatchBytes:   patchBytes,
			HunkCount:    hunks,
			Patch:        patchToStore,
			AddedLines:   added,
			RemovedLines: removed,
		}

		result.Files = append(result.Files, diffFile)
		result.Size += int64(len(patchToStore))
	}
	result.Profile.RenderFilePatches = time.Since(renderStart)
	result.Profile.FileCount = len(result.Files)
	result.Profile.PatchBytes = result.Size
	result.Profile.TotalDuration = time.Since(totalStart)

	return result, nil
}

// countHunks counts unified diff hunk headers in a patch string.
func countHunks(patch string) (hunks int) {
	scanner := bufio.NewScanner(strings.NewReader(patch))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "@@") {
			hunks++
		}
	}
	return hunks
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
		relPath := filepath.Clean(filepath.FromSlash(file.Path))
		if relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
			return fmt.Errorf("invalid diff path %q", file.Path)
		}
		patchFile := filepath.Join(patchDir, relPath+".patch")
		if err := os.MkdirAll(filepath.Dir(patchFile), 0755); err != nil {
			return fmt.Errorf("failed to create patch subdirectory: %w", err)
		}

		if err := os.WriteFile(patchFile, []byte(file.Patch), 0644); err != nil {
			return fmt.Errorf("failed to write patch file %s: %w", patchFile, err)
		}
	}

	return nil
}
