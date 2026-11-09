# Commit-by-commit review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: subagent-driven-development task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let reviewers read a PR one commit at a time on dedicated routes, with local per-commit viewed state that survives content-preserving rebases.

**Architecture:** A go-git range walk in `internal/git/diff` lists `merge-base..head` commits and diffs one commit against its first parent on demand, under the shared repo lock. A new `services.Commits` (List / Get / SetViewed) wraps it, keyed by an occurrence-aware patch fingerprint persisted in a new `commitviewed` store. The renderer adds `/prs/$prId/commits` and `/prs/$prId/commits/$sha` pages, a `services/commits.ts` request module, and pure keybinding config.

**Tech Stack:** Go 1.26.1 (go-git v5.19.3, sqlc, goose), Wails v3.0.0 bindings, React 19 + TypeScript, TanStack Router/Store/Hotkeys, `@thourum/fault`, Vitest, Biome.

**Spec:** `docs/superpowers/specs/2026-11-02-commit-by-commit-review-design.md` — read it before any task.

## Global Constraints

- Go module path is `github.com/itterno/bistro` (from `go.mod`).
- Range cap: `250` commits, oldest kept, `Truncated` set when more exist.
- Viewed key, non-merge: `<commitPatchID>:<n>` (n = zero-based ordinal among earlier same-fingerprint commits, oldest first) plus stored `occurrences`; viewed only when stored `occurrences` equals the current count. Merge key: `merge:<sha>`, occurrences `1`.
- Merge commits: no diff, zero stats.
- Unavailable states return `available: false` + `reason`, never an error. Classified errors: unknown PR `not_found`; malformed or out-of-range SHA `invalid` (out-of-range uses `Reason: "commit_not_in_pr"`); no login on `SetViewed` `unauthenticated`.
- Frontend TS uses `@thourum/fault` Result chains — never `try`/`catch`. Reads via `call()`, writes via `callWrite()` from `frontend/src/services/engine/call.ts`. Never call bindings from components.
- Hotkeys are registered only through `useAppHotkeys`; bindings are config in `ACTION_CATALOG` (`utils/keybindings.ts`) + `KEYMAPS` (`utils/keymaps.ts`); handlers in `utils/keybindingActions.ts`.
- Never hand-edit `frontend/bindings/` or `internal/store/db/`; regenerate with `wails3 task generate` (run from repo root).
- No React component tests. Utility/hook tests only, and only where they would fail if logic broke.
- Commit after each task (`type: description`). Never `--no-verify`, never push. Pre-commit hook blocks authored files over ~5,000 tokens (bytes/4) — keep new files small.
- Full verification (repo root): `go vet ./... && go test -race ./...`; then `cd frontend && pnpm type-check && pnpm lint && pnpm test`.

---

### Task 1: Git commit range and per-commit diff

**Files:**
- Create: `internal/git/diff/commits.go`
- Create: `internal/git/diff/commits_walk.go` (range walk + topological sort, kept separate so each file stays small)
- Test: `internal/git/diff/commits_test.go`

**Interfaces:**
- Consumes: `repolock.Locks.WithRepo`, `repolock.Key`, existing unexported `renderDiffFiles(patch, opts)`, `DefaultOptions()`, `ErrHeadUnavailable`, `File` (all in package `diff`).
- Produces (used by Task 3):

```go
const CommitRangeLimit = 250
var ErrCommitNotInRange = errors.New("commit is not in the PR range")

type CommitRangeRequest struct {
	RepoURL      string // lock key, same URL sync clones with
	RepoPath     string
	HeadSHA      string
	MergeBaseSHA string
	WithDiffs    bool // compute Diff for every non-merge commit
}
type CommitDiff struct {
	Files     []*File
	Unified   string
	Additions int
	Deletions int
	PatchID   string // commit fingerprint: sha256 over file PatchIDs
}
type RangeCommit struct {
	SHA        string
	ParentSHAs []string
	Subject    string
	Body       string
	AuthorName string
	AuthoredAt time.Time
	IsMerge    bool
	Diff       *CommitDiff // nil for merges, or when diffs were not requested
}
type CommitRange struct {
	Commits   []RangeCommit // oldest first, at most CommitRangeLimit
	Truncated bool
}

func ListCommitRange(locks *repolock.Locks, req CommitRangeRequest) (CommitRange, error)
// GetRangeCommit returns one commit of the capped range with its Diff
// (nil for merges); ErrCommitNotInRange when sha is not in it.
func GetRangeCommit(locks *repolock.Locks, req CommitRangeRequest, sha string) (RangeCommit, error)
```

- [ ] **Step 1: Implement the range walk** in `commits_walk.go`.

