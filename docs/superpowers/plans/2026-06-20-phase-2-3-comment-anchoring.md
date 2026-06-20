# Phase 2.3 — Comment Anchoring Across Rebase (Implementation Plan)

<!-- agentic-worker: tdd-guide -->
<!-- sub-skill: everything-claude-code:tdd -->

**Spec:** `docs/superpowers/specs/2026-06-20-phase-2-3-comment-anchoring-design.md`
**Migration:** 015 (adds `anchored_line`, `anchor_head_sha`, `outdated`, `outdated_near_line` to `comments`)
**Depends on:** Phase 2.0 Foundation (migration 012), 2.1+2.2 (migration 014)

---

## Goal

Add pure-local comment anchoring: re-resolve inline review comments to new line
numbers after a rebase, using content-hash + ±3-line context matching over the
go-git clone. Mark comments outdated (content deleted) and attach them to the nearest
surviving hunk line.

---

## Architecture

```
go/internal/git/anchor.go          — AnchorResolver struct + Resolve()
go/internal/git/anchor_test.go     — table-driven fixture-repo tests
go/internal/store/migrations/015_add_anchor_columns.sql
go/internal/sync/writer.go         — stamp patchset_number on review-comment INSERT
go/internal/server/handler/comments.go  — call resolver; return anchored fields
go/internal/server/handler/comments_test.go
```

**Tech stack:** Go 1.26, go-git v5, `fdiff.FilePatch.Chunks()`, `crypto/sha256`,
`github.com/go-git/go-git/v5/plumbing/format/diff` (already imported in
`diff.go`). No new dependencies.

**Conventions:** `fmt.Errorf("%w", ...)`, `NewAnchorResolver(log *slog.Logger)`,
nil-guard on log (fall back to `slog.Default()`). Return `(AnchorResult, error)` —
never panic; errors mean "fell back to raw line", not a fatal condition.

---

## File Structure

```
go/internal/git/
  anchor.go           ~200 lines
  anchor_test.go      ~280 lines
go/internal/store/migrations/
  015_add_anchor_columns.sql   ~10 lines
go/internal/sync/
  writer.go           MODIFY: ~15 lines changed
go/internal/server/handler/
  comments.go         MODIFY: ~60 lines added
  comments_test.go    MODIFY: ~80 lines added
```

---

## Tasks

---

### Task 1 — Migration 015

**File:** `go/internal/store/migrations/015_add_anchor_columns.sql`

**Step 1: Write the file**

```sql
-- Migration: 015_add_anchor_columns
-- Description: Cache columns for AnchorResolver results on kind='review' comments.
--              NULL = not yet resolved. anchor_head_sha used for cache invalidation.
--              Idempotent per standard migration runner (version table guards re-run).

ALTER TABLE comments ADD COLUMN anchored_line      INTEGER;
ALTER TABLE comments ADD COLUMN anchor_head_sha    TEXT;
ALTER TABLE comments ADD COLUMN outdated           INTEGER NOT NULL DEFAULT 0;
ALTER TABLE comments ADD COLUMN outdated_near_line INTEGER;
```

**Step 2: Verify**
```bash
cd go && go test -race ./internal/store/... -v 2>&1 | tail -20
```

Existing migration runner tests must still pass (they apply all migrations in order).

---

### Task 2 — `AnchorResult` type and `AnchorResolver` constructor

**File:** `go/internal/git/anchor.go`

**Step 1: Write failing test**

```go
// anchor_test.go
package git_test

import (
    "testing"
    "github.com/itterno/bistro/internal/git"
)

func TestNewAnchorResolver_NilLog(t *testing.T) {
    r := git.NewAnchorResolver(nil)
    if r == nil {
        t.Fatal("expected non-nil resolver with nil logger")
    }
}
```

**Step 2: Run — expect compile error (type not defined yet)**
```bash
cd go && go test ./internal/git/ -run TestNewAnchorResolver -v 2>&1 | tail -10
```

**Step 3: Implement constructor**

