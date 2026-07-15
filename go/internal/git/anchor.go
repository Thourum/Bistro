package git

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	dmp "github.com/sergi/go-diff/diffmatchpatch"
)

const (
	contextLines          = 3
	maxFileLinesForAnchor = 200_000
)

// AnchorResult is the output of AnchorResolver.Resolve for one comment.
type AnchorResult struct {
	CommentID        string
	AnchoredLine     int  // 1-based line in HeadSHA where content was found; valid when Anchored=true
	Outdated         bool // true when content was deleted or file is gone in HeadSHA
	OutdatedNearLine int  // nearest surviving hunk line when Outdated=true; 0 = file entirely gone
	Anchored         bool // true when content was located in HeadSHA
	HeadSHA          string
	// PatchsetNumber is the SOURCE patchset whose geometry (base_sha) was used for
	// position-to-line mapping. Equals current patchset for current-position comments;
	// equals the historical patchset for original-position (outdated) comments.
	// Set by resolveAnchors after Resolve returns.
	PatchsetNumber int
	// CurrentPatchsetNumber is the CURRENT patchset at time of anchor resolution —
	// the (pr_id, base_sha, head_sha) state the anchor is valid against.
	// Persisted as anchor_current_patchset_number. Handler freshness requires
	// anchor_current_patchset_number == current patchset, guarding against
	// same-head/different-base retargets that advance the current patchset number.
	CurrentPatchsetNumber int
}

// ResolveRequest carries all inputs needed to re-anchor one review comment.
type ResolveRequest struct {
	LocalPath string // path to the local go-git repo
	CommitSHA string // commit where the comment was originally posted (PR head at that time)
	HeadSHA   string // current PR head SHA to resolve against
	// DiffBaseSHA is the patchset base (PR declared base at time of comment).
	// REQUIRED for correct geometry: GitHub positions are relative to
	// DiffBaseSHA..CommitSHA, NOT to parent(CommitSHA)..CommitSHA.
	// When empty the resolver falls back to firstParent(CommitSHA) which is
	// correct only for single-commit PRs; multi-commit PRs will degrade.
	DiffBaseSHA string
	FilePath    string
	Side        string // "LEFT" | "RIGHT"
	Position    int    // GitHub unified-diff position (1-based sequential count)
	CommentID   string
	PatchsetNum int // patchset number when comment was posted
	CurrentNum  int // current patchset number (used for same-patchset fast path)
}

// AnchorResolver re-anchors GitHub inline review comments across rebases.
// It uses the local go-git clone (already fetched by the sync worker for diffs)
// and never clones; gracefully degrades when the clone is absent.
type AnchorResolver struct {
	log *slog.Logger
}

// NewAnchorResolver creates an AnchorResolver. Pass nil to use slog.Default().
func NewAnchorResolver(log *slog.Logger) *AnchorResolver {
	if log == nil {
		log = slog.Default()
	}
	return &AnchorResolver{log: log}
}

