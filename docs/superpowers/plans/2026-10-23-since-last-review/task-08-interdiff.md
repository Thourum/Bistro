# Task 8: `REQUEST_INTERDIFF` — per-file interdiff between two patchset heads

**Files:**
- Create: `go/internal/git/interdiff.go`
- Create: `go/internal/git/interdiff_test.go`
- Create: `go/internal/server/handler/interdiff.go`
- Create: `go/internal/server/interdiff_repo.go` (PR → repo adapter)
- Modify: `go/internal/server/app.go` (registration; pre-existing oversized, see README)

**Interfaces:**
- Consumes: `store.Patchset{Number, HeadSHA, MergeBaseSHA}` + `ListPatchsets` (Task 3), `patchsetByNumber` (`handler/patchsets.go`), `filePatchToString` (`git/diff.go`), `repoLockKey`, `truncateSHAForLog` (`git/clone.go`), Task 7's `newForcePushFixture` / `forcePush` / `syncHead` test helpers and `forcePushRepoURL`, `store.RepoLocationStore.Get(ctx, repoID) (*RepoLocation, error)` (nil, nil when missing).
- Produces the WS contract Task 10 consumes:

Request `REQUEST_INTERDIFF`:
```json
{ "pr_id": "github:o/r:1", "path": "a.go", "from_patchset": 3, "to_patchset": 0 }
```
`to_patchset` 0 = latest. `from_patchset` is required (the client passes triage's `from_patchset`).

Response `INTERDIFF_READY` data (always this shape after payload validation):
```json
{
  "pr_id": "github:o/r:1", "path": "a.go", "from_patchset": 3, "to_patchset": 5,
  "available": true, "reason": "", "patch": "diff --git a/a.go b/a.go\n...", "rebased": false
}
```
`patch` is `""` when the file is identical at both heads. `available: false` carries a human-readable `reason` and an empty patch.

**Why:** Spec §5. The patch is the file at the baseline head vs. the file at the current head; `rebased` flags upstream changes to the file between the two merge bases (no subtraction — out of scope).

- [ ] **Step 1: Write the failing tests**

`go/internal/git/interdiff_test.go`:

```go
package git

import (
	"errors"
	"strings"
	"testing"
)

func TestInterdiffAfterForcePush(t *testing.T) {
	fixture := newForcePushFixture(t)
	client := NewClient(NewCache())
	syncHead(t, client, fixture, fixture.head1)
	head2 := forcePush(t, fixture)
	syncHead(t, client, fixture, head2)
	request := InterdiffRequest{
		RepoURL: forcePushRepoURL, RepoPath: fixture.cacheDir, Path: "foo.go",
		FromHead: fixture.head1, ToHead: head2,
		FromMergeBase: fixture.baseSHA, ToMergeBase: fixture.baseSHA,
	}

	result, err := client.Interdiff(request)
	if err != nil {
		t.Fatalf("Interdiff: %v", err)
	}
	if !strings.Contains(result.Patch, "-var x = 1") || !strings.Contains(result.Patch, "+var x = 2") {
		t.Fatalf("patch = %q, want head1 → head2 change", result.Patch)
	}
	if result.Rebased {
		t.Error("Rebased = true with identical merge bases")
	}

	// head1 changed foo.go relative to base, so treating them as the two merge
	// bases means upstream touched the file.
	request.ToMergeBase = fixture.head1
	if result, err = client.Interdiff(request); err != nil || !result.Rebased {
		t.Errorf("upstream-changed file: rebased=%v err=%v, want rebased", result.Rebased, err)
	}

	request.FromHead = strings.Repeat("ab", 20)
	if _, err := client.Interdiff(request); !errors.Is(err, ErrHeadUnavailable) {
		t.Errorf("missing head: err = %v, want ErrHeadUnavailable", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/git/ -run InterdiffAfterForcePush -v`
Expected: FAIL to compile (`undefined: InterdiffRequest`).

- [ ] **Step 3: Implement `Interdiff`**

`go/internal/git/interdiff.go`:

```go
package git

import (
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ErrHeadUnavailable means a patchset head is not in the local repo: it was
// recorded before heads were pinned, or the cache clone was deleted.
var ErrHeadUnavailable = errors.New("patchset head unavailable locally")

// InterdiffRequest names one file at two patchset heads.
type InterdiffRequest struct {
	RepoURL       string // the sync's clone URL; serializes with syncs of the repo
	RepoPath      string
	Path          string
	FromHead      string
	ToHead        string
	FromMergeBase string // "" when unknown
	ToMergeBase   string // "" when unknown
}

// InterdiffResult is the file's change between the two heads.
type InterdiffResult struct {
	Patch   string // "" when the file is identical at both heads
	Rebased bool   // the file also differs between the two merge bases
}

// Interdiff diffs req.Path from FromHead to ToHead.
func (c *Client) Interdiff(req InterdiffRequest) (InterdiffResult, error) {
	var result InterdiffResult
	err := c.locks.WithRepo(repoLockKey(req.RepoURL), func() error {
		repo, err := gitlib.PlainOpen(req.RepoPath)
		if err != nil {
			return fmt.Errorf("open repo %s: %w", req.RepoPath, err)
		}
		fromTree, err := headTree(repo, req.FromHead)
		if err != nil {
			return err
		}
		toTree, err := headTree(repo, req.ToHead)
		if err != nil {
			return err
		}
		if result.Patch, err = treeFilePatch(fromTree, toTree, req.Path); err != nil {
			return err
		}
		result.Rebased = fileChangedBetween(repo, req.FromMergeBase, req.ToMergeBase, req.Path)
		return nil
	})
	return result, err
}

func headTree(repo *gitlib.Repository, sha string) (*object.Tree, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrHeadUnavailable, truncateSHAForLog(sha))
	}
	if err != nil {
		return nil, fmt.Errorf("load commit %s: %w", truncateSHAForLog(sha), err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load tree of %s: %w", truncateSHAForLog(sha), err)
	}
	return tree, nil
}

func treeFilePatch(from, to *object.Tree, path string) (string, error) {
	changes, err := object.DiffTree(from, to)
	if err != nil {
		return "", fmt.Errorf("diff trees for %s: %w", path, err)
	}
	for _, change := range changes {
		if change.From.Name != path && change.To.Name != path {
			continue
		}
		patch, err := change.Patch()
		if err != nil {
			return "", fmt.Errorf("patch %s: %w", path, err)
		}
		for _, filePatch := range patch.FilePatches() {
			return filePatchToString(filePatch)
		}
	}
	return "", nil
}

// fileChangedBetween reports whether path differs between two merge bases.
// Unreadable merge bases count as changed: the label only warns.
func fileChangedBetween(repo *gitlib.Repository, fromMergeBase, toMergeBase, path string) bool {
	if fromMergeBase == toMergeBase {
		return false
	}
	if fromMergeBase == "" || toMergeBase == "" {
		return false // Pre-feature patchsets: no merge base recorded, nothing to compare.
	}
	fromBlob, fromErr := blobAt(repo, fromMergeBase, path)
	toBlob, toErr := blobAt(repo, toMergeBase, path)
	if fromErr != nil || toErr != nil {
		return true
	}
	return fromBlob != toBlob
}

// blobAt returns path's blob hash at commitSHA, or ZeroHash when absent.
func blobAt(repo *gitlib.Repository, commitSHA, path string) (plumbing.Hash, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(commitSHA))
	if err != nil {
		return plumbing.ZeroHash, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	entry, err := tree.FindEntry(path)
	if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
		return plumbing.ZeroHash, nil
	}
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return entry.Hash, nil
}
```

Check `object.ErrDirectoryNotFound` / `object.ErrEntryNotFound` exist in the installed go-git (`go doc github.com/go-git/go-git/v5/plumbing/object | grep -i notfound`); use whatever `FindEntry` returns.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go && go test ./internal/git/ -run 'InterdiffAfterForcePush|ForcePush' -v`
Expected: PASS.

- [ ] **Step 5: The handler**

`go/internal/server/handler/interdiff.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type interdiffPatchsets interface {
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
}

type interdiffer interface {
	Interdiff(req git.InterdiffRequest) (git.InterdiffResult, error)
}

// InterdiffHandlerDeps bundles REQUEST_INTERDIFF dependencies.
type InterdiffHandlerDeps struct {
	Patchsets interdiffPatchsets
	Git       interdiffer
	// RepoForPR returns the sync's clone URL and the on-disk repo for a PR;
	// ok=false when no repo is recorded yet.
	RepoForPR func(ctx context.Context, prID string) (repoURL, repoPath string, ok bool, err error)
	Log       *slog.Logger
}

type interdiffRequest struct {
	PRID         string `json:"pr_id"`
	Path         string `json:"path"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
}

type interdiffResponse struct {
	PRID         string `json:"pr_id"`
	Path         string `json:"path"`
	FromPatchset int    `json:"from_patchset"`
	ToPatchset   int    `json:"to_patchset"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason"`
	Patch        string `json:"patch"`
	Rebased      bool   `json:"rebased"`
}

// NewInterdiffHandler returns a WS handler for REQUEST_INTERDIFF: one file at
// the from-patchset head vs. the to-patchset head.
func NewInterdiffHandler(deps InterdiffHandlerDeps) ws.MessageHandler {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req interdiffRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse REQUEST_INTERDIFF: %w", err)
		}
		if req.PRID == "" || req.Path == "" || req.FromPatchset <= 0 {
			return nil, fmt.Errorf("REQUEST_INTERDIFF requires pr_id, path, and from_patchset")
		}
		logger := log.With("request_id", msg.ID, "msg_type", msg.Type, "pr_id", req.PRID)
		response := buildInterdiff(logging.WithLogger(ctx, logger), deps, req)
		return map[string]interface{}{"id": msg.ID, "type": "INTERDIFF_READY", "data": response}, nil
	}
}