```go
// anchor.go
package git

import (
    "log/slog"
)

// AnchorResult is the output of resolving one review comment against the current head.
type AnchorResult struct {
    CommentID        string
    AnchoredLine     int    // resolved line in current head; 0 if outdated
    OutdatedNearLine int    // nearest surviving hunk line when Outdated=true; 0 if file gone
    Outdated         bool
    Anchored         bool   // false = fell back to raw line_number (clone unavailable etc.)
    HeadSHA          string // head SHA this result is valid for
}

// AnchorResolver re-resolves inline review comments after a rebase.
// All computation is pure-local over the go-git clone; no API calls are made.
type AnchorResolver struct {
    log *slog.Logger
}

// NewAnchorResolver creates a resolver. If log is nil, slog.Default() is used.
func NewAnchorResolver(log *slog.Logger) *AnchorResolver {
    if log == nil {
        log = slog.Default()
    }
    return &AnchorResolver{log: log}
}
```

**Step 4: Run — expect PASS**
```bash
cd go && go test ./internal/git/ -run TestNewAnchorResolver -v
```

**Step 5: Commit**
```
feat(anchor): add AnchorResult type and AnchorResolver constructor
```

---

### Task 3 — Position-to-line resolver (Step 1 of algorithm)

`positionToLine` converts a GitHub unified-diff `position` value into a 1-based
file line number. It uses `fdiff.FilePatch.Chunks()`.

**Step 1: Write failing test (fixture repo)**

```go
// anchor_test.go — add below existing test

import (
    gogit "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/storage/memory"
    "github.com/go-git/go-git/v5/plumbing/object"
    "time"
)

// buildTwoCommitRepo returns a repo with two commits:
//   commit A: files[0] content
//   commit B: files[1] content
// Returns (localPath string, baseSHA string, headSHA string).
func buildTwoCommitRepo(t *testing.T, path string, contentA, contentB string) (string, string, string) {
    t.Helper()
    repo, err := gogit.PlainInit(path, false)
    if err != nil {
        t.Fatalf("init: %v", err)
    }
    wt, _ := repo.Worktree()

    writeFile := func(name, content string) {
        full := path + "/" + name
        if err := os.WriteFile(full, []byte(content), 0644); err != nil {
            t.Fatalf("write %s: %v", name, err)
        }
        if _, err := wt.Add(name); err != nil {
            t.Fatalf("add %s: %v", name, err)
        }
    }

    writeFile("foo.go", contentA)
    hashA, err := wt.Commit("commit A", &gogit.CommitOptions{
        Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
    })
    if err != nil { t.Fatalf("commit A: %v", err) }

    writeFile("foo.go", contentB)
    hashB, err := wt.Commit("commit B", &gogit.CommitOptions{
        Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
    })
    if err != nil { t.Fatalf("commit B: %v", err) }

    return path, hashA.String(), hashB.String()
}

func TestPositionToLine_Basic(t *testing.T) {
    dir := t.TempDir()
    contentA := "line1\nline2\nline3\nline4\nline5\n"
    // position=5 in a diff with 3 context lines before line4 means line4 changed
    _, baseSHA, headSHA := buildTwoCommitRepo(t, dir,
        contentA,
        "line1\nline2\nline3\nLINE4_CHANGED\nline5\n",
    )
    r := git.NewAnchorResolver(nil)
    lineNo, err := r.PositionToLine(dir, baseSHA, headSHA, "foo.go", "RIGHT", 5)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if lineNo != 4 {
        t.Errorf("expected line 4, got %d", lineNo)
    }
}
```

**Step 2: Run — expect compile failure**
```bash
cd go && go test ./internal/git/ -run TestPositionToLine -v 2>&1 | tail -10
```

**Step 3: Implement `PositionToLine` (exported for test, unexported internally)**