// Resolve re-anchors a single review comment against the current head.
//
// Geometry: GitHub diff positions are relative to DiffBaseSHA..CommitSHA (the
// full PR/patchset diff), not to parent(CommitSHA)..CommitSHA. Callers must
// supply DiffBaseSHA for correct results on multi-commit PRs.
//
// Algorithm:
//  1. Convert GitHub diff position → original file line using DiffBaseSHA..CommitSHA.
//  2. Compute content hash + context hash at that line.
//  3. Scan HeadSHA file for matching hash pair → AnchoredLine.
//  4. If not found → Outdated=true, find nearest surviving hunk line.
//
// On any clone/git error returns AnchorResult{Anchored:false} + the error.
func (r *AnchorResolver) Resolve(req ResolveRequest) (AnchorResult, error) {
	fallback := AnchorResult{CommentID: req.CommentID, Anchored: false, HeadSHA: req.HeadSHA}

	repo, err := gogit.PlainOpen(req.LocalPath)
	if err != nil {
		r.log.Debug("anchor: clone unavailable", "path", req.LocalPath, "err", err)
		return fallback, err
	}

	// Determine the diff base for position→line conversion.
	// DiffBaseSHA is preferred (correct PR geometry); fall back to firstParent
	// only when not provided (backward-compat / graceful degradation).
	diffBase := req.DiffBaseSHA
	if diffBase == "" {
		commitObj, cerr := repo.CommitObject(plumbing.NewHash(req.CommitSHA))
		if cerr != nil {
			r.log.Warn("anchor: commit not found", "sha", req.CommitSHA, "err", cerr)
			return fallback, cerr
		}
		if commitObj.NumParents() > 0 {
			parent, perr := commitObj.Parent(0)
			if perr != nil {
				return fallback, fmt.Errorf("parent commit: %w", perr)
			}
			diffBase = parent.Hash.String()
		} else {
			diffBase = plumbing.ZeroHash.String()
		}
	}

	// Step 1: GitHub diff position → original file line in CommitSHA.
	originalLine, err := r.PositionToLine(req.LocalPath, diffBase, req.CommitSHA, req.FilePath, req.Side, req.Position)
	if err != nil {
		r.log.Warn("anchor: position out of range", "comment_id", req.CommentID, "err", err)
		return fallback, err
	}

	// Fast path: comment is on the current head — no re-resolution needed.
	if req.CommitSHA == req.HeadSHA || req.PatchsetNum == req.CurrentNum {
		return AnchorResult{
			CommentID:    req.CommentID,
			AnchoredLine: originalLine,
			Anchored:     true,
			HeadSHA:      req.HeadSHA,
		}, nil
	}

	// Step 2: compute content hash at original line in CommitSHA.
	lineHash, ctxHash, err := r.FileLineHash(req.LocalPath, req.CommitSHA, req.FilePath, originalLine)
	if err != nil {
		r.log.Warn("anchor: could not hash original line", "err", err)
		return fallback, err
	}

	// Step 3: scan HeadSHA file for matching hash pair.
	headCommit, err := repo.CommitObject(plumbing.NewHash(req.HeadSHA))
	if err != nil {
		r.log.Warn("anchor: head commit not found", "sha", req.HeadSHA, "err", err)
		return fallback, err
	}
	headFile, err := headCommit.File(req.FilePath)
	if err != nil {
		// File deleted entirely in the current head.
		nearLine := r.nearestHunkLine(repo, diffBase, req.HeadSHA, req.FilePath, originalLine)
		return AnchorResult{
			CommentID:        req.CommentID,
			Outdated:         true,
			OutdatedNearLine: nearLine,
			Anchored:         false,
			HeadSHA:          req.HeadSHA,
		}, nil
	}
	headContents, err := headFile.Contents()
	if err != nil {
		return fallback, fmt.Errorf("read head file: %w", err)
	}

	headLines := splitFileLines(headContents)
	if len(headLines) > maxFileLinesForAnchor {
		return fallback, fmt.Errorf("file too large to anchor (%d lines)", len(headLines))
	}

	// Scan for content hash match, then verify context hash.
	for i, line := range headLines {
		if sha256hex(strings.TrimSpace(line)) != lineHash {
			continue
		}
		lo, hi := contextBounds(len(headLines), i)
		ctxParts := make([]string, 0, hi-lo)
		for _, l := range headLines[lo:hi] {
			ctxParts = append(ctxParts, strings.TrimSpace(l))
		}
		if sha256hex(strings.Join(ctxParts, "\n")) == ctxHash {
			return AnchorResult{
				CommentID:    req.CommentID,
				AnchoredLine: i + 1,
				Anchored:     true,
				HeadSHA:      req.HeadSHA,
			}, nil
		}
	}

	// Step 4: content not found → outdated; find nearest surviving hunk line.
	nearLine := r.nearestHunkLine(repo, diffBase, req.HeadSHA, req.FilePath, originalLine)
	return AnchorResult{
		CommentID:        req.CommentID,
		Outdated:         true,
		OutdatedNearLine: nearLine,
		Anchored:         false,
		HeadSHA:          req.HeadSHA,
	}, nil
}