Algorithm (git's two-colour "paint down"): walk from head and merge-base together in committer-time order (newest first); flags propagate to parents; stop once every queued commit carries the base flag. Commits whose final flag is head-only are the range. Then order them topologically, parents first, ties by committer time then SHA.

```go
package diff

import (
	"container/heap"
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/logging"
)

const (
	flagHead uint8 = 1 << iota
	flagBase
)

type commitQueue []*object.Commit

func (q commitQueue) Len() int { return len(q) }
func (q commitQueue) Less(i, j int) bool {
	ti, tj := q[i].Committer.When, q[j].Committer.When
	if !ti.Equal(tj) {
		return ti.After(tj)
	}
	return q[i].Hash.String() < q[j].Hash.String()
}
func (q commitQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *commitQueue) Push(x any)   { *q = append(*q, x.(*object.Commit)) }
func (q *commitQueue) Pop() any {
	old := *q
	c := old[len(old)-1]
	*q = old[:len(old)-1]
	return c
}

func loadRangeCommit(repo *gitlib.Repository, sha string) (*object.Commit, error) {
	c, err := repo.CommitObject(plumbing.NewHash(sha))
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrHeadUnavailable, logging.ShortSHA(sha))
	}
	if err != nil {
		return nil, fmt.Errorf("load commit %s: %w", logging.ShortSHA(sha), err)
	}
	return c, nil
}

// rangeCommits returns commits reachable from head but not from mergeBase,
// parents before children.
func rangeCommits(repo *gitlib.Repository, headSHA, mergeBaseSHA string) ([]*object.Commit, error) {
	head, err := loadRangeCommit(repo, headSHA)
	if err != nil {
		return nil, err
	}
	base, err := loadRangeCommit(repo, mergeBaseSHA)
	if err != nil {
		return nil, err
	}
	flags := map[plumbing.Hash]uint8{head.Hash: flagHead}
	flags[base.Hash] |= flagBase
	seen := map[plumbing.Hash]*object.Commit{head.Hash: head, base.Hash: base}
	queue := &commitQueue{head}
	if base.Hash != head.Hash {
		heap.Push(queue, base)
	}
	for queue.Len() > 0 && anyHeadOnly(*queue, flags) {
		c := heap.Pop(queue).(*object.Commit)
		f := flags[c.Hash]
		for _, parentHash := range c.ParentHashes {
			before := flags[parentHash]
			after := before | f
			if after == before {
				continue
			}
			flags[parentHash] = after
			parent, ok := seen[parentHash]
			if !ok {
				if parent, err = loadRangeCommit(repo, parentHash.String()); err != nil {
					return nil, err
				}
				seen[parentHash] = parent
			}
			heap.Push(queue, parent)
		}
	}
	var candidates []*object.Commit
	for hash, c := range seen {
		if flags[hash] == flagHead {
			candidates = append(candidates, c)
		}
	}
	return topoOldestFirst(candidates), nil
}

func anyHeadOnly(queue commitQueue, flags map[plumbing.Hash]uint8) bool {
	for _, c := range queue {
		if flags[c.Hash]&flagBase == 0 {
			return true
		}
	}
	return false
}

// oldestQueue pops the oldest ready commit first.
type oldestQueue struct{ commitQueue }

func (q oldestQueue) Less(i, j int) bool { return q.commitQueue.Less(j, i) }

func topoOldestFirst(commits []*object.Commit) []*object.Commit {
	inSet := make(map[plumbing.Hash]bool, len(commits))
	for _, c := range commits {
		inSet[c.Hash] = true
	}
	pending := make(map[plumbing.Hash]int, len(commits))
	children := make(map[plumbing.Hash][]*object.Commit)
	ready := &oldestQueue{}
	for _, c := range commits {
		for _, p := range c.ParentHashes {
			if inSet[p] {
				pending[c.Hash]++
				children[p] = append(children[p], c)
			}
		}
		if pending[c.Hash] == 0 {
			heap.Push(ready, c)
		}
	}
	ordered := make([]*object.Commit, 0, len(commits))
	for ready.Len() > 0 {
		c := heap.Pop(ready).(*object.Commit)
		ordered = append(ordered, c)
		for _, child := range children[c.Hash] {
			pending[child.Hash]--
			if pending[child.Hash] == 0 {
				heap.Push(ready, child)
			}
		}
	}
	return ordered
}
```

Note: `oldestQueue` embeds `commitQueue`; the embedded `Push`/`Pop` have pointer receivers on `*commitQueue`, so give `oldestQueue` its own `Push`/`Pop` forwarding to `q.commitQueue.Push/Pop` with a `*oldestQueue` receiver if the compiler rejects `heap.Push(ready, …)`. Keep the behaviour: oldest first.

- [ ] **Step 2: Implement the public API** in `commits.go`.

```go
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git/repolock"
)

// (types and CommitRangeLimit / ErrCommitNotInRange from the Interfaces block)

func ListCommitRange(locks *repolock.Locks, req CommitRangeRequest) (CommitRange, error) {
	var out CommitRange
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		commits, truncated, err := cappedRange(req)
		if err != nil {
			return err
		}
		out.Truncated = truncated
		out.Commits = make([]RangeCommit, 0, len(commits))
		for _, c := range commits {
			rc := toRangeCommit(c)
			if req.WithDiffs && !rc.IsMerge {
				d, err := commitDiff(c)
				if err != nil {
					return err
				}
				rc.Diff = &d
			}
			out.Commits = append(out.Commits, rc)
		}
		return nil
	})
	return out, err
}

func GetRangeCommit(locks *repolock.Locks, req CommitRangeRequest, sha string) (RangeCommit, error) {
	var out RangeCommit
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		commits, _, err := cappedRange(req)
		if err != nil {
			return err
		}
		for _, c := range commits {
			if c.Hash.String() != sha {
				continue
			}
			out = toRangeCommit(c)
			if !out.IsMerge {
				d, err := commitDiff(c)
				if err != nil {
					return err
				}
				out.Diff = &d
			}
			return nil
		}
		return ErrCommitNotInRange
	})
	return out, err
}

func cappedRange(req CommitRangeRequest) ([]*object.Commit, bool, error) {
	repo, err := gitlib.PlainOpen(req.RepoPath)
	if err != nil {
		return nil, false, fmt.Errorf("open repo %s: %w", req.RepoPath, err)
	}
	commits, err := rangeCommits(repo, req.HeadSHA, req.MergeBaseSHA)
	if err != nil {
		return nil, false, err
	}
	if len(commits) > CommitRangeLimit {
		return commits[:CommitRangeLimit], true, nil
	}
	return commits, false, nil
}

func toRangeCommit(c *object.Commit) RangeCommit {
	subject, body, _ := strings.Cut(strings.TrimRight(c.Message, "\n"), "\n")
	parents := make([]string, len(c.ParentHashes))
	for i, p := range c.ParentHashes {
		parents[i] = p.String()
	}
	return RangeCommit{
		SHA: c.Hash.String(), ParentSHAs: parents,
		Subject: strings.TrimSpace(subject), Body: strings.TrimSpace(body),
		AuthorName: c.Author.Name, AuthoredAt: c.Author.When,
		IsMerge: len(c.ParentHashes) > 1,
	}
}

func commitDiff(c *object.Commit) (CommitDiff, error) {
	tree, err := c.Tree()
	if err != nil {
		return CommitDiff{}, fmt.Errorf("load tree of %s: %w", c.Hash, err)
	}
	parentTree := &object.Tree{} // root commit: diff against the empty tree
	if len(c.ParentHashes) > 0 {
		parent, err := c.Parent(0)
		if err != nil {
			return CommitDiff{}, fmt.Errorf("load parent of %s: %w", c.Hash, err)
		}
		if parentTree, err = parent.Tree(); err != nil {
			return CommitDiff{}, fmt.Errorf("load parent tree of %s: %w", c.Hash, err)
		}
	}
	changes, err := object.DiffTree(parentTree, tree)
	if err != nil {
		return CommitDiff{}, fmt.Errorf("diff %s: %w", c.Hash, err)
	}
	patch, err := changes.Patch()
	if err != nil {
		return CommitDiff{}, fmt.Errorf("patch %s: %w", c.Hash, err)
	}
	result, err := renderDiffFiles(patch, DefaultOptions())
	if err != nil {
		return CommitDiff{}, err
	}
	d := CommitDiff{Files: result.Files, Unified: unifiedPatch(result.Files), PatchID: commitPatchID(result.Files)}
	for _, f := range result.Files {
		d.Additions += f.AddedLines
		d.Deletions += f.RemovedLines
	}
	return d, nil
}

// unifiedPatch joins file patches; empty (binary/truncated) patches get a
// header so the file stays visible — mirrors prdiff.Store.GetUnified.
func unifiedPatch(files []*File) string {
	var b strings.Builder
	for _, f := range files {
		if !strings.HasPrefix(f.Patch, "diff --git ") {
			oldPath := f.Path
			if f.OldPath != "" {
				oldPath = f.OldPath
			}
			fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldPath, f.Path)
		}
		b.WriteString(f.Patch)
		if f.Patch != "" && !strings.HasSuffix(f.Patch, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func commitPatchID(files []*File) string {
	ids := make([]string, len(files))
	for i, f := range files {
		ids[i] = f.PatchID
	}
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])
}
```

Check `errors` import is used (it is, by `ErrCommitNotInRange`); drop `time` if the struct lives elsewhere.

- [ ] **Step 3: Add behavioural tests** in `commits_test.go`. Build fixtures with explicit, increasing commit times (do not use `gittest.CommitFile`, it stamps `time.Now()` and ties break ordering).

```go
package diff

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git/repolock"
)

type fixture struct {
	t    *testing.T
	path string
	repo *gitlib.Repository
	tick time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	path := t.TempDir()
	repo, err := gitlib.PlainInit(path, false)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, path: path, repo: repo, tick: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// commit writes file=content and commits; parents nil means HEAD.
func (f *fixture) commit(file, content, msg string, parents ...plumbing.Hash) plumbing.Hash {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.path, file), []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	wt, err := f.repo.Worktree()
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := wt.Add(file); err != nil {
		f.t.Fatal(err)
	}
	f.tick = f.tick.Add(time.Minute)
	sig := &object.Signature{Name: "Bistro Test", Email: "t@example.com", When: f.tick}
	hash, err := wt.Commit(msg, &gitlib.CommitOptions{Author: sig, Committer: sig, Parents: parents})
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

func (f *fixture) reset(to plumbing.Hash) {
	f.t.Helper()
	wt, _ := f.repo.Worktree()
	if err := wt.Reset(&gitlib.ResetOptions{Commit: to, Mode: gitlib.HardReset}); err != nil {
		f.t.Fatal(err)
	}
}

func shas(r CommitRange) []string {
	out := make([]string, len(r.Commits))
	for i, c := range r.Commits {
		out[i] = c.SHA
	}
	return out
}

func TestListCommitRange_ExcludesBaseHistoryIncludingMergedMain(t *testing.T) {
	f := newFixture(t)
	b0 := f.commit("base.txt", "base\n", "base")
	m1 := f.commit("main.txt", "main\n", "main work")
	f.reset(b0)
	f1 := f.commit("a.txt", "a\n", "feat: a\n\nbody line")
	f2 := f.commit("b.txt", "b\n", "feat: b")
	mg := f.commit("main.txt", "main\n", "Merge main", f2, m1)
	f3 := f.commit("c.txt", "c\n", "feat: c")

	got, err := ListCommitRange(repolock.New(), CommitRangeRequest{RepoURL: "u", RepoPath: f.path, HeadSHA: f3.String(), MergeBaseSHA: m1.String(), WithDiffs: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{f1.String(), f2.String(), mg.String(), f3.String()}
	if g := shas(got); len(g) != len(want) || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] || g[3] != want[3] {
		t.Fatalf("range = %v, want %v", g, want)
	}
	if got.Commits[0].Subject != "feat: a" || got.Commits[0].Body != "body line" {
		t.Fatalf("message split = %q / %q", got.Commits[0].Subject, got.Commits[0].Body)
	}
	if !got.Commits[2].IsMerge || got.Commits[2].Diff != nil {
		t.Fatalf("merge commit = %+v, want IsMerge with nil Diff", got.Commits[2])
	}
	if d := got.Commits[0].Diff; d == nil || d.Additions != 1 || len(d.Files) != 1 || d.Files[0].Path != "a.txt" {
		t.Fatalf("first commit diff = %+v, want a.txt +1", d)
	}
}

func TestListCommitRange_CapsAtLimitKeepingOldest(t *testing.T) {
	f := newFixture(t)
	base := f.commit("f.txt", "0\n", "base")
	var first plumbing.Hash
	var head plumbing.Hash
	for i := 1; i <= CommitRangeLimit+5; i++ {
		head = f.commit("f.txt", fmt.Sprintf("%d\n", i), fmt.Sprintf("c%d", i))
		if i == 1 {
			first = head
		}
	}
	got, err := ListCommitRange(repolock.New(), CommitRangeRequest{RepoURL: "u", RepoPath: f.path, HeadSHA: head.String(), MergeBaseSHA: base.String()})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Commits) != CommitRangeLimit || got.Commits[0].SHA != first.String() {
		t.Fatalf("truncated=%v len=%d first=%s, want true/%d/%s", got.Truncated, len(got.Commits), got.Commits[0].SHA, CommitRangeLimit, first)
	}
}

func TestGetRangeCommit_RejectsCommitOutsideRange(t *testing.T) {
	f := newFixture(t)
	base := f.commit("f.txt", "0\n", "base")
	head := f.commit("f.txt", "1\n", "change")
	req := CommitRangeRequest{RepoURL: "u", RepoPath: f.path, HeadSHA: head.String(), MergeBaseSHA: base.String()}
	if _, err := GetRangeCommit(repolock.New(), req, base.String()); !errors.Is(err, ErrCommitNotInRange) {
		t.Fatalf("base commit err = %v, want ErrCommitNotInRange", err)
	}
	got, err := GetRangeCommit(repolock.New(), req, head.String())
	if err != nil || got.Diff == nil || got.Diff.Additions != 1 || got.Diff.Deletions != 1 {
		t.Fatalf("head commit = %+v, %v; want +1 -1", got.Diff, err)
	}
}
```

Add `errors` and `fmt` to the test imports.

- [ ] **Step 4: Verify**

Run: `go vet ./internal/git/... && go test -race ./internal/git/diff/`
Expected: `ok  github.com/itterno/bistro/internal/git/diff`

- [ ] **Step 5: Commit**

```bash
git add internal/git/diff/commits.go internal/git/diff/commits_walk.go internal/git/diff/commits_test.go
git commit -m "feat: list PR commit range and per-commit diffs"
```

---

### Task 2: `commits_viewed` table and `commitviewed` store

**Files:**
- Create: `internal/store/migrations/00038_add_commits_viewed.sql`
- Create: `internal/store/queries/commits_viewed.sql`
- Generated: `internal/store/db/commits_viewed.sql.go`, `internal/store/db/models.go` (via sqlc)
- Create: `internal/store/commitviewed/store.go`
- Test: `internal/store/commitviewed/store_test.go`

**Interfaces:**
- Consumes: `store.LocalReviewAccountProvider` (`"github"`), `storedb.New`.
- Produces (used by Task 3):

```go
package commitviewed
func New(db *sql.DB) *Store
// List returns patch_key → stored occurrences for login on prID.
func (s *Store) List(ctx context.Context, prID, login string) (map[string]int, error)
func (s *Store) Mark(ctx context.Context, prID, login, patchKey string, occurrences int) error
func (s *Store) Unmark(ctx context.Context, prID, login, patchKey string) error
```

- [ ] **Step 1: Migration** `00038_add_commits_viewed.sql`:

```sql
-- +goose Up
-- Migration: 00038_add_commits_viewed
-- Description: Local per-account "viewed" marks for commits in the
-- commit-by-commit review view. patch_key is a content fingerprint plus an
-- occurrence ordinal; occurrences is the fingerprint's count in the PR range
-- when marked (a mismatch means a rewrite touched duplicates: unviewed).
CREATE TABLE IF NOT EXISTS commits_viewed (
    pr_id            TEXT      NOT NULL,
    patch_key        TEXT      NOT NULL,
    occurrences      INTEGER   NOT NULL CHECK (occurrences > 0),
    account_provider TEXT      NOT NULL DEFAULT 'github',
    account_login    TEXT      NOT NULL DEFAULT '',
    login            TEXT      NOT NULL,
    viewed_at        TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, patch_key, account_provider, account_login, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS commits_viewed;
```

- [ ] **Step 2: Queries** `commits_viewed.sql`:

```sql
-- name: UpsertCommitViewed :exec
INSERT INTO commits_viewed (pr_id, patch_key, occurrences, account_provider, account_login, login, viewed_at)
VALUES (sqlc.arg(pr_id), sqlc.arg(patch_key), sqlc.arg(occurrences), sqlc.arg(account_provider), sqlc.arg(account_login), sqlc.arg(login), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
ON CONFLICT(pr_id, patch_key, account_provider, account_login, login) DO UPDATE SET
    occurrences = excluded.occurrences,
    viewed_at = excluded.viewed_at;

-- name: DeleteCommitViewed :exec
DELETE FROM commits_viewed
WHERE pr_id = sqlc.arg(pr_id) AND patch_key = sqlc.arg(patch_key)
  AND account_provider = sqlc.arg(account_provider) AND account_login = sqlc.arg(account_login) AND login = sqlc.arg(login);

-- name: ListCommitsViewed :many
SELECT patch_key, occurrences
FROM commits_viewed
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider) AND account_login = sqlc.arg(account_login) AND login = sqlc.arg(login)
ORDER BY patch_key;
```

Run from repo root: `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`. Read the generated param struct names in `internal/store/db/commits_viewed.sql.go` before writing the store.

- [ ] **Step 3: Store** `internal/store/commitviewed/store.go` (pattern: `internal/store/reviewdraft/store.go`):

```go
// Package commitviewed stores local per-account viewed marks for PR commits.
package commitviewed

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/itterno/bistro/internal/store"
	storedb "github.com/itterno/bistro/internal/store/db"
)

// Store persists commit viewed marks.
type Store struct{ queries *storedb.Queries }

// New creates a commit viewed store backed by db.
func New(db *sql.DB) *Store { return &Store{queries: storedb.New(db)} }

// List returns patch_key → occurrences recorded for login on prID.
func (s *Store) List(ctx context.Context, prID, login string) (map[string]int, error) {
	rows, err := s.queries.ListCommitsViewed(ctx, storedb.ListCommitsViewedParams{PrID: prID, AccountProvider: store.LocalReviewAccountProvider, AccountLogin: login, Login: login})
	if err != nil {
		return nil, fmt.Errorf("list commits viewed (%s, %s): %w", prID, login, err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.PatchKey] = int(row.Occurrences)
	}
	return out, nil
}

// Mark records patchKey viewed with the fingerprint's current occurrence count.
func (s *Store) Mark(ctx context.Context, prID, login, patchKey string, occurrences int) error {
	if err := s.queries.UpsertCommitViewed(ctx, storedb.UpsertCommitViewedParams{PrID: prID, PatchKey: patchKey, Occurrences: int64(occurrences), AccountProvider: store.LocalReviewAccountProvider, AccountLogin: login, Login: login}); err != nil {
		return fmt.Errorf("mark commit viewed (%s, %s): %w", prID, patchKey, err)
	}
	return nil
}

// Unmark removes the viewed mark; a missing mark is not an error.
func (s *Store) Unmark(ctx context.Context, prID, login, patchKey string) error {
	if err := s.queries.DeleteCommitViewed(ctx, storedb.DeleteCommitViewedParams{PrID: prID, PatchKey: patchKey, AccountProvider: store.LocalReviewAccountProvider, AccountLogin: login, Login: login}); err != nil {
		return fmt.Errorf("unmark commit viewed (%s, %s): %w", prID, patchKey, err)
	}
	return nil
}
```

Adjust field names to whatever sqlc generated. If importing the root `store` package from here creates a cycle (it must not — subpackages may import `store`), stop and report.

- [ ] **Step 4: Round-trip test** `store_test.go`. The FK needs a `prs` row: look at how `internal/store/reviewdraft/store_test.go` or other `storetest.Open` users insert a minimal PR (search `INSERT INTO prs` under `internal/store`), and reuse that minimal insert.

```go
func TestCommitsViewed_MarkListUnmark_ScopedToLogin(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	insertPR(t, db, "pr-1") // minimal prs row, copied from an existing test helper
	s := New(db)
	if err := s.Mark(ctx, "pr-1", "me", "abc:0", 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark(ctx, "pr-1", "other", "abc:0", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark(ctx, "pr-1", "me", "abc:0", 3); err != nil { // upsert updates occurrences
		t.Fatal(err)
	}
	got, err := s.List(ctx, "pr-1", "me")
	if err != nil || len(got) != 1 || got["abc:0"] != 3 {
		t.Fatalf("List(me) = %v, %v; want {abc:0: 3}", got, err)
	}
	if err := s.Unmark(ctx, "pr-1", "me", "abc:0"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(ctx, "pr-1", "me"); len(got) != 0 {
		t.Fatalf("after Unmark = %v, want empty", got)
	}
	if got, _ := s.List(ctx, "pr-1", "other"); got["abc:0"] != 1 {
		t.Fatalf("other login = %v, want untouched", got)
	}
}
```

- [ ] **Step 5: Verify**

Run: `go vet ./internal/store/... && go test -race ./internal/store/...`
Expected: all `ok`, including any migration-count tests (update expected migration counts if a test pins them).

- [ ] **Step 6: Commit**

```bash
git add internal/store/migrations/00038_add_commits_viewed.sql internal/store/queries/commits_viewed.sql internal/store/db internal/store/commitviewed
git commit -m "feat: store local commit viewed marks"
```

---

### Task 3: `Commits` service, engine wiring, bindings

**Files:**
- Create: `internal/services/commits.go`
- Create: `internal/services/commits_keys.go` (viewed-key derivation, pure)
- Modify: `internal/services/set.go` (add `Commits *Commits`)
- Modify: `internal/engine/services.go` (construct it)
- Modify: `main.go` (register after `set.Interdiff`)
- Generated: `frontend/bindings/**` via `wails3 task generate`
- Modify: `AGENTS.md` (architecture notes)
- Test: `internal/services/commits_test.go`

**Interfaces:**
- Consumes: Task 1 `diff.CommitRangeRequest`, `diff.CommitRange`, `diff.RangeCommit`, `diff.ListCommitRange`, `diff.GetRangeCommit`, `diff.ErrCommitNotInRange`, `diff.ErrHeadUnavailable`; Task 2 `commitviewed.Store`; existing `interdiffPatchsets`, `currentUserReader`, `guard`, `Errorf`, `WithReason`, `NormalizeID`, `contextError`, `githubProvider`, `PRRef`, `logging.FromContext`; `git.MergeBase(localPath, baseSHA, headSHA string) (string, error)`.
- Produces (bound to the renderer; JSON names are the contract for Tasks 4–5):

```go
type Commits struct {
	Patchsets   interdiffPatchsets
	Range       func(diff.CommitRangeRequest) (diff.CommitRange, error)
	Commit      func(diff.CommitRangeRequest, string) (diff.RangeCommit, error)
	MergeBase   func(repoPath, baseSHA, headSHA string) (string, error)
	RepoForPR   func(context.Context, string) (string, string, bool, error)
	Viewed      commitViewedStore
	CurrentUser currentUserReader
	Panics      PanicReporter
	Log         *slog.Logger
}
type CommitSummary struct {
	SHA        string    `json:"sha"`
	ParentSHAs []string  `json:"parent_shas"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	AuthorName string    `json:"author_name"`
	AuthoredAt time.Time `json:"authored_at"`
	IsMerge    bool      `json:"is_merge"`
	Additions  int       `json:"additions"`
	Deletions  int       `json:"deletions"`
	FileCount  int       `json:"file_count"`
	Viewed     bool      `json:"viewed"`
}
type CommitsResponse struct {
	PRID      string          `json:"pr_id"`
	Available bool            `json:"available"`
	Reason    string          `json:"reason"`
	Truncated bool            `json:"truncated"`
	Commits   []CommitSummary `json:"commits"`
}
type CommitRequest struct {
	PRID string `json:"pr_id"`
	SHA  string `json:"sha"`
}
type CommitDiffResponse struct {
	PRID      string `json:"pr_id"`
	SHA       string `json:"sha"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Diff      string `json:"diff"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}