```go
// anchor.go (append)

import (
    "fmt"
    gogit "github.com/go-git/go-git/v5"
    fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
    "github.com/go-git/go-git/v5/plumbing"
)

// PositionToLine converts a GitHub unified-diff position (1-based, counting all
// diff output lines including context) into a 1-based file line number in the
// RIGHT (head) side of the diff for side="RIGHT", or LEFT (base) for side="LEFT".
// Returns (0, error) if the position is out of range.
func (r *AnchorResolver) PositionToLine(localPath, baseSHA, headSHA, filePath, side string, position int) (int, error) {
    repo, err := gogit.PlainOpen(localPath)
    if err != nil {
        return 0, fmt.Errorf("open repo %s: %w", localPath, err)
    }
    baseHash := plumbing.NewHash(baseSHA)
    headHash := plumbing.NewHash(headSHA)
    baseCommit, err := repo.CommitObject(baseHash)
    if err != nil {
        return 0, fmt.Errorf("base commit %s: %w", baseSHA, err)
    }
    headCommit, err := repo.CommitObject(headHash)
    if err != nil {
        return 0, fmt.Errorf("head commit %s: %w", headSHA, err)
    }
    baseTree, err := baseCommit.Tree()
    if err != nil {
        return 0, fmt.Errorf("base tree: %w", err)
    }
    headTree, err := headCommit.Tree()
    if err != nil {
        return 0, fmt.Errorf("head tree: %w", err)
    }
    patch, err := baseTree.Patch(headTree)
    if err != nil {
        return 0, fmt.Errorf("patch: %w", err)
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
    return 0, fmt.Errorf("file %q not found in diff %s..%s", filePath, baseSHA, headSHA)
}

// positionToLineInPatch walks chunk lines counting diff-position until target.
// GitHub position counts every line in the diff output (context + added + deleted).
func positionToLineInPatch(fp fdiff.FilePatch, side string, target int) (int, error) {
    pos := 0
    oldLine := 0
    newLine := 0
    for _, chunk := range fp.Chunks() {
        lines := splitLines(chunk.Content())
        op := chunk.Type()
        for _, _ = range lines {
            pos++
            switch op {
            case fdiff.Add:
                newLine++
                if pos == target {
                    if side == "RIGHT" {
                        return newLine, nil
                    }
                    return 0, fmt.Errorf("position %d is an added line; side LEFT is invalid", target)
                }
            case fdiff.Delete:
                oldLine++
                if pos == target {
                    if side == "LEFT" {
                        return oldLine, nil
                    }
                    return 0, fmt.Errorf("position %d is a deleted line; side RIGHT is invalid", target)
                }
            case fdiff.Equal:
                oldLine++
                newLine++
                if pos == target {
                    if side == "RIGHT" {
                        return newLine, nil
                    }
                    return oldLine, nil
                }
            }
        }
    }
    return 0, fmt.Errorf("position %d exceeds diff length %d", target, pos)
}

// splitLines splits a chunk content string into individual lines (non-empty).
func splitLines(s string) []string {
    var out []string
    start := 0
    for i := 0; i < len(s); i++ {
        if s[i] == '\n' {
            out = append(out, s[start:i])
            start = i + 1
        }
    }
    if start < len(s) {
        out = append(out, s[start:])
    }
    return out
}
```

**Step 4: Run — expect PASS**
```bash
cd go && go test -race ./internal/git/ -run TestPositionToLine -v
```

**Step 5: Commit**
```
feat(anchor): implement PositionToLine via fdiff.FilePatch.Chunks()
```

---

### Task 4 — Content hash computation (Step 2 of algorithm)

**Step 1: Write failing test**

```go
func TestFileLineHash_Basic(t *testing.T) {
    dir := t.TempDir()
    content := "alpha\nbeta\ngamma\ndelta\nepsilon\n"
    _, sha, _ := buildTwoCommitRepo(t, dir, content, content)
    r := git.NewAnchorResolver(nil)
    lh, ch, err := r.FileLineHash(dir, sha, "foo.go", 3) // gamma
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if lh == "" || ch == "" {
        t.Error("expected non-empty hashes")
    }
    // Idempotent
    lh2, ch2, _ := r.FileLineHash(dir, sha, "foo.go", 3)
    if lh != lh2 || ch != ch2 {
        t.Error("expected deterministic hashes")
    }
}
```

**Step 2: Run — expect compile failure**

**Step 3: Implement `FileLineHash`**

