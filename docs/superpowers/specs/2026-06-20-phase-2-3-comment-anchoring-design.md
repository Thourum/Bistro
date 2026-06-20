# Phase 2.3 — Comment Anchoring Across Rebase (Design Spec)

**Slice of:** Phase 2 Re-scope — Patchset Semantics
**Builds on:** Phase 2 Groundwork (`2026-06-20-phase-2-groundwork-design.md`) — migration 012 already adds `commit_sha`, `original_sha`, `side`, `position`, `original_position`, `in_reply_to`, `thread_id`, `patchset_number` to `comments`; the `patchsets` table; `review_threads`.
**Migration needed:** YES — **015** (see Schema section; 014 belongs to 2.1+2.2).
**Status:** spec written 2026-06-20; not started.

---

## Goal

When a PR author rebases, existing inline review comments retain their GitHub-assigned
`position` / `original_position` values, which become meaningless against the new diff.
Phase 2.3 makes those comments re-appear at the correct new line number — or, if the
commented line was deleted entirely, marks them **outdated** and shows them inline at
the nearest surviving hunk boundary. Zero new API calls are needed: all computation
runs against the local go-git clone.

---

## Settled Decisions

1. **Anchoring triple:** each `kind='review'` comment is anchored to
   `(file_path, line_content_hash, surrounding_context_hash)` in addition to the
   raw `position` value stored from the API.

2. **Compute on-the-fly, not stored:** anchor hashes are computed at serve time from
   the clone, not persisted. Rationale: storing them would require a further migration
   and a background re-hash job whenever the clone is refreshed; re-computation over
   a local file is cheap (SHA-256 of a handful of lines) and deterministic.
   Exception: `patchset_number` IS stored (written by the sync writer, see
   foundation spec §3 "Observed-patchset capture") because it encodes *when* the
   comment was made, not where.

3. **`patchset_number` stamps outdated:** a comment is outdated when
   `comment.patchset_number < current_patchset_number` for that PR AND re-anchoring
   fails to find the content hash in the current head tree.

4. **Outdated placement:** show outdated comments inline, attached to the nearest
   surviving hunk boundary line (closest `+`/`-` line in the same file by absolute
   line-number distance). Never banish to a sidebar.

5. **Clone fallback:** if the local clone is unavailable or the diff computation
   fails, the handler falls back to returning the raw stored `position`/`line_number`
   with `anchored: false`. This is never an error to the caller.

6. **Scope:** pure Go, pure-local, no new API calls, no new provider methods, no
   Electron changes in this slice (the TS side already receives `line`, `file`,
   `outdated`, `anchored_line` as nullable fields — foundation passes them through
   flat; 2.3 starts populating them).

7. **`patchset_number` population:** the sync writer must stamp `patchset_number` on
   every `kind='review'` comment at write time (the patchset upsert logic defined in
   foundation §3 runs first; the comment write reads back the just-inserted patchset
   number). This is a writer responsibility, not an anchoring responsibility.

---

## Architecture / Module Layout

```
go/internal/git/
  anchor.go          NEW  — AnchorResolver: all anchoring logic
  anchor_test.go     NEW  — table-driven tests over fixture repos

go/internal/server/handler/
  comments.go        MODIFY — call AnchorResolver.Resolve() on each kind='review' row
  comments_test.go   MODIFY — add anchor/outdated assertion cases

go/internal/sync/
  writer.go          MODIFY — stamp patchset_number on kind='review' INSERT/UPSERT

go/internal/store/migrations/
  015_add_anchor_columns.sql  NEW — see Schema section
```

No new package. `anchor.go` lives in `go/internal/git/` alongside `diff.go` because
it uses the same go-git `fdiff.FilePatch` / `plumbing` primitives and the same
`Client` struct (extended with a new `Resolve` method).

---

## Schema

Migration **015** is needed for one reason: the comments handler must be able to
return `anchored_line` and `outdated` without recomputing anchors on every request
for comments that are known-current (patchset_number == current). Storing the result
of the last anchor resolution avoids redundant clone traversal on every WS request.

```sql
-- Migration: 015_add_anchor_columns
-- Description: Persist last resolved anchor position and outdated flag for
--              kind='review' comments. Computed by AnchorResolver at serve time
--              and written back (best-effort) so subsequent serves are free.
--              NULL means "not yet resolved" (same as pre-2.3 rows).

ALTER TABLE comments ADD COLUMN anchored_line   INTEGER;  -- resolved line in current head; NULL = unresolved/outdated
ALTER TABLE comments ADD COLUMN anchor_head_sha TEXT;     -- head SHA this anchor was resolved against; invalidated on mismatch
ALTER TABLE comments ADD COLUMN outdated        INTEGER NOT NULL DEFAULT 0;  -- 1 = content deleted in current head
ALTER TABLE comments ADD COLUMN outdated_near_line INTEGER; -- nearest surviving hunk line when outdated=1
```

