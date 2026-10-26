package anchor

import (
	"bytes"
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	dmp "github.com/sergi/go-diff/diffmatchpatch"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/logging"
)

// PositionToLine converts a GitHub unified-diff position value (1-based,
// counting every diff content line across all hunks) to the 1-based file line
// number on the given Side ("LEFT"=old/deleted, "RIGHT"=new/added/context).
//
// The base..head range MUST use DiffBaseSHA..CommitSHA (the actual PR/patchset
// diff range), not parent(CommitSHA)..CommitSHA, to match GitHub's position
// numbering for multi-commit PRs.
func (r *Resolver) PositionToLine(localPath, baseSHA, headSHA, filePath, side string, position int) (int, error) {
	repo, err := gogit.PlainOpen(localPath)
	if err != nil {
		return 0, fmt.Errorf("open repo: %w", err)
	}
	return positionToLine(repo, baseSHA, headSHA, filePath, side, position)
}

func positionToLine(repo *gogit.Repository, baseSHA, headSHA, filePath, side string, position int) (int, error) {
	if position < 1 {
		return 0, fmt.Errorf("position must be >= 1, got %d", position)
	}
	// Root-commit base (zero hash): entire file is one Add chunk; position == line.
	if baseSHA == plumbing.ZeroHash.String() {
		return positionInRootCommit(repo, headSHA, filePath, side, position)
	}

	baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return 0, fmt.Errorf("base commit %s: %w", baseSHA, err)
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return 0, fmt.Errorf("base tree: %w", err)
	}
	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return 0, fmt.Errorf("head commit %s: %w", headSHA, err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return 0, fmt.Errorf("head tree: %w", err)
	}

	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return 0, fmt.Errorf("patch %s..%s: %w", logging.ShortSHA(baseSHA), logging.ShortSHA(headSHA), err)
	}

	if filePatch := patchForPath(patch, filePath); filePatch != nil {
		return positionToLineInPatch(filePatch, side, position)
	}
	return 0, fmt.Errorf("file %q not found in diff %s..%s", filePath, logging.ShortSHA(baseSHA), logging.ShortSHA(headSHA))
}

// positionInRootCommit handles the zero-hash base case (new file, all additions).
// Position == line number for RIGHT side.
func positionInRootCommit(repo *gogit.Repository, commitSHA, filePath, side string, position int) (int, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(commitSHA))
	if err != nil {
		return 0, fmt.Errorf("commit %s: %w", commitSHA, err)
	}
	f, err := commit.File(filePath)
	if err != nil {
		return 0, fmt.Errorf("file %s: %w", filePath, err)
	}
	contents, err := f.Contents()
	if err != nil {
		return 0, fmt.Errorf("contents: %w", err)
	}
	lines := splitFileLines(contents)
	if position < 1 || position > len(lines) {
		return 0, fmt.Errorf("position %d out of range for %d-line file", position, len(lines))
	}
	if side == "LEFT" {
		return 0, fmt.Errorf("root commit: position %d is an addition; LEFT side invalid", position)
	}
	return position, nil
}

// positionToLineInPatch converts a GitHub diff position to a file line number
// by rendering the patch with 3 context lines (matching GitHub's diff format)
// and parsing the unified diff text with hunk-header offsets.
//
// The raw Chunks() API returns the FULL diff (all equal lines), but GitHub's
// position counts lines in the CONTEXT-WINDOWED diff (±3 lines per hunk).
// Rendering and re-parsing is the only correct way to align with GitHub's
// position numbering for hunks that start far below line 1.
func positionToLineInPatch(fp fdiff.FilePatch, side string, target int) (int, error) {
	var buf bytes.Buffer
	enc := fdiff.NewUnifiedEncoder(&buf, fdiff.DefaultContextLines)
	if err := enc.Encode(&diff.SingleFilePatch{FilePatch: fp}); err != nil {
		return 0, fmt.Errorf("render file patch: %w", err)
	}
	return positionFromRenderedDiff(buf.String(), side, target)
}