```go
// anchor.go (append)

import (
    "crypto/sha256"
    "encoding/hex"
    "strings"
)

const contextLines = 3

// FileLineHash returns (lineContentHash, contextHash) for the given 1-based lineNo
// in filePath at the given commitSHA. Both hashes are hex-encoded SHA-256.
// The line hash is over TrimSpace(line); the context hash is over the ±contextLines
// window joined with newlines (also TrimSpace per line).
func (r *AnchorResolver) FileLineHash(localPath, commitSHA, filePath string, lineNo int) (string, string, error) {
    repo, err := gogit.PlainOpen(localPath)
    if err != nil {
        return "", "", fmt.Errorf("open repo: %w", err)
    }
    commit, err := repo.CommitObject(plumbing.NewHash(commitSHA))
    if err != nil {
        return "", "", fmt.Errorf("commit %s: %w", commitSHA, err)
    }
    f, err := commit.File(filePath)
    if err != nil {
        return "", "", fmt.Errorf("file %q at %s: %w", filePath, commitSHA, err)
    }
    contents, err := f.Contents()
    if err != nil {
        return "", "", fmt.Errorf("read %q: %w", filePath, err)
    }
    lines := strings.Split(contents, "\n")
    // Remove trailing empty element from final newline
    if len(lines) > 0 && lines[len(lines)-1] == "" {
        lines = lines[:len(lines)-1]
    }
    idx := lineNo - 1
    if idx < 0 || idx >= len(lines) {
        return "", "", fmt.Errorf("line %d out of range (file has %d lines)", lineNo, len(lines))
    }
    lineHash := hashString(strings.TrimSpace(lines[idx]))

    start := idx - contextLines
    if start < 0 {
        start = 0
    }
    end := idx + contextLines + 1
    if end > len(lines) {
        end = len(lines)
    }
    var ctx []string
    for _, l := range lines[start:end] {
        ctx = append(ctx, strings.TrimSpace(l))
    }
    ctxHash := hashString(strings.Join(ctx, "\n"))
    return lineHash, ctxHash, nil
}

func hashString(s string) string {
    h := sha256.Sum256([]byte(s))
    return hex.EncodeToString(h[:])
}
```

**Step 4: Run — expect PASS**
```bash
cd go && go test -race ./internal/git/ -run TestFileLineHash -v
```

**Step 5: Commit**
```
feat(anchor): implement FileLineHash with SHA-256 ± 3-line context window
```

---

### Task 5 — Full `Resolve()` method (Steps 3 & 4 of algorithm)

**Step 1: Write failing table-driven test**

```go
// anchor_test.go — add full Resolve test suite

import "os"

type resolveCase struct {
    name          string
    contentA      string
    contentB      string
    commentLine   int   // 1-based line in commit A
    wantAnchored  bool
    wantOutdated  bool
    wantLine      int   // AnchoredLine or OutdatedNearLine depending on Outdated
}

var resolveCases = []resolveCase{
    {
        name:         "content survived same line",
        contentA:     "a\nb\ntarget\nd\ne\n",
        contentB:     "a\nb\ntarget\nd\ne\n",
        commentLine:  3,
        wantAnchored: true, wantOutdated: false, wantLine: 3,
    },
    {
        name:         "content moved down",
        contentA:     "a\nb\ntarget\nd\ne\n",
        contentB:     "x\ny\nz\na\nb\ntarget\nd\ne\n",
        commentLine:  3,
        wantAnchored: true, wantOutdated: false, wantLine: 6,
    },
    {
        name:         "content moved up",
        contentA:     "x\ny\nz\na\nb\ntarget\nd\ne\n",
        contentB:     "a\nb\ntarget\nd\ne\n",
        commentLine:  6,
        wantAnchored: true, wantOutdated: false, wantLine: 3,
    },
    {
        name:         "content deleted",
        contentA:     "a\nb\ntarget\nd\ne\n",
        contentB:     "a\nb\nd\ne\n",
        commentLine:  3,
        wantAnchored: false, wantOutdated: true,
        // nearest surviving line: 2 (b) or 3 (d) — either is acceptable
    },
    {
        name:         "file deleted entirely",
        contentA:     "a\nb\ntarget\nd\ne\n",
        contentB:     "",
        commentLine:  3,
        wantAnchored: false, wantOutdated: true, wantLine: 0,
    },
}

func TestAnchorResolver_Resolve(t *testing.T) {
    for _, tc := range resolveCases {
        t.Run(tc.name, func(t *testing.T) {
            dir := t.TempDir()
            _, commitSHA, headSHA := buildTwoCommitRepo(t, dir, tc.contentA, tc.contentB)
            r := git.NewAnchorResolver(nil)
            res, err := r.Resolve(git.ResolveRequest{
                LocalPath:   dir,
                CommitSHA:   commitSHA,
                HeadSHA:     headSHA,
                FilePath:    "foo.go",
                Side:        "RIGHT",
                Position:    positionForLine(t, dir, commitSHA, headSHA, "foo.go", tc.commentLine),
                CommentID:   "c1",
                PatchsetNum: 1,
                CurrentNum:  2,
            })
            if err != nil && tc.wantAnchored {
                t.Fatalf("unexpected error: %v", err)
            }
            if res.Outdated != tc.wantOutdated {
                t.Errorf("Outdated: got %v want %v", res.Outdated, tc.wantOutdated)
            }
            if tc.wantAnchored && res.AnchoredLine != tc.wantLine {
                t.Errorf("AnchoredLine: got %d want %d", res.AnchoredLine, tc.wantLine)
            }
            if tc.name == "file deleted entirely" && res.OutdatedNearLine != 0 {
                t.Errorf("OutdatedNearLine: got %d want 0 for deleted file", res.OutdatedNearLine)
            }
        })
    }
}
```