// PositionToLine converts a GitHub unified-diff position value (1-based,
// counting every diff content line across all hunks) to the 1-based file line
// number on the given Side ("LEFT"=old/deleted, "RIGHT"=new/added/context).
//
// The base..head range MUST use DiffBaseSHA..CommitSHA (the actual PR/patchset
// diff range), not parent(CommitSHA)..CommitSHA, to match GitHub's position
// numbering for multi-commit PRs.
func (r *AnchorResolver) PositionToLine(localPath, baseSHA, headSHA, filePath, side string, position int) (int, error) {
	if position < 1 {
		return 0, fmt.Errorf("position must be >= 1, got %d", position)
	}

	// Root-commit base (zero hash): entire file is one Add chunk; position == line.
	if baseSHA == plumbing.ZeroHash.String() {
		return r.positionInRootCommit(localPath, headSHA, filePath, side, position)
	}

	repo, err := gogit.PlainOpen(localPath)
	if err != nil {
		return 0, fmt.Errorf("open repo: %w", err)
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
		return 0, fmt.Errorf("patch %s..%s: %w", baseSHA[:7], headSHA[:7], err)
	}

	for _, fp := range patch.FilePatches() {
		from, to := fp.Files()
		var fpPath string
		if to != nil {
			fpPath = to.Path()
		} else if from != nil {
			fpPath = from.Path()
		}
		if fpPath != filePath {
			continue
		}
		return positionToLineInPatch(fp, side, position)
	}
	return 0, fmt.Errorf("file %q not found in diff %s..%s", filePath, baseSHA[:7], headSHA[:7])
}

// positionInRootCommit handles the zero-hash base case (new file, all additions).
// Position == line number for RIGHT side.
func (r *AnchorResolver) positionInRootCommit(localPath, commitSHA, filePath, side string, position int) (int, error) {
	repo, err := gogit.PlainOpen(localPath)
	if err != nil {
		return 0, fmt.Errorf("open repo: %w", err)
	}
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
	if err := enc.Encode(&singleFilePatch{fp: fp}); err != nil {
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

// FileLineHash returns (lineContentHash, contextHash) for the 1-based lineNo
// in filePath at commitSHA. Both are hex-encoded SHA-256 values.
func (r *AnchorResolver) FileLineHash(localPath, commitSHA, filePath string, lineNo int) (lineHash, ctxHash string, err error) {
	repo, rerr := gogit.PlainOpen(localPath)
	if rerr != nil {
		return "", "", fmt.Errorf("open repo: %w", rerr)
	}
	commit, cerr := repo.CommitObject(plumbing.NewHash(commitSHA))
	if cerr != nil {
		return "", "", fmt.Errorf("commit %s: %w", commitSHA, cerr)
	}
	f, ferr := commit.File(filePath)
	if ferr != nil {
		return "", "", fmt.Errorf("file %s@%s: %w", filePath, commitSHA[:7], ferr)
	}
	contents, ierr := f.Contents()
	if ierr != nil {
		return "", "", fmt.Errorf("read contents: %w", ierr)
	}
	lines := splitFileLines(contents)
	if lineNo < 1 || lineNo > len(lines) {
		return "", "", fmt.Errorf("lineNo %d out of range [1, %d]", lineNo, len(lines))
	}

	targetLine := strings.TrimSpace(lines[lineNo-1])
	lineHash = sha256hex(targetLine)

	lo, hi := contextBounds(len(lines), lineNo-1)
	ctxParts := make([]string, 0, hi-lo)
	for _, l := range lines[lo:hi] {
		ctxParts = append(ctxParts, strings.TrimSpace(l))
	}
	ctxHash = sha256hex(strings.Join(ctxParts, "\n"))
	return lineHash, ctxHash, nil
}

// nearestHunkLine returns the RIGHT-side line number closest to originalLine
// among all added and context lines in the diff baseSHA..headSHA for filePath.
func (r *AnchorResolver) nearestHunkLine(repo *gogit.Repository, baseSHA, headSHA, filePath string, originalLine int) int {
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

	for _, fp := range patch.FilePatches() {
		from, to := fp.Files()
		var fpPath string
		if to != nil {
			fpPath = to.Path()
		} else if from != nil {
			fpPath = from.Path()
		}
		if fpPath != filePath {
			continue
		}
		return nearestHunkLineInPatch(fp, originalLine)
	}
	return 0
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

// ── helpers ────────────────────────────────────────────────────────────────

func splitFileLines(s string) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
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
