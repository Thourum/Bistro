package anchor

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const maxFileLinesForAnchor = 200_000

// Result is the output of Resolver.Resolve for one comment.
type Result struct {
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

// Request carries all inputs needed to re-anchor one review comment.
type Request struct {
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

// Resolver re-anchors GitHub inline review comments across rebases.
// It uses the local go-git clone (already fetched by the sync worker for diffs)
// and never clones; gracefully degrades when the clone is absent.
type Resolver struct{}

// NewResolver creates a Resolver. Errors are returned to the
// caller rather than logged inside the resolver.
func NewResolver() *Resolver {
	return &Resolver{}
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
// On any clone/git error returns Result{Anchored:false} + the error.
func (r *Resolver) Resolve(req Request) (Result, error) {
	fallback := Result{CommentID: req.CommentID, Anchored: false, HeadSHA: req.HeadSHA}

	repo, err := gogit.PlainOpen(req.LocalPath)
	if err != nil {
		return fallback, fmt.Errorf("open anchor repository %q: %w", req.LocalPath, err)
	}

	diffBase, err := anchorDiffBase(repo, req)
	if err != nil {
		return fallback, err
	}

	// Step 1: GitHub diff position → original file line in CommitSHA.
	originalLine, err := positionToLine(repo, diffBase, req.CommitSHA, req.FilePath, req.Side, req.Position)
	if err != nil {
		return fallback, fmt.Errorf("anchor comment %s position: %w", req.CommentID, err)
	}

	// Fast path: comment is on the current head — no re-resolution needed.
	if req.CommitSHA == req.HeadSHA || req.PatchsetNum == req.CurrentNum {
		return Result{
			CommentID:    req.CommentID,
			AnchoredLine: originalLine,
			Anchored:     true,
			HeadSHA:      req.HeadSHA,
		}, nil
	}

	// Step 2: compute content hash at original line in CommitSHA.
	lineHash, ctxHash, err := fileLineHash(repo, req.CommitSHA, req.FilePath, originalLine)
	if err != nil {
		return fallback, fmt.Errorf("hash original anchor line: %w", err)
	}

	return resolveAgainstHead(repo, req, diffBase, originalLine, anchorHashes{line: lineHash, context: ctxHash})
}

// anchorDiffBase prefers the PR's actual diff base, with a first-parent
// fallback for older callers that do not supply one.
func anchorDiffBase(repo *gogit.Repository, req Request) (string, error) {
	if req.DiffBaseSHA != "" {
		return req.DiffBaseSHA, nil
	}
	commit, err := repo.CommitObject(plumbing.NewHash(req.CommitSHA))
	if err != nil {
		return "", fmt.Errorf("anchor commit %s: %w", req.CommitSHA, err)
	}
	if commit.NumParents() == 0 {
		return plumbing.ZeroHash.String(), nil
	}
	parent, err := commit.Parent(0)
	if err != nil {
		return "", fmt.Errorf("parent commit: %w", err)
	}
	return parent.Hash.String(), nil
}

func resolveAgainstHead(repo *gogit.Repository, req Request, diffBase string, originalLine int, hashes anchorHashes) (Result, error) {
	fallback := Result{CommentID: req.CommentID, HeadSHA: req.HeadSHA}
	headCommit, err := repo.CommitObject(plumbing.NewHash(req.HeadSHA))
	if err != nil {
		return fallback, fmt.Errorf("anchor head commit %s: %w", req.HeadSHA, err)
	}
	headFile, err := headCommit.File(req.FilePath)
	if err != nil {
		return outdatedAnchor(repo, req, diffBase, originalLine), nil
	}
	headContents, err := headFile.Contents()
	if err != nil {
		return fallback, fmt.Errorf("read head file: %w", err)
	}
	headLines := splitFileLines(headContents)
	if len(headLines) > maxFileLinesForAnchor {
		return fallback, fmt.Errorf("file too large to anchor (%d lines)", len(headLines))
	}
	if line := matchingHeadLine(headLines, hashes); line != 0 {
		return Result{CommentID: req.CommentID, AnchoredLine: line, Anchored: true, HeadSHA: req.HeadSHA}, nil
	}
	return outdatedAnchor(repo, req, diffBase, originalLine), nil
}

func matchingHeadLine(lines []string, hashes anchorHashes) int {
	for index, line := range lines {
		if sha256hex(strings.TrimSpace(line)) != hashes.line {
			continue
		}
		lo, hi := contextBounds(len(lines), index)
		if contextHash(lines[lo:hi]) == hashes.context {
			return index + 1
		}
	}
	return 0
}

func outdatedAnchor(repo *gogit.Repository, req Request, diffBase string, originalLine int) Result {
	return Result{
		CommentID:        req.CommentID,
		Outdated:         true,
		OutdatedNearLine: nearestHunkLine(repo, diffBase, req.HeadSHA, req.FilePath, originalLine),
		HeadSHA:          req.HeadSHA,
	}
}