type SetCommitViewedRequest struct {
	PRID   string `json:"pr_id"`
	SHA    string `json:"sha"`
	Viewed bool   `json:"viewed"`
}
type CommitViewed struct {
	PRID   string `json:"pr_id"`
	SHA    string `json:"sha"`
	Viewed bool   `json:"viewed"`
}
func (s *Commits) List(ctx context.Context, req PRRef) (CommitsResponse, error)
func (s *Commits) Get(ctx context.Context, req CommitRequest) (CommitDiffResponse, error)
func (s *Commits) SetViewed(ctx context.Context, req SetCommitViewedRequest) (CommitViewed, error)
const ReasonCommitNotInPR = "commit_not_in_pr"
```

- [ ] **Step 1: Viewed keys** in `commits_keys.go`:

```go
package services

import (
	"fmt"

	"github.com/itterno/bistro/internal/git/diff"
)

// commitViewKey identifies a commit's viewed mark; see the spec's Viewed state.
type commitViewKey struct {
	Key         string
	Occurrences int
}

// commitViewKeys returns one key per commit, aligned with commits. Commits
// without a Diff must be merges (Range is always called WithDiffs here).
func commitViewKeys(commits []diff.RangeCommit) []commitViewKey {
	totals := map[string]int{}
	for _, c := range commits {
		if c.Diff != nil {
			totals[c.Diff.PatchID]++
		}
	}
	seen := map[string]int{}
	keys := make([]commitViewKey, len(commits))
	for i, c := range commits {
		if c.Diff == nil {
			keys[i] = commitViewKey{Key: "merge:" + c.SHA, Occurrences: 1}
			continue
		}
		id := c.Diff.PatchID
		keys[i] = commitViewKey{Key: fmt.Sprintf("%s:%d", id, seen[id]), Occurrences: totals[id]}
		seen[id]++
	}
	return keys
}
```

- [ ] **Step 2: Service** in `commits.go`. Shared resolution step:

```go
type commitViewedStore interface {
	List(ctx context.Context, prID, login string) (map[string]int, error)
	Mark(ctx context.Context, prID, login, patchKey string, occurrences int) error
	Unmark(ctx context.Context, prID, login, patchKey string) error
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// commitRange resolves the PR's range request. A non-empty reason means
// "unavailable"; err is only a classified service error.
func (s *Commits) rangeRequest(ctx context.Context, prID string) (diff.CommitRangeRequest, string, error) {
	logger := logging.FromContext(ctx)
	repoURL, repoPath, ok, err := s.RepoForPR(ctx, prID)
	if errors.Is(err, sql.ErrNoRows) {
		return diff.CommitRangeRequest{}, "", Errorf(CodeNotFound, "PR not found")
	}
	if cerr := contextError(err); cerr != nil {
		return diff.CommitRangeRequest{}, "", cerr
	}
	if err != nil {
		logger.Warn("commits: repo lookup failed", "error", err)
		return diff.CommitRangeRequest{}, "could not locate the repository", nil
	}
	if !ok {
		return diff.CommitRangeRequest{}, "no local copy of this repository yet", nil
	}
	current, found, err := s.Patchsets.CurrentPatchset(ctx, prID)
	if cerr := contextError(err); cerr != nil {
		return diff.CommitRangeRequest{}, "", cerr
	}
	if err != nil {
		logger.Warn("commits: current patchset lookup failed", "error", err)
		return diff.CommitRangeRequest{}, "could not load the PR head", nil
	}
	if !found {
		return diff.CommitRangeRequest{}, "this PR has not been synced yet", nil
	}
	mergeBase := current.MergeBaseSHA
	if mergeBase == "" {
		if mergeBase, err = s.MergeBase(repoPath, current.BaseSHA, current.HeadSHA); err != nil {
			logger.Warn("commits: merge base failed", "error", err)
			return diff.CommitRangeRequest{}, "could not find the merge base", nil
		}
	}
	return diff.CommitRangeRequest{RepoURL: repoURL, RepoPath: repoPath, HeadSHA: current.HeadSHA, MergeBaseSHA: mergeBase}, "", nil
}

// rangeFailure maps a git error to an unavailable reason (or a context error).
func rangeFailure(ctx context.Context, err error) (string, error) {
	if cerr := contextError(err); cerr != nil {
		return "", cerr
	}
	if errors.Is(err, diff.ErrHeadUnavailable) {
		return "the PR head or merge base is not in the local repository", nil
	}
	logging.FromContext(ctx).Warn("commits: range failed", "error", err)
	return "could not list commits", nil
}

func (s *Commits) login(ctx context.Context) (string, error) {
	if s.CurrentUser == nil {
		return "", nil
	}
	login, ok, err := s.CurrentUser.Get(ctx, githubProvider)
	if err != nil || !ok {
		return "", err
	}
	return login, nil
}
```

`List`:

```go
func (s *Commits) List(ctx context.Context, req PRRef) (CommitsResponse, error) {
	return guard(s.Panics, "Commits.List", func() (CommitsResponse, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return CommitsResponse{}, Errorf(CodeInvalid, "pr_id is required")
		}
		resp := CommitsResponse{PRID: prID, Commits: []CommitSummary{}}
		rangeReq, reason, err := s.rangeRequest(ctx, prID)
		if err != nil || reason != "" {
			resp.Reason = reason
			return resp, err
		}
		rangeReq.WithDiffs = true
		commits, err := s.Range(rangeReq)
		if err != nil {
			resp.Reason, err = rangeFailure(ctx, err)
			return resp, err
		}
		login, err := s.login(ctx)
		if err != nil {
			return CommitsResponse{}, err
		}
		stored := map[string]int{}
		if login != "" {
			if stored, err = s.Viewed.List(ctx, prID, login); err != nil {
				return CommitsResponse{}, err
			}
		}
		keys := commitViewKeys(commits.Commits)
		resp.Available, resp.Truncated = true, commits.Truncated
		for i, c := range commits.Commits {
			summary := CommitSummary{SHA: c.SHA, ParentSHAs: c.ParentSHAs, Subject: c.Subject, Body: c.Body, AuthorName: c.AuthorName, AuthoredAt: c.AuthoredAt, IsMerge: c.IsMerge}
			if c.Diff != nil {
				summary.Additions, summary.Deletions, summary.FileCount = c.Diff.Additions, c.Diff.Deletions, len(c.Diff.Files)
			}
			occurrences, ok := stored[keys[i].Key]
			summary.Viewed = ok && occurrences == keys[i].Occurrences
			resp.Commits = append(resp.Commits, summary)
		}
		return resp, nil
	})
}
```

`Get`: validate `NormalizeID(pr_id)` non-empty and `shaPattern.MatchString(strings.ToLower(req.SHA))` else `Errorf(CodeInvalid, "sha must be a 40-character commit id")`; resolve range (unavailable → `resp.Reason`); call `s.Commit(rangeReq, sha)`; `errors.Is(err, diff.ErrCommitNotInRange)` → `WithReason(CodeInvalid, ReasonCommitNotInPR, "this commit is no longer part of the PR")`; other errors → `rangeFailure`. On success `Available: true`, and when `c.Diff != nil` set `Diff`, `Additions`, `Deletions` (merges stay empty).

`SetViewed`: validate as `Get`; `login, err := s.login(ctx)`; empty login → `Errorf(CodeUnauthenticated, "sign in to mark commits viewed")`; resolve range (an unavailable reason here → `Errorf(CodeInvalid, "%s", reason)` since a write cannot succeed); `Range` with `WithDiffs: true`; find the index of `sha` (missing → `WithReason(CodeInvalid, ReasonCommitNotInPR, …)`); `key := commitViewKeys(commits)[i]`; `Viewed.Mark(ctx, prID, login, key.Key, key.Occurrences)` or `Unmark(ctx, prID, login, key.Key)`; return `CommitViewed{PRID, SHA, Viewed: req.Viewed}`. Git errors other than context → `Errorf(CodeInternal, "could not list commits")` after logging.

- [ ] **Step 3: Wire it.** `set.go`: add `Commits *Commits` after `Interdiff`. `internal/engine/services.go`: add

```go
Commits: &services.Commits{
	Patchsets: a.db,
	Range:     func(req diff.CommitRangeRequest) (diff.CommitRange, error) { return diff.ListCommitRange(a.repoLocks, req) },
	Commit:    func(req diff.CommitRangeRequest, sha string) (diff.RangeCommit, error) { return diff.GetRangeCommit(a.repoLocks, req, sha) },
	MergeBase: git.MergeBase,
	RepoForPR: prRepoLocator{db: a.db, locations: locations}.RepoForPR,
	Viewed:    commitviewed.New(a.db.DB),
	CurrentUser: a.currentUserStore,
	Panics: a.panics, Log: a.log,
},
```

(import `github.com/itterno/bistro/internal/git` and `.../internal/store/commitviewed`; confirm `internal/engine` already may import `internal/git` — it uses `a.gitClient`). `main.go`: `app.RegisterService(application.NewService(set.Commits))` right after `set.Interdiff`. If engine tests assert the full `Set` is non-nil field by field, extend them.

- [ ] **Step 4: Behavioural tests** `commits_test.go` (stubs only for the git/repo edges; the service logic is what's tested). Reuse `mockPatchsetLister` from `interdiff_test.go` if it implements `CurrentPatchset` (read it; if it doesn't return a current patchset usefully, write a small `commitsPatchsetStub`). Use a real `commitviewed.New(storetest.Open(t).DB)` with a `prs` row inserted, and a `currentUserReader` stub returning `"me"`.

Tests (one per behaviour, each would fail if the logic broke):
1. `TestCommitsGet_OutOfRangeSHA_ReturnsInvalidWithReason`: `Commit` stub returns `diff.ErrCommitNotInRange`; assert `*ServiceError` with `Code == CodeInvalid` and `Reason == ReasonCommitNotInPR`. Also a malformed SHA (`"xyz"`) → `CodeInvalid` without calling the stub.
2. `TestCommitsList_NoLocalCopy_Unavailable`: `RepoForPR` returns `ok=false`; assert `Available=false`, `Reason == "no local copy of this repository yet"`, nil error, `Range` never called.
3. `TestCommitsViewed_SurvivesRebaseWithIdenticalContent`: first `Range` returns commit `sha1` with `PatchID "p"`; `SetViewed(sha1, true)`; swap the stub to return `sha2` (different SHA, same `PatchID "p"`); `List` → `Viewed == true`. Swap to `sha3` with `PatchID "q"` → `Viewed == false`.
4. `TestCommitsViewed_DuplicatesStaySeparateAndResetOnRewrite`: range `[a(p), b(p)]`; mark `a`; `List` → a viewed, b not. Rewrite range to `[b'(p)]` only; `List` → `b'` not viewed (stored occurrences 2 ≠ current 1).

- [ ] **Step 5: Generate bindings** from repo root: `wails3 task generate`. Confirm `frontend/bindings/github.com/itterno/bistro/internal/services/` now has `Commits` methods and the models above; `git status` must show only expected generated changes.

- [ ] **Step 6: Docs.** In `AGENTS.md` (edit `AGENTS.md`, never the `CLAUDE.md` symlink):
  - Architecture → Independent SQLite stores sentence: add `commitviewed` to the list (`currentuser`, `commitviewed`, `dismissal`, …).
  - After the interdiff mention in "Within `internal/git`", add: "`diff` also lists a PR's `merge-base..head` commit range (capped at 250) and diffs one commit against its first parent, under the shared repo lock; `services.Commits` serves it with occurrence-aware patch-id viewed keys."

- [ ] **Step 7: Verify**

Run: `go vet ./... && go test -race ./...`
Expected: all packages `ok`.

- [ ] **Step 8: Commit**

```bash
git add internal/services/commits.go internal/services/commits_keys.go internal/services/commits_test.go internal/services/set.go internal/engine/services.go main.go frontend/bindings AGENTS.md
git commit -m "feat: add Commits service for commit-by-commit review"
```

---

### Task 4: Renderer data, routes, and the two pages

**Files:**
- Modify: `frontend/src/types/index.ts` (re-export `CommitSummary`, `CommitsResponse as CommitsResult`, `CommitDiffResponse as CommitDiffResult`, `CommitViewed as CommitViewedAck` from `@bindings/services`)
- Create: `frontend/src/services/commits.ts`
- Create: `frontend/src/utils/commitNavigation.ts`
- Test: `frontend/src/utils/commitNavigation.test.ts`
- Modify: `frontend/src/store/ui.ts` (add `commitShas: string[]`, `selectedCommitIndex: number`, defaults `[]` / `0`)
- Create: `frontend/src/pages/useCommitsData.ts`, `frontend/src/pages/CommitsPage.tsx`
- Create: `frontend/src/pages/useCommitData.ts`, `frontend/src/pages/CommitPage.tsx`
- Modify: `frontend/src/router.tsx` (two routes + header comment tree)
- Modify: `frontend/src/utils/keybindings.ts` (`RouteBase` + `ALL_ROUTES` get `'commits'`, `'commit'`)
- Modify: `frontend/src/utils/keybindingActivation.ts` (`routeBaseFromPath`)

**Interfaces:**
- Consumes: Task 3 bindings `Commits.List({pr_id})`, `Commits.Get({pr_id, sha})`, `Commits.SetViewed({pr_id, sha, viewed})`; `call`, `callWrite`, `faultReason` from `services/engine/call.ts`; `addToast`; `DiffView` props (`diff`, `theme`, `diffStyle`, `selectedFileKey`, `onFilesResolved`, `syncFileSelection`, `findQuery`).
- Produces (used by Task 5):

```ts
// utils/commitNavigation.ts
export interface CommitPosition { index: number; total: number; prevSha: string | null; nextSha: string | null }
export function commitPosition(shas: readonly string[], sha: string): CommitPosition | null;
export function shaFromCommitPath(pathname: string): string | null; // '/prs/x/commits/<sha>' → sha
// services/commits.ts
export function loadCommits(prId: string): ResultAsync<CommitsResult, Fault>;
export function loadCommitDiff(prId: string, sha: string): ResultAsync<CommitDiffResult, Fault>;
export function setCommitViewed(prId: string, sha: string, viewed: boolean): Promise<CommitViewedAck | null>;
// DOM events the pages listen for (dispatched by Task 5 handlers via emitAppEvent):
export const COMMIT_TOGGLE_VIEWED_EVENT = 'bistro:commit:toggle-viewed';
export const COMMIT_VIEWED_AND_NEXT_EVENT = 'bistro:commit:viewed-and-next';
// RouteBase: 'commits' | 'commit'
// UIState: commitShas (list order, written by both pages), selectedCommitIndex (list cursor)
```

Routes: `/prs/$prId/commits` (`CommitsPage`) and `/prs/$prId/commits/$sha` (`CommitPage`, `params.parse` lowercases `prId` and `sha`, `validateSearch: diffSearchSchema` so `?file=` drives DiffView selection exactly as on the diff page). `routeBaseFromPath`: `/^\/prs\/[^/]+\/commits$/` → `'commits'`, `/^\/prs\/[^/]+\/commits\/[^/]+$/` → `'commit'`. Put the event-name constants in `utils/commitNavigation.ts` (pure, importable by handlers without importing pages).

- [ ] **Step 1: Pure helper** `utils/commitNavigation.ts`:

```ts
export const COMMIT_TOGGLE_VIEWED_EVENT = 'bistro:commit:toggle-viewed';
export const COMMIT_VIEWED_AND_NEXT_EVENT = 'bistro:commit:viewed-and-next';

export interface CommitPosition {
  index: number;
  total: number;
  prevSha: string | null;
  nextSha: string | null;
}

/** Where `sha` sits in the oldest-first list; null when it is not listed. */
export function commitPosition(
  shas: readonly string[],
  sha: string,
): CommitPosition | null {
  const index = shas.indexOf(sha);
  if (index < 0) return null;
  return {
    index,
    total: shas.length,
    prevSha: shas[index - 1] ?? null,
    nextSha: shas[index + 1] ?? null,
  };
}

const COMMIT_PATH = /^\/prs\/[^/]+\/commits\/([^/]+)$/;

export function shaFromCommitPath(pathname: string): string | null {
  return COMMIT_PATH.exec(pathname)?.[1] ?? null;
}
```

Test `commitNavigation.test.ts`: middle commit gives both neighbours; first has `prevSha: null`; last has `nextSha: null`; unknown SHA returns `null`; `shaFromCommitPath('/prs/p/commits/abc')` is `'abc'` and `'/prs/p/commits'` is `null`.

- [ ] **Step 2: Request module** `services/commits.ts` (pattern: `services/readState.ts`):

```ts
import { Commits } from '@bindings/services';
import type { CommitDiffResult, CommitsResult, CommitViewedAck } from '../types';
import { createLogger } from '../utils/logger';
import { call, callWrite, faultReason } from './engine/call';
import { addToast } from './toasts';

const log = createLogger('commits');

export function loadCommits(prId: string) {
  return call(() => Commits.List({ pr_id: prId }));
}

export function loadCommitDiff(prId: string, sha: string) {
  return call(() => Commits.Get({ pr_id: prId, sha }));
}

/** Returns the ack, or null after toasting the reason (UI rolls back). */
export function setCommitViewed(
  prId: string,
  sha: string,
  viewed: boolean,
): Promise<CommitViewedAck | null> {
  return callWrite(() => Commits.SetViewed({ pr_id: prId, sha, viewed })).match(
    (ack) => ack,
    (fault) => {
      log.warn({ err: fault.message, sha }, 'failed to set commit viewed');
      const reason = faultReason(fault);
      addToast({
        type: 'warning',
        message: reason
          ? `Could not update viewed: ${reason}`
          : 'Could not update viewed: network or communication error',
        dismissible: true,
      });
      return null;
    },
  );
}
```

Check the exact `call`/`callWrite` return types in `call.ts` and annotate `loadCommits`/`loadCommitDiff` with explicit `ResultAsync<…, Fault>` return types if Biome requires explicit return types.

- [ ] **Step 3: Hooks.** `useCommitsData(prId)` loads via `loadCommits` on mount/`prId` change (cancel late responses with an `isActive` flag, as `hooks/useInterdiff.ts` does), exposes `{ result, loading, error, toggleViewed(sha) }`. `toggleViewed` flips the commit's `viewed` locally first, calls `setCommitViewed`, and restores the previous value when it resolves `null`. On every successful load it writes `appStore` `commitShas` (list order) and clamps `selectedCommitIndex`. `useCommitData(prId, sha)` loads `loadCommits` (for header data, position and viewed) and `loadCommitDiff` in parallel, exposes `{ commit, position, diff, loading, error, notInPr, toggleViewed }`. On every successful list load it also writes `appStore` `commitShas` and sets `selectedCommitIndex` to this commit's index, so `]`/`[` work on a direct visit and never act on another PR's list; `notInPr` is true when the diff fault's `faultReason` is `'commit_not_in_pr'`. Both hooks listen for `COMMIT_TOGGLE_VIEWED_EVENT` (list: selected commit; commit page: current commit) and the commit page also for `COMMIT_VIEWED_AND_NEXT_EVENT` (if already viewed, navigate to `position.nextSha` at once; otherwise await `setCommitViewed(…, true)` and navigate only when it returns a non-null ack — on failure stay on the commit with the rolled-back state and the toast `setCommitViewed` already shows; on the last commit mark viewed and stay put). Listener pattern: `document.addEventListener` in a `useEffect`, as in `hooks/useDismissals.ts`.

- [ ] **Step 4: Pages.** Use design tokens and the mono/terminal style of `DiffPageChrome.tsx` and `PRListTable.tsx`; no raw hex, no magic spacing where a token exists. Read `docs/DESIGN.md` first.
  - `CommitsPage`: toolbar with back-to-PR and "N of M viewed"; truncation notice "Showing the first 250 commits; review the rest in the combined diff" with a link to `/prs/$prId/diff`; when `available` is false, the reason in a centered message like `InterdiffPane`'s `PaneMessage`; rows (short SHA = first 7 chars, subject, author, relative date via the project's existing date util — search `utils/` for one, `+additions −deletions`, `N files`, `StatusSquare` or equivalent for viewed, a `merge` tag). The selected row is `selectedCommitIndex`; clicking a row opens it.
  - `CommitPage`: header with short SHA, subject, author, date, `commit {index+1}/{total}`, viewed state, and the body (preformatted, wrapped); then either the merge note ("Merge commit — its diff is not shown. Edits made while resolving conflicts live only in the merge, so they appear in the combined diff but in none of the listed commits. Review them there." + link to `/prs/$prId/diff`), the `notInPr` message ("This commit is no longer part of the PR." + link to the list), the unavailable reason, or `<DiffView diff={diff} theme diffStyle selectedFileKey={search.file} onFilesResolved={…} syncFileSelection />` plus `prBrowserUrl={prBrowserUrl(pr)}` and `headSha={sha}` so `diff.openFileBrowser` opens the file at this commit (read how `DiffView` builds that URL from `headSha`; load the PR the way `DiffPage` does). Pass no threads, comments, drafts, viewedPaths, or prId (so line-comment UI stays off — verify `DiffView` hides drafts/line comments when `prId` is undefined; if not, report it rather than hacking around it). `onFilesResolved` must store `diffFileKeys` the same way `DiffPage` does — read `DiffPage.tsx`'s `handleFilesResolved` and reuse its store write.

- [ ] **Step 5: Verify**

Run: `cd frontend && pnpm type-check && pnpm lint && pnpm test`
Expected: all pass, including `commitNavigation.test.ts`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "feat: add commit list and commit diff pages"
```

---

### Task 5: Keyboard bindings, palette, help, docs

**Files:**
- Modify: `frontend/src/utils/keybindings.ts` (catalog entries; activation constants)
- Modify: `frontend/src/utils/keymaps.ts` (vim / emacs / standard keys)
- Modify: `frontend/src/utils/keybindingActions.ts` (handlers; escape chain; `selectDiffFile` on `'commit'`)
- Modify: `frontend/src/components/commandPaletteCommands.ts` (an "Open commits" entry)
- Modify: `frontend/src/utils/keybindings.ts` `helpSourceFor` (map `'commits'` → `'detail'`, `'commit'` → `'diff'`) only if the help overlay needs it to list the new bindings — read `HelpOverlay.tsx` first
- Test: extend the existing keybinding activation / conflict test file (find it: `utils/keybinding*.test.ts`)
- Modify: `website/src/pages/docs/keybindings.mdx`

**Interfaces:**
- Consumes: Task 4 `RouteBase` `'commits' | 'commit'`, `commitPosition`, `shaFromCommitPath`, `COMMIT_TOGGLE_VIEWED_EVENT`, `COMMIT_VIEWED_AND_NEXT_EVENT`, `UIState.commitShas`, `UIState.selectedCommitIndex`; `emitAppEvent`, `router`.
- Produces: action ids `detail.commits`, `diff.commits`, `commits.down`, `commits.up`, `commits.open`, `commit.next`, `commit.prev`, `commit.toggleViewed`, `commit.viewedAndNext`.

- [ ] **Step 1: Catalog** (`DETAIL_BINDINGS` neighbourhood). Add `const COMMITS = { routes: ['commits'] } as const; const COMMIT = { routes: ['commit'] } as const; const COMMIT_VIEWS = { routes: ['commits', 'commit'] } as const;` and entries:

| actionId | description | activation |
|---|---|---|
| `detail.commits` | Commits: review one at a time | `DETAIL` |
| `diff.commits` | Commits: review one at a time | `DIFF` |
| `commits.down` | Next commit | `COMMITS` |
| `commits.up` | Previous commit | `COMMITS` |
| `commits.open` | Open commit | `COMMITS` |
| `commit.next` | Next commit | `COMMIT` |
| `commit.prev` | Previous commit | `COMMIT` |
| `commit.toggleViewed` | Mark commit viewed | `COMMIT_VIEWS` |
| `commit.viewedAndNext` | Mark viewed, next commit | `COMMIT` |

Extend activation of `detail.scroll*`, `detail.pageDown`, `detail.pageUp`, `detail.scrollTop`, `detail.scrollBottom` (currently `SCROLL`) to include `'commit'`, and of `diff.nextFile`, `diff.prevFile`, `diff.toggleStyle`, `diff.toggleCollapse`, `diff.toggleWhitespace`, `diff.openFileBrowser` to `['diff', 'commit']`. Do not extend comment, draft, review, interdiff, since-last-review, or markViewed actions. `review.toggleResolve` stays on `SCROLL`: change `SCROLL` into a new constant rather than editing it if other actions share it.

- [ ] **Step 2: Keys.** vim: `detail.commits` / `diff.commits` `['g', 'c']`, `commits.down` `j`, `commits.up` `k`, `commits.open` `Enter`, `commit.next` `]`, `commit.prev` `[`, `commit.toggleViewed` `v`, `commit.viewedAndNext` `Shift+V`. emacs and standard: choose keys consistent with each style's existing conventions (emacs: `Control+N`/`Control+P` for list moves, `Enter` open, `Control+C` prefixed sequences for commands, e.g. `['Control+C', 'Control+G']` for open commits; standard: `ArrowDown`/`ArrowUp`/`Enter`, `Alt+]`/`Alt+[` or similar). Every chosen key must pass `utils/keybindingConflicts.ts` against bindings active on the same routes (`isBindingEnabled` gates) — run the conflict test after adding.

- [ ] **Step 3: Handlers** in `ACTIONS`:

```ts
'detail.commits': openCommits,
'diff.commits': openCommits,
'commits.down': (context) => moveCommitCursor(context, 1),
'commits.up': (context) => moveCommitCursor(context, -1),
'commits.open': openSelectedCommit,
'commit.next': (context) => stepCommit(context, 'next'),
'commit.prev': (context) => stepCommit(context, 'prev'),
'commit.toggleViewed': () => emitAppEvent(COMMIT_TOGGLE_VIEWED_EVENT),
'commit.viewedAndNext': () => emitAppEvent(COMMIT_VIEWED_AND_NEXT_EVENT),
```

```ts
function openCommits(context: KeyActionContext): void {
  if (context.prId === undefined) return;
  void router.navigate({ to: '/prs/$prId/commits', params: { prId: context.prId } });
}

function moveCommitCursor(context: KeyActionContext, delta: 1 | -1): void {
  context.store.setState((state) => ({
    ...state,
    selectedCommitIndex: Math.min(
      Math.max(0, state.selectedCommitIndex + delta),
      Math.max(0, state.commitShas.length - 1),
    ),
  }));
}

function openSelectedCommit(context: KeyActionContext): void {
  const { commitShas, selectedCommitIndex } = context.store.state;
  const sha = commitShas[selectedCommitIndex];
  if (context.prId === undefined || sha === undefined) return;
  void router.navigate({ to: '/prs/$prId/commits/$sha', params: { prId: context.prId, sha }, search: { file: '', side: 'right' } });
}

function stepCommit(context: KeyActionContext, direction: 'next' | 'prev'): void {
  const current = shaFromCommitPath(context.location.pathname);
  if (context.prId === undefined || current === null) return;
  const position = commitPosition(context.store.state.commitShas, current);
  const sha = direction === 'next' ? position?.nextSha : position?.prevSha;
  if (sha === null || sha === undefined) return;
  void router.navigate({ to: '/prs/$prId/commits/$sha', params: { prId: context.prId, sha }, search: { file: '', side: 'right' } });
}
```

Escape (`handleEscape`): before the `'diff'` branch add `routeBase === 'commit'` → navigate to `/prs/$prId/commits`, and `routeBase === 'commits'` → navigate to `/prs/$prId`. `selectDiffFile`: when `context.routeBase === 'commit'`, navigate to `/prs/$prId/commits/$sha` (sha from `shaFromCommitPath`) with `search: { file, side: 'right' }, replace: true` instead of the diff route. Confirm `useKeyActionContext`'s `routePrId` finds `prId` on the new routes (it scans matches for a `prId` param, so it should).

- [ ] **Step 4: Palette.** Add to `COMMANDS` an item `{ id: 'commits', name: 'Review commits one at a time', icon: 'commit', actionId: 'detail.commits' }`. Read how `actionId` items are gated (`isActive`); if a palette item can only name one actionId and must also work on the diff route, add a second item for `diff.commits` with the same name only if the palette dedupes by route — otherwise follow whatever pattern existing dual-route commands use.

- [ ] **Step 5: Tests.** In the existing activation/conflict test file add one focused test: `routeBaseFromPath('/prs/p/commits')` is `'commits'`, `'/prs/p/commits/abc'` is `'commit'`, and `getEffectiveBindings` for each style produces no conflicts on `'commits'` and `'commit'` (use whatever helper the existing conflict test uses for other routes). If an existing test already sweeps all routes in `ALL_ROUTES` for conflicts, rely on it and only add the `routeBaseFromPath` assertions.

- [ ] **Step 6: Docs.** `website/src/pages/docs/keybindings.mdx`: add a "Commits" section following the file's existing table format with the vim/emacs/standard keys chosen in Step 2, and one sentence explaining the commit list, the read-only commit diff, viewed marks surviving rebases with identical content, and that merges and >250-commit PRs point to the combined diff.

- [ ] **Step 7: Verify (full)**

Run from repo root: `go vet ./... && go test -race ./...` then `cd frontend && pnpm type-check && pnpm lint && pnpm test`
Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add frontend/src website/src/pages/docs/keybindings.mdx
git commit -m "feat: keyboard navigation for commit-by-commit review"
```

