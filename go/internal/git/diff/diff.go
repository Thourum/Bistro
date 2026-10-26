package diff

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
)

// SingleFilePatch wraps a FilePatch to implement the fdiff.Patch interface.
type SingleFilePatch struct {
	FilePatch fdiff.FilePatch
}

// FilePatches returns the wrapped file patch.
func (s *SingleFilePatch) FilePatches() []fdiff.FilePatch { return []fdiff.FilePatch{s.FilePatch} }

// Message returns an empty message for the per-file patch.
func (s *SingleFilePatch) Message() string { return "" }

// filePatchToString renders a single FilePatch as a unified diff string
func filePatchToString(fp fdiff.FilePatch) (string, error) {
	buf := bytes.NewBuffer(nil)
	enc := fdiff.NewUnifiedEncoder(buf, fdiff.DefaultContextLines)
	if err := enc.Encode(&SingleFilePatch{FilePatch: fp}); err != nil {
		return "", fmt.Errorf("failed to encode file patch: %w", err)
	}
	return buf.String(), nil
}

// Generate creates per-file diffs between two commits in the on-disk
// repository at localPath.
func Generate(localPath, baseSHA, headSHA string, opts Options) (*Result, error) {
	if localPath == "" || baseSHA == "" || headSHA == "" {
		return nil, errors.New("localPath, baseSHA, and headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	return generateDiffFromRepo(repo, baseSHA, headSHA, opts)
}

// generateDiffFromRepo computes per-file diffs using an already-open repository.
func generateDiffFromRepo(repo *gitlib.Repository, baseSHA, headSHA string, opts Options) (*Result, error) {
	totalStart := time.Now()
	if repo == nil {
		return nil, errors.New("repository cannot be nil")
	}
	if baseSHA == "" || headSHA == "" {
		return nil, errors.New("baseSHA and headSHA cannot be empty")
	}

	patch, profile, err := loadDiffPatch(repo, baseSHA, headSHA)
	if err != nil {
		return nil, err
	}
	renderStart := time.Now()
	result, err := renderDiffFiles(patch, opts)
	if err != nil {
		return nil, err
	}
	result.Profile.LoadCommits = profile.LoadCommits
	result.Profile.LoadTrees = profile.LoadTrees
	result.Profile.BuildPatch = profile.BuildPatch
	result.Profile.RenderFilePatches = time.Since(renderStart)
	result.Profile.FileCount = len(result.Files)
	result.Profile.PatchBytes = result.Size
	result.Profile.TotalDuration = time.Since(totalStart)
	return result, nil
}

func loadDiffPatch(repo *gitlib.Repository, baseSHA, headSHA string) (fdiff.Patch, Profile, error) {
	commitStart := time.Now()
	baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return nil, Profile{}, fmt.Errorf("failed to get base commit: %w", err)
	}
	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, Profile{}, fmt.Errorf("failed to get head commit: %w", err)
	}
	commitDuration := time.Since(commitStart)

	treeStart := time.Now()
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, Profile{}, fmt.Errorf("failed to get base tree: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, Profile{}, fmt.Errorf("failed to get head tree: %w", err)
	}
	treeDuration := time.Since(treeStart)

	patchStart := time.Now()
	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return nil, Profile{}, fmt.Errorf("failed to create patch: %w", err)
	}
	return patch, Profile{
		LoadCommits: commitDuration,
		LoadTrees:   treeDuration,
		BuildPatch:  time.Since(patchStart),
	}, nil
}

func renderDiffFiles(patch fdiff.Patch, opts Options) (*Result, error) {
	result := &Result{Files: make([]*File, 0)}
	for _, filePatch := range patch.FilePatches() {
		diffFile, err := renderDiffFile(filePatch, opts)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, diffFile)
		result.Size += int64(len(diffFile.Patch))
		switch {
		case diffFile.Binary:
			result.Profile.BinaryFileCount++
		case diffFile.DiffStatus == StatusTruncated:
			result.Profile.TruncatedFiles++
		}
	}
	return result, nil
}

func renderDiffFile(filePatch fdiff.FilePatch, opts Options) (*File, error) {
	from, to := filePatch.Files()
	var path, oldPath, status string
	switch {
	case from == nil:
		path, status = to.Path(), "added"
	case to == nil:
		path, oldPath, status = from.Path(), from.Path(), "deleted"
	case from.Path() != to.Path():
		path, oldPath, status = to.Path(), from.Path(), "renamed"
	default:
		path, status = to.Path(), "modified"
	}
	isBinary := filePatch.IsBinary()
	if isBinary {
		status = "binary"
	}
	patchStr, err := filePatchToString(filePatch)
	if err != nil {
		return nil, fmt.Errorf("failed to encode patch for %s: %w", path, err)
	}
	patchID := PatchID(patchStr)
	if isBinary {
		// Binary patches render as "Binary files differ"; the blob IDs are
		// what identify the content.
		patchID = PatchID(fmt.Sprintf("binary %s %s\n", fileHash(from), fileHash(to)))
	}
	patchBytes := len(patchStr)
	diffStatus := StatusOK
	patchToStore := patchStr
	switch {
	case isBinary:
		patchToStore = ""
	case opts.MaxFilePatchBytes > 0 && int64(patchBytes) > opts.MaxFilePatchBytes:
		diffStatus, patchToStore = StatusTruncated, ""
	}

	// Count from the full patch so truncated/binary files retain line stats.
	added, removed := countLines(patchStr)
	return &File{
		Path:         path,
		Status:       status,
		DiffStatus:   diffStatus,
		OldPath:      oldPath,
		Binary:       isBinary,
		Truncated:    diffStatus == StatusTruncated,
		PatchBytes:   patchBytes,
		HunkCount:    countHunks(patchStr),
		Patch:        patchToStore,
		PatchID:      patchID,
		AddedLines:   added,
		RemovedLines: removed,
	}, nil
}

// fileHash returns the blob ID of one side of a file patch; "" when the side
// does not exist (added or deleted file).
func fileHash(file fdiff.File) string {
	if file == nil {
		return ""
	}
	return file.Hash().String()
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
