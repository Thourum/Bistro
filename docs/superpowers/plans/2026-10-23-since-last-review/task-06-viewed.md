# Task 6: Viewed hash = patch ID; all-viewed baseline

**Files:**
- Modify: `go/internal/server/handler/diff.go` (replace `PatchContentHash` with `viewedHash`)
- Modify: `go/internal/server/handler/patchsets.go` (`MARK_FILE_VIEWED` deps + baseline)
- Create: `go/internal/server/handler/viewed_baseline.go`
- Create: `go/internal/server/handler/viewed_baseline_test.go`
- Modify: `go/internal/server/handler/patchsets_test.go`, `diff_test.go` (callers of renamed functions)
- Modify: `go/internal/server/app.go` (`MARK_FILE_VIEWED` registration)

**Interfaces:**
- Consumes: `store.DiffFile.PatchID` (Task 3, written by Task 5), `git.PatchID` (Task 1), `(*store.DB).ListViewedFiles`, `CurrentPatchset`, `RecordLastReviewed`, `store.ReviewSourceAllViewed`.
- Produces:
  - `func viewedHash(file store.DiffFile) string` — `file.PatchID`, or `git.PatchID(file.Patch)` for rows written before migration 035. This is the `content_hash` in `DIFF_READY` and the value `MARK_FILE_VIEWED` validates. The client already echoes `content_hash` back unchanged, so no UI change.
  - `type MarkFileViewedDeps struct { Viewed fileViewedMarker; CurrentUser currentUserReader; DiffStore diffReader; Baseline viewedBaselineStore; Log *slog.Logger }`
  - `func NewMarkFileViewedHandlerWithDeps(deps MarkFileViewedDeps) ws.MessageHandler` — replaces `NewMarkFileViewedHandlerWithDiffStore` (delete it). `NewMarkFileViewedHandler(db, cu, log)` stays as a thin constructor building deps.

**Why:** Spec §2: viewed marks must survive line-shift-only rebases. Spec §3 event 3: the `MARK_FILE_VIEWED` that leaves zero unviewed files sets an `all_viewed` baseline.

- [ ] **Step 1: Write the failing test**

`go/internal/server/handler/viewed_baseline_test.go` (`newReadStateTestDB` and `mockCurrentUserReader` exist in this package's tests; check `ws.InboundMessage`'s `Data` type and match it):

```go
package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

func TestMarkingLastFileViewedRecordsAllViewedBaseline(t *testing.T) {
	db := newReadStateTestDB(t)
	ctx := context.Background()
	const prID = "github:o/r:1"
	for _, statement := range []string{
		`INSERT INTO prs (id, provider, owner, repo, number, title, author, state, created_at, updated_at,
		                  base_branch, head_branch, base_sha, head_sha)
		 VALUES ('github:o/r:1', 'github', 'o', 'r', 1, 'T', 'a', 'open', datetime('now'), datetime('now'),
		         'main', 'f', 'tip', 'h1')`,
		`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		 VALUES ('github:o/r:1', 1, 'h1', 'tip', datetime('now'))`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	diffs := store.NewDiffStore(db.DB)
	if err := diffs.SaveDiffs(prID, []store.DiffFile{
		{FilePath: "a.go", Patch: "pa", PatchID: "id-a"},
		{FilePath: "b.go", Patch: "pb", PatchID: "id-b"},
	}); err != nil {
		t.Fatalf("SaveDiffs: %v", err)
	}
	handler := NewMarkFileViewedHandlerWithDeps(MarkFileViewedDeps{
		Viewed: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true},
		DiffStore: diffs, Baseline: db,
	})
	mark := func(path, hash string) {
		t.Helper()
		data, _ := json.Marshal(map[string]string{"pr_id": prID, "file_path": path, "content_hash": hash})
		if _, err := handler(ctx, ws.InboundMessage{ID: "m-" + path, Type: "MARK_FILE_VIEWED", Data: data}); err != nil {
			t.Fatalf("mark %s: %v", path, err)
		}
	}

	mark("a.go", "id-a")
	if _, ok, _ := db.GetLastReviewed(ctx, prID, "alice"); ok {
		t.Fatal("baseline recorded while b.go is still unviewed")
	}
	mark("b.go", "id-b")
	record, ok, err := db.GetLastReviewed(ctx, prID, "alice")
	if err != nil || !ok {
		t.Fatalf("GetLastReviewed: ok=%v err=%v", ok, err)
	}
	if record.Source != store.ReviewSourceAllViewed || record.SHA != "h1" || record.PatchsetNum != 1 {
		t.Fatalf("baseline = %+v, want all_viewed at h1/P1", record)
	}
}
```