// positionFromRenderedDiff parses a unified diff string to find the file line
// number for a given GitHub position. Hunk headers (@@ ... @@) are not counted
// as positions; they set the line-number context for subsequent diff lines.
func positionFromRenderedDiff(diffText, side string, target int) (int, error) {
	pos := 0
	oldLine := 0
	newLine := 0

	for _, rawLine := range strings.Split(diffText, "\n") {
		if len(rawLine) == 0 {
			continue
		}

		// Handle file-level header lines (diff, index, ---, +++).
		// These must be checked before the single-char switch to avoid treating
		// "--- a/foo.go" as a deletion line.
		if strings.HasPrefix(rawLine, "--- ") || strings.HasPrefix(rawLine, "+++ ") ||
			strings.HasPrefix(rawLine, "diff ") || strings.HasPrefix(rawLine, "index ") ||
			strings.HasPrefix(rawLine, `\`) {
			continue
		}

		if strings.HasPrefix(rawLine, "@@") {
			// Hunk header: parse start line numbers to initialise oldLine/newLine.
			// Format: "@@ -oldStart[,count] +newStart[,count] @@ [context]"
			o, n, perr := parseHunkHeader(rawLine)
			if perr == nil {
				oldLine = o - 1 // will be incremented before first use
				newLine = n - 1
			}
			continue // hunk headers do not count as positions
		}

		pos++
		switch rawLine[0] {
		case '+':
			newLine++
			if pos == target {
				if side != "RIGHT" {
					return 0, fmt.Errorf("position %d is an addition; LEFT side invalid", target)
				}
				return newLine, nil
			}
		case '-':
			oldLine++
			if pos == target {
				if side != "LEFT" {
					return 0, fmt.Errorf("position %d is a deletion; RIGHT side invalid", target)
				}
				return oldLine, nil
			}
		case ' ':
			oldLine++
			newLine++
			if pos == target {
				if side == "RIGHT" {
					return newLine, nil
				}
				return oldLine, nil
			}
		default:
			// Unexpected line type; don't count it as a position.
			pos--
		}
	}
	return 0, fmt.Errorf("position %d out of range (diff has %d content lines)", target, pos)
}

// parseHunkHeader extracts oldStart and newStart from a unified diff hunk
// header of the form "@@ -oldStart[,oldCount] +newStart[,newCount] @@ ...".
func parseHunkHeader(header string) (oldStart, newStart int, err error) {
	patches, err := dmp.New().PatchFromText(hunkHeaderOnly(header))
	if err != nil {
		return 0, 0, fmt.Errorf("malformed hunk header: %q: %w", header, err)
	}
	if len(patches) != 1 {
		return 0, 0, fmt.Errorf("malformed hunk header: %q", header)
	}
	patch := patches[0]
	return dmpPatchStart(patch.Start1, patch.Length1), dmpPatchStart(patch.Start2, patch.Length2), nil
}

func dmpPatchStart(start, length int) int {
	if length == 0 {
		return start
	}
	return start + 1
}

func hunkHeaderOnly(header string) string {
	if end := strings.LastIndex(header, "@@"); end > 0 {
		return header[:end+2] + "\n"
	}
	return header + "\n"
}

// nearestHunkLine returns the RIGHT-side line number closest to originalLine
// among all added and context lines in the diff baseSHA..headSHA for filePath.
func nearestHunkLine(repo *gogit.Repository, baseSHA, headSHA, filePath string, originalLine int) int {
	var baseTree *object.Tree
	if baseSHA != plumbing.ZeroHash.String() {
		baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
		if err != nil {
			return 0
		}
		bt, err := baseCommit.Tree()
		if err != nil {
			return 0
		}
		baseTree = bt
	}
	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return 0
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return 0
	}

	if baseTree == nil {
		// Root commit: all lines survive.
		hf, ferr := headCommit.File(filePath)
		if ferr != nil {
			return 0
		}
		contents, cerr := hf.Contents()
		if cerr != nil {
			return 0
		}
		lines := splitFileLines(contents)
		surviving := make([]int, len(lines))
		for i := range lines {
			surviving[i] = i + 1
		}
		return nearestLine(surviving, originalLine)
	}

	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return 0
	}

	if filePatch := patchForPath(patch, filePath); filePatch != nil {
		return nearestHunkLineInPatch(filePatch, originalLine)
	}
	return 0
}

func patchForPath(patch fdiff.Patch, filePath string) fdiff.FilePatch {
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()
		if to != nil && to.Path() == filePath {
			return filePatch
		}
		if to == nil && from != nil && from.Path() == filePath {
			return filePatch
		}
	}
	return nil
}

// nearestHunkLineInPatch collects all RIGHT-side (Equal + Add) lines and
// returns the new-file line number closest to originalLine.
func nearestHunkLineInPatch(fp fdiff.FilePatch, originalLine int) int {
	var surviving []int
	newLine := 0
	for _, chunk := range fp.Chunks() {
		lines := splitFileLines(chunk.Content())
		for range lines {
			switch chunk.Type() {
			case fdiff.Add:
				newLine++
				surviving = append(surviving, newLine)
			case fdiff.Equal:
				newLine++
				surviving = append(surviving, newLine)
			case fdiff.Delete:
				// deleted line: no new-file line number
			}
		}
	}
	return nearestLine(surviving, originalLine)
}

// nearestLine finds the integer in candidates closest to target.
func nearestLine(candidates []int, target int) int {
	if len(candidates) == 0 {
		return 0
	}
	best := 0
	bestDist := -1
	for _, c := range candidates {
		d := c - target
		if d < 0 {
			d = -d
		}
		if bestDist < 0 || d < bestDist {
			best = c
			bestDist = d
		}
	}
	return best
}

func splitFileLines(s string) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