**Step 2: Run — expect compile failure**

**Step 3: Implement `Resolve()`**

```go
// anchor.go (append)

const maxFileLinesForAnchor = 200_000

// ResolveRequest contains all inputs needed to anchor one review comment.
type ResolveRequest struct {
    CommentID   string
    LocalPath   string
    CommitSHA   string // SHA comment was made against (GitHub commit_id)
    HeadSHA     string // current patchset head SHA
    FilePath    string
    Side        string // "LEFT" | "RIGHT"
    Position    int    // GitHub unified-diff position
    PatchsetNum int    // comment.patchset_number (0 = unknown)
    CurrentNum  int    // latest patchset number for this PR
}

// Resolve re-anchors one review comment against the current head.
// On any clone/git error it returns a non-nil AnchorResult with Anchored=false;
// the error is also returned so callers can log it.
func (r *AnchorResolver) Resolve(req ResolveRequest) (AnchorResult, error) {
    fallback := AnchorResult{CommentID: req.CommentID, Anchored: false, HeadSHA: req.HeadSHA}

    // Step 1: position → original line in commit_sha
    // Find the parent of req.CommitSHA to build the diff it was made against.
    repo, err := gogit.PlainOpen(req.LocalPath)
    if err != nil {
        r.log.Debug("anchor: clone unavailable", "path", req.LocalPath, "err", err)
        return fallback, err
    }
    commitObj, err := repo.CommitObject(plumbing.NewHash(req.CommitSHA))
    if err != nil {
        r.log.Warn("anchor: commit not found", "sha", req.CommitSHA, "err", err)
        return fallback, err
    }
    // Use the first parent as base; root commits have no parent (use empty tree).
    var baseSHA string
    if commitObj.NumParents() > 0 {
        parent, perr := commitObj.Parent(0)
        if perr != nil {
            return fallback, fmt.Errorf("parent: %w", perr)
        }
        baseSHA = parent.Hash.String()
    } else {
        baseSHA = plumbing.ZeroHash.String()
    }

    originalLine, err := r.PositionToLine(req.LocalPath, baseSHA, req.CommitSHA, req.FilePath, req.Side, req.Position)
    if err != nil {
        r.log.Warn("anchor: position out of range", "err", err)
        return fallback, err
    }

    // If the comment is on the current head already, no re-resolution needed.
    if req.CommitSHA == req.HeadSHA || req.PatchsetNum == req.CurrentNum {
        return AnchorResult{
            CommentID:    req.CommentID,
            AnchoredLine: originalLine,
            Anchored:     true,
            HeadSHA:      req.HeadSHA,
        }, nil
    }

    // Step 2: content hash at original commit
    lineHash, ctxHash, err := r.FileLineHash(req.LocalPath, req.CommitSHA, req.FilePath, originalLine)
    if err != nil {
        r.log.Warn("anchor: could not hash original line", "err", err)
        return fallback, err
    }

    // Step 3: scan current head file for matching hash
    headCommit, err := repo.CommitObject(plumbing.NewHash(req.HeadSHA))
    if err != nil {
        r.log.Warn("anchor: head commit not found", "sha", req.HeadSHA, "err", err)
        return fallback, err
    }
    headFile, err := headCommit.File(req.FilePath)
    if err != nil {
        // File deleted entirely
        nearLine := r.nearestHunkLine(repo, baseSHA, req.HeadSHA, req.FilePath, originalLine)
        return AnchorResult{
            CommentID:        req.CommentID,
            Outdated:         true,
            OutdatedNearLine: nearLine,
            Anchored:         false,
            HeadSHA:          req.HeadSHA,
        }, nil
    }
    contents, err := headFile.Contents()
    if err != nil {
        return fallback, fmt.Errorf("read head file: %w", err)
    }
    lines := strings.Split(contents, "\n")
    if len(lines) > 0 && lines[len(lines)-1] == "" {
        lines = lines[:len(lines)-1]
    }
    if len(lines) > maxFileLinesForAnchor {
        r.log.Warn("anchor: file too large, falling back to line number",
            "file", req.FilePath, "lines", len(lines))
        return fallback, nil
    }

    for i, line := range lines {
        if hashString(strings.TrimSpace(line)) != lineHash {
            continue
        }
        // Candidate: check context hash
        start := i - contextLines
        if start < 0 { start = 0 }
        end := i + contextLines + 1
        if end > len(lines) { end = len(lines) }
        var ctx []string
        for _, l := range lines[start:end] {
            ctx = append(ctx, strings.TrimSpace(l))
        }
        if hashString(strings.Join(ctx, "\n")) == ctxHash {
            return AnchorResult{
                CommentID:    req.CommentID,
                AnchoredLine: i + 1,
                Anchored:     true,
                HeadSHA:      req.HeadSHA,
            }, nil
        }
    }

    // Step 4: outdated — find nearest surviving hunk line
    nearLine := r.nearestHunkLine(repo, baseSHA, req.HeadSHA, req.FilePath, originalLine)
    return AnchorResult{
        CommentID:        req.CommentID,
        Outdated:         true,
        OutdatedNearLine: nearLine,
        Anchored:         false,
        HeadSHA:          req.HeadSHA,
    }, nil
}

// nearestHunkLine returns the RIGHT-side line number closest to originalLine
// among all added and context lines in the diff for filePath between baseSHA
// and headSHA. Returns 0 if the file has no surviving lines.
func (r *AnchorResolver) nearestHunkLine(repo *gogit.Repository, baseSHA, headSHA, filePath string, originalLine int) int {
    baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
    if err != nil {
        return 0
    }
    headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
    if err != nil {
        return 0
    }
    baseTree, _ := baseCommit.Tree()
    headTree, _ := headCommit.Tree()
    patch, err := baseTree.Patch(headTree)
    if err != nil {
        return 0
    }
    for _, fp := range patch.FilePatches() {
        _, to := fp.Files()
        if to == nil || to.Path() != filePath {
            continue
        }
        survivingLines := collectSurvivingLines(fp)
        return closestLine(survivingLines, originalLine)
    }
    return 0
}

// collectSurvivingLines returns all new line numbers (RIGHT side) from context
// and added chunks.
func collectSurvivingLines(fp fdiff.FilePatch) []int {
    var result []int
    newLine := 0
    for _, chunk := range fp.Chunks() {
        lines := splitLines(chunk.Content())
        for range lines {
            switch chunk.Type() {
            case fdiff.Add, fdiff.Equal:
                newLine++
                result = append(result, newLine)
            case fdiff.Delete:
                // deleted lines have no new-side line number
            }
        }
    }
    return result
}

// closestLine returns the element of candidates closest to target.
// Returns 0 if candidates is empty.
func closestLine(candidates []int, target int) int {
    if len(candidates) == 0 {
        return 0
    }
    best := candidates[0]
    bestDist := abs(best - target)
    for _, c := range candidates[1:] {
        if d := abs(c - target); d < bestDist {
            best = c
            bestDist = d
        }
    }
    return best
}

func abs(x int) int {
    if x < 0 { return -x }
    return x
}
```