func buildInterdiff(ctx context.Context, deps InterdiffHandlerDeps, req interdiffRequest) interdiffResponse {
	logger := logging.FromContext(ctx)
	response := interdiffResponse{PRID: req.PRID, Path: req.Path, FromPatchset: req.FromPatchset, ToPatchset: req.ToPatchset}
	patchsets, err := deps.Patchsets.ListPatchsets(ctx, req.PRID)
	if err != nil {
		logger.Warn("interdiff: list patchsets failed", "error", err)
		return unavailableInterdiff(response, "could not load patchsets")
	}
	from := patchsetByNumber(patchsets, req.FromPatchset)
	to := patchsetByNumber(patchsets, req.ToPatchset)
	if req.ToPatchset == 0 && len(patchsets) > 0 {
		to = &patchsets[len(patchsets)-1]
	}
	if from == nil || to == nil {
		return unavailableInterdiff(response, "patchset is not recorded")
	}
	response.ToPatchset = to.Number
	repoURL, repoPath, ok, err := deps.RepoForPR(ctx, req.PRID)
	if err != nil {
		logger.Warn("interdiff: repo lookup failed", "error", err)
		return unavailableInterdiff(response, "could not locate the repository")
	}
	if !ok {
		return unavailableInterdiff(response, "no local copy of this repository yet")
	}
	result, err := deps.Git.Interdiff(git.InterdiffRequest{
		RepoURL: repoURL, RepoPath: repoPath, Path: req.Path,
		FromHead: from.HeadSHA, ToHead: to.HeadSHA,
		FromMergeBase: from.MergeBaseSHA, ToMergeBase: to.MergeBaseSHA,
	})
	if errors.Is(err, git.ErrHeadUnavailable) {
		return unavailableInterdiff(response, fmt.Sprintf("P%d's head is no longer available locally", from.Number))
	}
	if err != nil {
		logger.Warn("interdiff failed", "path", req.Path, "error", err)
		return unavailableInterdiff(response, "could not compute the interdiff")
	}
	response.Available, response.Patch, response.Rebased = true, result.Patch, result.Rebased
	return response
}