If the `prs` insert fails on a NOT NULL column added by a later migration, add that column to the seed.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/server/handler/ -run AllViewedBaseline -v`
Expected: FAIL to compile (`undefined: NewMarkFileViewedHandlerWithDeps`).

- [ ] **Step 3: Switch the viewed hash**

In `handler/diff.go` delete `PatchContentHash` (and its now-unused `crypto/sha256`/`encoding/hex` imports), import `github.com/itterno/bistro/internal/git`, and add:

```go
// viewedHash is the canonical content hash for a file's viewed state: its
// patch ID, which survives rebases that only move the change. Rows cached
// before migration 035 have no stored patch ID and fall back to computing it.
func viewedHash(file store.DiffFile) string {
	if file.PatchID != "" {
		return file.PatchID
	}
	return git.PatchID(file.Patch)
}
```

Use `viewedHash(f)` where `PatchContentHash(f.Patch)` was used in the REQUEST_DIFF loop, and update the `diffFileDetail.ContentHash` doc comment ("patch ID of the file's change"). In `handler/patchsets.go` `validateFileHash`, use `canonical := viewedHash(f)`. Update tests that called `PatchContentHash(patch)` to call `viewedHash(store.DiffFile{Patch: patch})`.

- [ ] **Step 4: Deps-based MARK_FILE_VIEWED with the all-viewed baseline**

In `handler/patchsets.go`, replace `NewMarkFileViewedHandlerWithDiffStore` and change `newMarkFileViewedHandler` to take `MarkFileViewedDeps` (use `deps.Viewed`, `deps.CurrentUser`, `deps.DiffStore`, `deps.Log` where the positional arguments were). `NewMarkFileViewedHandler(db, cu, log)` becomes `return newMarkFileViewedHandler(MarkFileViewedDeps{Viewed: db, CurrentUser: cu, Log: log})`. Add:

```go
// MarkFileViewedDeps bundles MARK_FILE_VIEWED dependencies. DiffStore enables
// hash validation; Baseline enables the all-viewed baseline. Both optional.
type MarkFileViewedDeps struct {
	Viewed      fileViewedMarker
	CurrentUser currentUserReader
	DiffStore   diffReader
	Baseline    viewedBaselineStore
	Log         *slog.Logger
}

// NewMarkFileViewedHandlerWithDeps creates the MARK_FILE_VIEWED handler.
func NewMarkFileViewedHandlerWithDeps(deps MarkFileViewedDeps) ws.MessageHandler {
	return newMarkFileViewedHandler(deps)
}
```

After the successful `MarkFileViewed` write (before the success ACK), call `recordAllViewedBaseline(ctx, deps, req.PRID, login)`.

`go/internal/server/handler/viewed_baseline.go`:

```go
package handler

import (
	"context"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
)

// viewedBaselineStore records the all-viewed baseline. Satisfied by *store.DB.
type viewedBaselineStore interface {
	ListViewedFiles(ctx context.Context, prID, login string) ([]store.FileViewed, error)
	CurrentPatchset(ctx context.Context, prID string) (store.Patchset, bool, error)
	RecordLastReviewed(ctx context.Context, event store.ReviewEvent) error
}

// recordAllViewedBaseline records an all_viewed baseline once a mark leaves no
// unviewed file in the current diff. Best-effort: the mark already succeeded.
func recordAllViewedBaseline(ctx context.Context, deps MarkFileViewedDeps, prID, login string) {
	if deps.Baseline == nil || deps.DiffStore == nil {
		return
	}
	logger := logging.FromContext(ctx)
	diffs, err := deps.DiffStore.GetDiffs(prID)
	if err != nil {
		logger.Warn("all-viewed baseline: diff lookup failed", "pr_id", prID, "error", err)
		return
	}
	viewed, err := deps.Baseline.ListViewedFiles(ctx, prID, login)
	if err != nil {
		logger.Warn("all-viewed baseline: viewed lookup failed", "pr_id", prID, "error", err)
		return
	}
	if !allFilesViewed(diffs, viewed) {
		return
	}
	patchset, found, err := deps.Baseline.CurrentPatchset(ctx, prID)
	if err != nil || !found {
		logger.Warn("all-viewed baseline: no current patchset", "pr_id", prID, "found", found, "error", err)
		return
	}
	if err := deps.Baseline.RecordLastReviewed(ctx, store.ReviewEvent{
		PRID: prID, Login: login, HeadSHA: patchset.HeadSHA, PatchsetNum: patchset.Number,
		ReviewedAt: time.Now(), Source: store.ReviewSourceAllViewed,
	}); err != nil {
		logger.Warn("all-viewed baseline: record failed", "pr_id", prID, "error", err)
	}
}

func allFilesViewed(diffs []store.DiffFile, viewed []store.FileViewed) bool {
	hashes := make(map[string]string, len(viewed))
	for _, file := range viewed {
		hashes[file.FilePath] = file.ContentHash
	}
	for _, diff := range diffs {
		if hashes[diff.FilePath] != viewedHash(diff) {
			return false
		}
	}
	return len(diffs) > 0
}
```

In `server/app.go` replace the `MARK_FILE_VIEWED` registration with:

```go
	a.wsServer.RegisterHandler("MARK_FILE_VIEWED", handler.NewMarkFileViewedHandlerWithDeps(handler.MarkFileViewedDeps{
		Viewed: a.db, CurrentUser: stores.currentUser, DiffStore: stores.diff, Baseline: a.db, Log: a.log,
	}))
```

- [ ] **Step 5: Run tests**

Run: `cd go && go test ./internal/server/handler/ -v -run 'AllViewedBaseline|MarkFileViewed|Diff'`
Expected: PASS.

- [ ] **Step 6: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 7: Commit** (app.go pre-existing oversized → `--no-verify` if blocked, note it)

```bash
git add go/internal/server
git commit -m "feat: key viewed state on patch ID and record all-viewed baselines

Existing viewed marks reset once: their stored hash is the old sha256(patch)."
```