**Step 4: Run — expect PASS**
```bash
cd go && go test -race ./internal/git/ -run TestAnchorResolver_Resolve -v
```

**Step 5: Run full git package tests**
```bash
cd go && go test -race ./internal/git/... -v 2>&1 | tail -30
```

**Step 6: Commit**
```
feat(anchor): implement Resolve() with content-hash re-anchoring and outdated detection
```

---

### Task 6 — Missing clone fallback + clone-unavailable test

**Step 1: Write failing test**

```go
func TestAnchorResolver_Resolve_MissingClone(t *testing.T) {
    r := git.NewAnchorResolver(nil)
    res, err := r.Resolve(git.ResolveRequest{
        LocalPath: "/tmp/does-not-exist-bistro-test",
        CommitSHA: "aaaa",
        HeadSHA:   "bbbb",
        FilePath:  "foo.go",
        Side:      "RIGHT",
        Position:  1,
        CommentID: "c1",
    })
    if err == nil {
        t.Error("expected error for missing clone")
    }
    if res.Anchored {
        t.Error("expected Anchored=false for missing clone")
    }
}
```

**Step 2: Run — should PASS already (fallback implemented in Task 5)**
```bash
cd go && go test -race ./internal/git/ -run TestAnchorResolver_Resolve_MissingClone -v
```