func unavailableInterdiff(response interdiffResponse, reason string) interdiffResponse {
	response.Reason = reason
	return response
}
```

(`ErrHeadUnavailable` can name either head; the from head is the one a force-push drops, so the message names it.)

- [ ] **Step 6: The PR → repo adapter and registration**

`go/internal/server/interdiff_repo.go`:

```go
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/itterno/bistro/internal/store"
)

// prRepoLocator finds the repo a PR's diffs were generated in: the user's
// local clone or the cache clone, whichever the sync recorded.
type prRepoLocator struct {
	db        *store.DB
	locations *store.RepoLocationStore
}

func (l prRepoLocator) RepoForPR(ctx context.Context, prID string) (string, string, bool, error) {
	var owner, repo string
	err := l.db.QueryRowContext(ctx, `SELECT owner, repo FROM prs WHERE id = ?`, prID).Scan(&owner, &repo)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("look up repo of PR %s: %w", prID, err)
	}
	location, err := l.locations.Get(ctx, "github:"+owner+"/"+repo)
	if err != nil {
		return "", "", false, fmt.Errorf("get repo location of PR %s: %w", prID, err)
	}
	if location == nil || location.LocalPath == "" {
		return "", "", false, nil
	}
	// Same URL the sync clones (sync/fetcher.go), so the repo lock matches.
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), location.LocalPath, true, nil
}
```

In `app.go` `registerDiffAndCheckoutHandlers`, after `REQUEST_TRIAGE`:

```go
	a.wsServer.RegisterHandler("REQUEST_INTERDIFF", handler.NewInterdiffHandler(handler.InterdiffHandlerDeps{
		Patchsets: a.db,
		Git:       a.gitClient,
		RepoForPR: prRepoLocator{db: a.db, locations: stores.repoLocation}.RepoForPR,
		Log:       a.log,
	}))
```

- [ ] **Step 7: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 8: Commit** (app.go pre-existing oversized → `--no-verify` if blocked, note it)

```bash
git add go/internal/git/interdiff.go go/internal/git/interdiff_test.go go/internal/server
git commit -m "feat: add REQUEST_INTERDIFF per-file interdiff between patchset heads"
```