**Why not compute-only?** The anchor resolution requires opening the clone, reading
the tree for `commit_sha` (original anchor) and the current head, iterating chunks,
and computing hashes. For a PR with 50 review comments, that is 50 × (two tree
lookups + chunk iteration). Caching per `anchor_head_sha` reduces subsequent serves
to a simple SQL read. The cache is invalidated by comparing stored `anchor_head_sha`
against the latest `patchsets.head_sha` — a single join.

**No migration 013 or 014 conflict:** 012 = Phase 2 Groundwork; 013 = Phase 3
classification; 014 = Phase 2.1+2.2 patchset timeline; 015 = this slice.

---

## Anchoring Algorithm

### Inputs per comment

- `comment.file_path` — file the comment was placed on
- `comment.commit_sha` — the HEAD SHA of the patchset at comment time (GitHub
  `commit_id` field, stored by foundation writer)
- `comment.side` — `'LEFT'` (base) or `'RIGHT'` (head)
- `comment.position` — GitHub unified-diff position (1-based line in the diff
  output, context lines counted)
- `comment.patchset_number` — which observed patchset this was made against
- current `patchsets.head_sha` for the PR
- `localPath` — path to the local clone

### Step 1 — Resolve original line number from stored position

Open the clone. Get the commit object for `commit_sha`. Traverse its parent to get
`baseSHA`. Call `baseTree.Patch(headTree)` (already done by `GenerateDiff`). Find
the `FilePatch` whose `to.Path() == file_path` (or `from.Path()` for LEFT). Walk
`filePatch.Chunks()` counting diff positions until reaching `comment.position`;
record the `newLineNo` (the 1-based line number in the RIGHT/HEAD file at that
position).

This gives `originalLine` — the line in `commit_sha`'s tree that the comment refers
to.

### Step 2 — Compute content hash at original line

Read the blob for `file_path` at `commit_sha` from the clone (via
`commitObj.File(file_path)` → `Contents()`). Split into lines. Extract:

- **target line:** `lines[originalLine-1]` after stripping leading/trailing whitespace
- **context window:** the 3 lines before and 3 lines after (clamped to file
  boundaries), also stripped

Compute:
```
line_content_hash     = hex(sha256(strings.TrimSpace(targetLine)))
context_hash          = hex(sha256(strings.Join(contextLines, "\n")))
```

The 3-line context window (±3) is chosen to tolerate single-line additions/deletions
without false negatives, while remaining specific enough to avoid false positives in
files with repeated boilerplate.

### Step 3 — Re-resolve in current head

Get `currentHeadSHA` from `patchsets` (latest number for this PR). If
`currentHeadSHA == commit_sha` the comment is current — skip re-resolution, use
`originalLine` directly.

Otherwise open `currentHeadSHA` commit. Read blob for `file_path` at current head.
If the file does not exist → **outdated, file deleted** (go to Step 4).

Walk lines of the current file. For each line `i`:

1. Compute `hex(sha256(strings.TrimSpace(lines[i])))`.
2. If it equals `line_content_hash`, compute the context hash for lines `[i-3, i+3]`.
3. If `context_hash` also matches → **match found**; `anchoredLine = i+1`. Return.

If the full scan yields no match → **outdated, content deleted** (go to Step 4).

The scan is O(n) over file lines. Files over 100 000 lines are pathological; cap
at 200 000 lines and fall back to line-number-only if exceeded (log a warning).

### Step 4 — Outdated: find nearest surviving hunk line

Get the diff between `commit_sha`'s parent and `currentHeadSHA` for `file_path`
(reuse `GenerateDiff`). Collect all lines in the diff output that are additions (`+`)
or context lines on the RIGHT side; record their new line numbers. Find the entry
with minimum `abs(newLineNo - originalLine)`. That is `outdatedNearLine`.

If no such lines exist (entire file deleted), `outdatedNearLine = 0`.

### Output per comment

```go
type AnchorResult struct {
    CommentID       string
    AnchoredLine    int    // 0 if outdated
    OutdatedNearLine int   // nearest surviving line; 0 if file gone
    Outdated        bool
    Anchored        bool   // false = fell back to raw line_number (clone missing etc.)
    HeadSHA         string // the head SHA this result is valid for
}
```

---

## Control Flow

### Where anchoring runs

Anchoring runs **at serve time** inside `NewRequestCommentsHandler`, not during sync.
Rationale: anchoring requires the clone to be present; clones are best-effort and may
lag behind the sync. Running at serve time means the user always sees the freshest
anchor. The resolved result is written back to the DB (columns added by 015) as a
best-effort cache; a failed write-back is logged and ignored (never returned as an
error to the WS client).

### Cache invalidation