**Step 3: Commit (test only)**
```
test(anchor): add missing-clone fallback coverage
```

---

### Task 7 — Stamp `patchset_number` in `writer.go`

**File:** `go/internal/sync/writer.go`

**Step 1: Write failing test (in writer_test.go)**

```go
// In writer_test.go — add to existing test suite

func TestWriteReviewComments_StampsPatchsetNumber(t *testing.T) {
    db := newTestDB(t)
    // Insert a PR
    mustExec(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
        created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
        VALUES ('pr1','github','o','r',1,'t','a','open','2026-01-01','2026-01-01','main','feat','b','headSHA1')`)
    // Insert a patchset
    mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
        VALUES ('pr1', 1, 'headSHA1', 'b', '2026-01-01')`)

    w := sync.NewWriter(db, nil)
    err := w.WriteReviewComments("pr1", []provider.Comment{
        {ID: "c1", PRID: "pr1", Kind: "review", CommitSHA: "headSHA1",
         Body: "fix this", Author: "alice", CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
    })
    if err != nil {
        t.Fatalf("WriteReviewComments: %v", err)
    }
    var psNum *int
    row := db.QueryRow(`SELECT patchset_number FROM comments WHERE id = 'c1'`)
    if err := row.Scan(&psNum); err != nil {
        t.Fatalf("scan: %v", err)
    }
    if psNum == nil || *psNum != 1 {
        t.Errorf("patchset_number: got %v want 1", psNum)
    }
}
```

**Step 2: Run — expect failure (method may not yet exist or not stamp)**
```bash
cd go && go test -race ./internal/sync/ -run TestWriteReviewComments_StampsPatchsetNumber -v 2>&1 | tail -20
```

**Step 3: Modify `writer.go`**

In the review-comment upsert block (added by Phase 2.0 Foundation), after the
patchset upsert, add a read-back:

```go
// After patchset INSERT OR IGNORE in writeReviewComments:
var patchsetNum *int
row := tx.QueryRowContext(ctx,
    `SELECT number FROM patchsets WHERE pr_id = ? AND head_sha = ?`,
    prID, comment.CommitSHA)
var n int
if err := row.Scan(&n); err == nil {
    patchsetNum = &n
} else {
    w.log.Warn("writer: patchset_number lookup failed", "pr_id", prID, "commit_sha", comment.CommitSHA, "err", err)
}
// Use patchsetNum in the comment INSERT (bind as nullable integer)
```

The exact edit depends on the state of the foundation writer; the principle is: bind
`patchset_number = patchsetNum` in the review-comment INSERT or UPSERT statement.

**Step 4: Run — expect PASS**
```bash
cd go && go test -race ./internal/sync/ -run TestWriteReviewComments_StampsPatchsetNumber -v
```

**Step 5: Commit**
```
feat(writer): stamp patchset_number on review comment INSERT from patchsets lookup
```

---

### Task 8 — Comments handler: serve anchored fields

**File:** `go/internal/server/handler/comments.go`

**Step 1: Write failing test**

```go
// comments_test.go — add anchor response test

func TestRequestComments_ReturnsAnchoredLine(t *testing.T) {
    db := newHandlerTestDB(t)
    // Insert PR + patchsets + a review comment with cached anchor
    mustExec(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
        created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
        VALUES ('p1','github','o','r',1,'t','a','open','2026-01-01','2026-01-01','main','f','b','sha2')`)
    mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
        VALUES ('p1', 2, 'sha2', 'b', '2026-01-01')`)
    mustExec(t, db, `INSERT INTO comments
        (id, pr_id, author, body, created_at, updated_at, kind, commit_sha, patchset_number,
         line_number, file_path, side, position,
         anchored_line, anchor_head_sha, outdated)
        VALUES
        ('c1','p1','alice','lgtm','2026-01-01','2026-01-01','review','sha1',1,
         10,'foo.go','RIGHT',5,
         12,'sha2',0)`)

    handler := handler.NewRequestCommentsHandler(db, nil)
    msg := ws.InboundMessage{
        Type: "REQUEST_COMMENTS",
        Data: []byte(`{"pr_id":"p1"}`),
    }
    resp, err := handler(context.Background(), msg)
    if err != nil {
        t.Fatalf("handler error: %v", err)
    }
    rows := resp.([]handler.CommentRow)
    if len(rows) != 1 {
        t.Fatalf("expected 1 row, got %d", len(rows))
    }
    if rows[0].AnchoredLine == nil || *rows[0].AnchoredLine != 12 {
        t.Errorf("AnchoredLine: got %v want 12", rows[0].AnchoredLine)
    }
    if rows[0].Outdated {
        t.Error("expected Outdated=false")
    }
}
```

**Step 2: Run — expect compile failure (fields not yet on commentRow)**
```bash
cd go && go test -race ./internal/server/handler/ -run TestRequestComments_ReturnsAnchoredLine -v 2>&1 | tail -20
```

**Step 3: Modify `comments.go`**

1. Extend `commentRow` struct:
```go
type commentRow struct {
    ID               string  `json:"id"`
    PRID             string  `json:"pr_id"`
    Author           string  `json:"author"`
    Body             string  `json:"body"`
    CreatedAt        string  `json:"created_at"`
    UpdatedAt        string  `json:"updated_at"`
    Line             *int    `json:"line,omitempty"`
    File             *string `json:"file,omitempty"`
    Kind             string  `json:"kind,omitempty"`
    Side             *string `json:"side,omitempty"`
    CommitSHA        *string `json:"commit_sha,omitempty"`
    PatchsetNumber   *int    `json:"patchset_number,omitempty"`
    AnchoredLine     *int    `json:"anchored_line,omitempty"`
    OutdatedNearLine *int    `json:"outdated_near_line,omitempty"`
    Outdated         bool    `json:"outdated"`
    Anchored         bool    `json:"anchored"`
}
```

2. Update the SELECT query to include new columns and join against patchsets for cache
   invalidation check. When `anchor_head_sha = current_head_sha`, read cached values.
   When mismatch (or NULL), call `AnchorResolver.Resolve()` and write back.

3. Handler receives an `AnchorResolver` (constructor injection):
```go
func NewRequestCommentsHandler(db prLister, resolver *git.AnchorResolver, log *slog.Logger) ws.MessageHandler
```

**Step 4: Run — expect PASS**
```bash
cd go && go test -race ./internal/server/handler/ -run TestRequestComments -v
```

**Step 5: Fix any wiring in server setup (where NewRequestCommentsHandler is called)**
```bash
cd go && go build ./... 2>&1
```

**Step 6: Commit**
```
feat(handler): serve anchored_line, outdated, outdated_near_line in REQUEST_COMMENTS
```

---

### Task 9 — Full suite + vet

```bash
cd go && go vet ./...
cd go && go test -race ./... 2>&1 | tail -30
```

All tests must pass. Zero data races. Zero vet warnings.

**Commit if anything was fixed:**
```
fix(anchor): address vet warnings and test failures
```

---

### Task 10 — Update slice tracking table

Update `docs/superpowers/plans/2026-06-20-phase-2-decomposition.md` slice table:
mark 2.3 as `spec ✅ / plan ✅`.

```
chore(docs): mark phase 2.3 spec and plan as written
```

---

## Verify Commands (final)

```bash
cd go && go vet ./...
cd go && go test -race ./internal/git/ -run TestAnchor -v
cd go && go test -race ./internal/sync/ -run TestWriteReviewComments -v
cd go && go test -race ./internal/server/handler/ -run TestRequestComments -v
cd go && go test -race ./... 2>&1 | grep -E 'FAIL|PASS|ok'
```