Before calling `AnchorResolver.Resolve()`, the handler checks:
```sql
SELECT c.id, c.anchored_line, c.anchor_head_sha, c.outdated, c.outdated_near_line,
       p.head_sha AS current_head_sha
FROM comments c
JOIN patchsets p ON p.pr_id = c.pr_id AND p.number = (
    SELECT MAX(number) FROM patchsets WHERE pr_id = c.pr_id
)
WHERE c.pr_id = ? AND c.kind = 'review'
```

If `anchor_head_sha == current_head_sha`, use cached values — no clone access.
If mismatch (or NULL), call resolver and write back.

### patchset_number stamping (sync writer)

In `writer.go`, inside the review-comment upsert (foundation added by Groundwork), after
the patchset INSERT OR IGNORE:

```sql
SELECT number FROM patchsets WHERE pr_id = ? AND head_sha = ?
```

Bind result as `patchset_number` in the comment INSERT. This is idempotent: if the
patchset row was already there, the SELECT still returns the right number.

---

## Error Handling

| Condition | Behaviour |
|---|---|
| Clone directory missing | `Anchored=false`, return raw `line_number`; log `Debug` |
| Commit SHA not found in clone | `Anchored=false`, return raw `line_number`; log `Warn` |
| File not in commit tree | `Outdated=true`, `AnchoredLine=0`, `OutdatedNearLine=0` |
| No content hash match | `Outdated=true`, run Step 4 for nearest line |
| Write-back to DB fails | Log `Warn`, continue — stale cache on next request |
| `position` out of range | `Anchored=false`, return raw `line_number`; log `Warn` |

No error is ever surfaced to the WS client from anchoring logic. The handler always
returns a valid response.

---

## Testing

All tests are pure-Go, deterministic, no network.

### Fixture repo construction

`anchor_test.go` creates in-memory or on-disk bare repos using `go-git` memory
storage (`memory.NewStorage()`) and programmatically commits files. This avoids any
dependency on a real GitHub repo or test fixture data files.

Helper `buildFixtureRepo(t, commits []fixtureCommit) *git.Client` creates a client
pointing at an in-memory clone; each `fixtureCommit` specifies a map of
`path → content`. Used across all test cases.

### Table-driven cases

| Case | Setup | Expected |
|---|---|---|
| Content survived, same line | commit A: `foo.go` L10 = "target"; comment at L10; commit B: identical file | `AnchoredLine=10`, `Outdated=false` |
| Content moved down | commit A: `foo.go` L10 = "target"; commit B: 5 lines inserted above | `AnchoredLine=15`, `Outdated=false` |
| Content moved up | commit A: `foo.go` L10 = "target"; commit B: 3 lines deleted above | `AnchoredLine=7`, `Outdated=false` |
| Content deleted | commit A: `foo.go` L10 = "target"; commit B: line 10 removed; L9 and L11 survive | `Outdated=true`, `OutdatedNearLine` ∈ {9, 11} |
| File deleted entirely | commit A: `foo.go` exists; commit B: file removed | `Outdated=true`, `OutdatedNearLine=0` |
| Same patchset (no rebase) | `commit_sha == currentHeadSHA` | `AnchoredLine = originalLine`, no clone re-read needed |
| Clone missing | resolver initialised with non-existent path | `Anchored=false` |
| Identical content, different context | two identical target lines in file; one with matching ±3 context | resolves to the line with matching context, not the other |
| Position out of range | `position > total diff lines` | `Anchored=false`, no panic |

### Verify commands

```bash
cd go && go test -race ./internal/git/ -run TestAnchor -v
cd go && go test -race ./internal/server/handler/ -run TestRequestComments -v
cd go && go vet ./...
```

---

## Acceptance Criteria

- [ ] `go test -race ./internal/git/ -run TestAnchor` passes all table-driven cases.
- [ ] `go test -race ./internal/server/handler/ -run TestRequestComments` includes
      assertions for `anchored_line`, `outdated`, `outdated_near_line` fields.
- [ ] `REQUEST_COMMENTS` WS response includes populated `anchored_line` / `outdated`
      for `kind='review'` comments when clone is present.
- [ ] When clone is absent, response contains `line_number` fallback and no error.
- [ ] Migration 015 applies cleanly on top of 012 + 013 + 014.
- [ ] `go vet ./...` clean; `go test -race ./...` 0 data races.

---

## Out of Scope

- Range-diff (belongs to 2.1+2.2).
- Resolve/unresolve write-back (belongs to 2.4).
- Electron UI changes — the TS `commentRow` type already has `line`, `file`,
  `outdated` as nullable fields; the React component uses whatever the handler sends.
  Visual treatment of "outdated" badges is a UI concern outside this slice.
- Backfilling `patchset_number` for comments written before 2.3 shipped — those rows
  have `patchset_number = NULL`; the handler treats NULL as "unknown patchset" and
  does not mark them outdated (conservative: show at raw line_number, `anchored=false`).
