# Phase 2.1 + 2.2 — Patchset Timeline + Since-Last-Review — Design

> Merged slice of the re-scoped Phase 2 (see
> `docs/superpowers/plans/2026-06-20-phase-2-decomposition.md`). Builds on the
> data + capability layer delivered by Phase 2 Groundwork (spec:
> `docs/superpowers/specs/2026-06-20-phase-2-groundwork-design.md`). Do NOT
> redesign the `patchsets`, `current_user`, or `ReviewThread`/`Comment` model —
> consume them.
>
> Grounded in the 2026-06-19 audit (`docs/ROADMAP-AUDIT-2026-06-19.md`) and a
> code-anchor exploration verified against the tree on 2026-06-20.
> Scope wording updated by BIS-31: FOSS exposes **observed patchsets** and best-effort
> changed-since-review only. Complete push-history capture is Team/server scope; see
> `docs/superpowers/specs/2026-06-29-foss-changed-since-review-strategy.md` and
> `docs/STAGE-2-TEAM.md`.

## Goal

Expose patchset history to the reviewer:

1. **Patchset timeline UI** — PR detail shows P1/P2/P3 tabs (observed patchsets
   from the foundation `patchsets` table) with a diff-pair selector. Labeled
   honestly: "snapshots observed since you started watching."
2. **Since-last-review toggle** — renders the best available diff between the
   last reviewed observed patchset and the current observed patchset via system
   `git range-diff` when possible, with graceful degraded states when system git
   or the required local objects are absent.
3. **Files-viewed survives rebase** — `files_viewed` table keyed on
   `(pr_id, file_path, login, content_hash)` so a file marked viewed stays viewed
   when its content is unchanged across a force-push.
4. **Degraded-mode indicator** — a small footer status icon when `git range-diff`
   is unavailable; not an intrusive per-diff banner.

Build order within the slice (sequential — each step depends on the previous):
`migration 014` → `rangediff.go` wrapper + capability probe → `last_reviewed_sha`
store + `files_viewed` store → Go handler additions → Electron UI.

## Settled decisions

- **Migration 014** — this slice owns it. Foundation owns 012; Phase 3
  classification owns 013; later 2.x slices start at 015.
- **Tables needed here:** `last_reviewed_sha (pr_id, login, sha, updated_at)` and
  `files_viewed (pr_id, file_path, login, content_hash, viewed_at)`. Neither
  existed before this slice.
- **`content_hash` derivation** — SHA-256 of the file's unified-diff string as
  produced by `go-git Tree.Patch`. Same content across a rebase → same hash →
  viewed state survives. Stored as a 64-char hex string.
- **`login` column** — sourced from `current_user.login` (written at auth-success
  by Groundwork). No integer FK; plain TEXT. Rationale: `current_user` is a
  single-row config table, not a multi-user FK target, and a TEXT join on login is
  sufficient at this scale.
- **`last_reviewed_sha`** — recorded on every review submission action
  (`APPROVE` / `REQUEST_CHANGES` / `COMMENT`) by intercepting the action handler
  in `go/internal/server/handler/`. It does NOT require a real-time patchset event.
- **`os/exec` discipline** — `git range-diff` is the **only** sanctioned
  `os/exec` in the codebase. It lives in one file (`go/internal/git/rangediff.go`)
  with args fully controlled (validated hex SHAs, repo path from our own clone
  directory). No user-supplied strings interpolated. No shell=true. No shell
  injection surface.
- **Capability probe** — one-time at startup via `git --version`. Parsed semver;
  require `>= 2.19.0` (when `git range-diff` was introduced). Result cached in
  `RangeDiffer.available bool`. Never re-probed per diff. Never hangs (timeout:
  3 s). Absent or too-old git → `available = false` → fallback.
- **Fallback** — `go-git Tree.Patch(baseTree, headTree)` head-to-head (the same
  path already used in `go/internal/git/diff.go:68`). Quality degrades but never
  crashes. UI shows footer degraded indicator.
- **Pure-Go range-diff engine is Stage 2** — explicitly NOT in this slice
  (see `docs/STAGE-2-TEAM.md` §S2.2). No reimplementation here.
- **TS error handling** — `neverthrow` `Result`/`ResultAsync` throughout. No
  `try`/`catch` in committed code. No `console.log`.
- **Go error handling** — idiomatic: `fmt.Errorf("context: %w", err)`. Per-item
  errors logged with `slog`, never abort outer tx/handler.

## Architecture / module layout

### New Go files

| File | Purpose |
|------|---------|
| `go/internal/git/rangediff.go` | `RangeDiffer`: capability probe + `RunRangeDiff(repoPath, base, head string) (RangeDiffResult, error)` |
| `go/internal/git/rangediff_test.go` | Table-driven tests: system-git-present (skip if not) and forced-absent fallback paths |
| `go/internal/store/last_reviewed.go` | `SetLastReviewedSHA` / `GetLastReviewedSHA` |
| `go/internal/store/files_viewed.go` | `MarkFileViewed` / `IsFileViewed` / `ListViewedFiles` |
| `go/internal/server/handler/patchsets.go` | `NewListPatchsetsHandler` WS handler |
| `go/internal/server/handler/rangediff.go` | `NewRangeDiffHandler` WS handler |

### Modified Go files

| File | Change |
|------|--------|
| `go/internal/store/migrations/014_add_last_reviewed_and_files_viewed.sql` | New migration (DDL below) |
| `go/internal/server/handler/auth.go` | No change — `SetCurrentUser` already wired by Groundwork |
| `go/internal/server/handler/diff.go` | No change — patchset-pair diff re-uses `NewRequestDiffHandler`; range-diff is its own handler |
| `go/internal/server/app.go` | Inject `RangeDiffer` into new handlers; run capability probe at startup |
| `go/internal/server/handler/pr.go` | `NewSubmitReviewHandler` (already exists or added here) calls `SetLastReviewedSHA` after successful action |
| `go/cmd/bistro-sync/main.go` | Log capability-probe result at startup (`slog.Info`) |

### New Electron files

| File | Purpose |
|------|---------|
| `electron/src/renderer/components/PatchsetTimeline.tsx` | P1/P2/P3 tab strip + diff-pair selector |
| `electron/src/renderer/components/RangeDiffView.tsx` | Renders range-diff output (or fallback plain diff) |
| `electron/src/renderer/components/DegradedRangeDiffIndicator.tsx` | Small icon; mounts via `FooterCenterSlot` |
| `electron/tests/unit/PatchsetTimeline.test.tsx` | Unit tests |
| `electron/tests/unit/RangeDiffView.test.tsx` | Unit tests |

### Modified Electron files

| File | Change |
|------|--------|
| `electron/src/renderer/pages/PRDetailPage.tsx` | Mount `PatchsetTimeline`; wire since-last-review toggle |
| `electron/src/renderer/pages/DiffPage.tsx` | Accept optional `patchsetPair` param; render `RangeDiffView` |
| `electron/src/renderer/utils/keybindings.ts` | Add `diff.sinceLastReview` binding (`s` key in diff mode) |
| `electron/src/renderer/types/index.ts` | Add `Patchset`, `RangeDiffResult`, `FileViewedState` types |

## Schema — migration 014

```sql
-- Migration: 014
-- Description: last_reviewed_sha per (pr_id, login) and files_viewed with content_hash for rebase-safe viewed state

CREATE TABLE IF NOT EXISTS last_reviewed_sha (
    pr_id      TEXT    NOT NULL,
    login      TEXT    NOT NULL,
    sha        TEXT    NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS files_viewed (
    pr_id        TEXT    NOT NULL,
    file_path    TEXT    NOT NULL,
    login        TEXT    NOT NULL,
    content_hash TEXT    NOT NULL,   -- SHA-256 hex of unified diff for this file
    viewed_at    TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, file_path, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_last_reviewed_pr ON last_reviewed_sha(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_viewed_pr_login ON files_viewed(pr_id, login);
```

**FK notes:**
- Both tables reference `prs(id)` with `ON DELETE CASCADE`; `_foreign_keys=on` is
  set in the DSN (`go/internal/store/db.go:22`).
- `login` is plain TEXT (not a FK to `current_user`) — `current_user` is a
  single-row config table, not a relational user registry.
- `files_viewed` PRIMARY KEY is `(pr_id, file_path, login)` — an upsert on this
  triple updates `content_hash` and `viewed_at`, so re-viewing a file after a rebase
  that changed its content replaces the old hash.

## `RangeDiffer` wrapper design

```
go/internal/git/rangediff.go
```

### Types

```go
// RangeDiffResult is the output of a range-diff run.
type RangeDiffResult struct {
    Raw       string // stdout from `git range-diff`
    Degraded  bool   // true when system git unavailable; output is plain Tree.Patch diff
    FallbackReason string // human-readable; empty when Degraded==false
}
```

### `NewRangeDiffer`

```go
func NewRangeDiffer(log *slog.Logger, gitClient *Client) *RangeDiffer
```

- `log == nil` guard: use `slog.Default()`.
- `gitClient` is the existing `*git.Client` (carries the local-clone path).
- On construction, immediately calls `probeGit()`:
  - Runs `exec.CommandContext(ctx, "git", "--version")` with a 3-second context.
  - Parses output: `git version X.Y.Z`. Extracts major/minor.
  - `available = major > 2 || (major == 2 && minor >= 19)`.
  - On any error (not found, timeout, parse fail) → `available = false`, log at
    `Info` level (not `Error` — absent git is a normal configuration).
  - Result stored in `rd.available` and `rd.fallbackReason` (set once, immutable).

### `RunRangeDiff`

```go
func (rd *RangeDiffer) RunRangeDiff(
    ctx context.Context,
    repoPath, baseRangeStart, baseRangeEnd, headRangeStart, headRangeEnd string,
) (RangeDiffResult, error)
```

**Injection safety** (all five callers' arguments are validated before `exec`):

1. `repoPath` — always the path returned by `gitClient.CloneDir(prID)`; never a
   user-supplied string. Validated to be an absolute path under the cache dir
   (`strings.HasPrefix(repoPath, os.UserCacheDir())`). If not → return error, no
   exec.
2. `baseRangeStart`, `baseRangeEnd`, `headRangeStart`, `headRangeEnd` — each
   validated as a 40-character lowercase hex string via `isHexSHA(s string) bool`.
   Any non-hex input → return error, no exec.
3. Args assembled as `[]string{"range-diff", "--no-color",
   baseRangeStart+".."+baseRangeEnd, headRangeStart+".."+headRangeEnd}` — no shell,
   no `cmd.String()` interpolation, no `sh -c`.
4. Working directory set to `repoPath` (via `cmd.Dir`).
5. Context carries a 30-second timeout (caller can tighten via its own context).

**Fallback path** (when `rd.available == false`):

- Call `rd.gitClient.GenerateDiff(repoPath, headRangeStart, headRangeEnd)` — the
  existing `go-git Tree.Patch` path (`diff.go:35`).
- Return `RangeDiffResult{Raw: diff, Degraded: true, FallbackReason: rd.fallbackReason}`.
- Never returns an error for the unavailability itself — degradation is not an error.

**No-crash contract:** any `exec` failure (non-zero exit, timeout) logs at `Warn`
and returns an error; the handler converts it to a degraded fallback response, not
an HTTP/WS 500.

## Timeline UI design

### `PatchsetTimeline` component

Renders a horizontal strip of patchset tabs (P1, P2, P3…) read from
`GET_PATCHSETS` WS response. Each tab shows `Pn — <short-sha>`.
A "diff pair" selector (two radio groups: FROM / TO) enables selecting any two
patchsets; defaults to `(Pn-1, Pn)`. Sends `REQUEST_RANGE_DIFF` WS message with
`{pr_id, base_sha, head_sha, since_last_review: false}`.

Label copy: **"Snapshots observed since you started watching."** — never "every
push." (Honesty contract from decomposition doc.)

### `RangeDiffView` component

Renders the `RangeDiffResult.Raw` string as a syntax-highlighted code block.
When `result.Degraded == true`, mounts `<FooterCenterSlot>` containing
`<DegradedRangeDiffIndicator>` (a small `⚠ range-diff unavailable` text + icon).
The indicator is NOT rendered on the main diff surface — footer only.

### Since-last-review toggle

In `PRDetailPage` / `DiffPage`: a keyboard-activated toggle (`s` key in diff mode,
registered in `keybindings.ts` as `diff.sinceLastReview`). When active, sends
`REQUEST_RANGE_DIFF` with `since_last_review: true`; the Go handler looks up
`last_reviewed_sha` for the current user and substitutes it as `base_sha`.

## WebSocket message types (new)

All messages follow the existing `ws.InboundMessage` / `ws.OutboundMessage`
envelope in `go/internal/server/ws/`.

| Direction | Type | Payload |
|-----------|------|---------|
| C → S | `GET_PATCHSETS` | `{pr_id: string}` |
| S → C | `PATCHSETS_READY` | `{pr_id, patchsets: Patchset[]}` |
| C → S | `REQUEST_RANGE_DIFF` | `{pr_id, base_sha, head_sha, since_last_review: bool}` |
| S → C | `RANGE_DIFF_READY` | `{pr_id, raw: string, degraded: bool, fallback_reason: string}` |
| C → S | `MARK_FILE_VIEWED` | `{pr_id, file_path, content_hash}` |
| S → C | `FILE_VIEWED_ACK` | `{pr_id, file_path}` |

## Control flow: since-last-review

```
User presses `s` in diff mode
  → Electron sends REQUEST_RANGE_DIFF {pr_id, since_last_review: true}
  → RangeDiffHandler:
      1. Fetch current_user.login from store
      2. Fetch last_reviewed_sha for (pr_id, login)
         → not found → return RANGE_DIFF_READY {degraded: true, fallback_reason: "no prior review"}
      3. Fetch PR current head_sha
      4. Call rd.RunRangeDiff(ctx, repoPath, baseSHA..lastSHA, baseSHA..headSHA)
      5. Return RANGE_DIFF_READY
  → RangeDiffView renders; if degraded → FooterCenterSlot shows indicator

User submits review (APPROVE / REQUEST_CHANGES / COMMENT):
  → Existing action handler succeeds
  → SetLastReviewedSHA(ctx, prID, login, headSHA)
```

## Control flow: files-viewed

```
User navigates to a file diff:
  → Electron sends REQUEST_DIFF {pr_id, base_sha, head_sha, file_path}
  → Go computes content_hash = SHA-256(per-file unified diff string)
  → DIFF_READY response includes {content_hash, viewed: IsFileViewed(pr_id, file_path, login, content_hash)}

User marks file viewed (keybinding or auto on full scroll):
  → Electron sends MARK_FILE_VIEWED {pr_id, file_path, content_hash}
  → MarkFileViewed(ctx, prID, filePath, login, contentHash)
  → FILE_VIEWED_ACK
```

## Error handling (degradation, never crash)

| Scenario | Behaviour |
|----------|-----------|
| No system git | `available=false` at probe; `RunRangeDiff` returns fallback diff; footer indicator shown |
| git < 2.19 | Same as above |
| git exec timeout (>30 s) | Context cancelled; `RunRangeDiff` returns fallback + logs Warn |
| git non-zero exit | Same: fallback + log Warn |
| Non-hex SHA passed to handler | Handler returns WS error `{type:"error", message:"invalid sha"}` before any exec |
| `last_reviewed_sha` not found | `RANGE_DIFF_READY {degraded: true, fallback_reason: "no prior review"}` |
| DB write error in `SetLastReviewedSHA` | Logged at Error; review submission itself NOT rolled back |
| DB write error in `MarkFileViewed` | Logged at Error; WS error response to client |
| `patchsets` empty for a PR | `PATCHSETS_READY {patchsets: []}` — UI shows "No snapshots yet" |

## Testing

### Go tests (`go/internal/git/rangediff_test.go`)

Two build tags guide CI:

- **`//go:build !requiregit`** (default) — all tests except live-git ones run. The
  capability-probe test is always compiled but the `systemGitPresent` function checks
  at runtime and calls `t.Skip("system git not available")` for the live path.
- **`//go:build requiregit`** (opt-in, for local dev) — forces the live tests to
  run without the skip guard.

Table-driven cases:

```
TestRangeDiffer_ProbeAvailable      — mock git binary on PATH returns "git version 2.43.0" → available=true
TestRangeDiffer_ProbeTooOld         — mock returns "git version 2.18.0" → available=false
TestRangeDiffer_ProbeNotFound       — PATH has no git → available=false (no panic)
TestRangeDiffer_ProbeTimeout        — mock sleeps >3 s → available=false (no hang)
TestRangeDiffer_FallbackDiff        — available=false → RunRangeDiff returns Degraded=true, non-empty Raw
TestRangeDiffer_InjectionGuard_Path — non-cache-dir repoPath → error, no exec
TestRangeDiffer_InjectionGuard_SHA  — non-hex sha → error, no exec
TestRangeDiffer_LiveRangeDiff       — skipped unless system git >= 2.19 present; runs real range-diff on temp repo
```

**Mock git binary pattern:** tests inject a fake `git` by writing a tiny shell
script to `t.TempDir()` and prepending it to `PATH` in the test process env (via
`t.Setenv`). The script echoes the desired output and exits 0/1. No network, no
real repo.

### Go store tests (`go/internal/store/`)

- `TestMigration014AddsLastReviewedAndFilesViewed` — in-memory DB, run all
  migrations, assert both tables and indexes exist via `pragma_table_info`.
- `TestSetGetLastReviewedSHA` — set/get round-trip; missing login returns `"", false, nil`.
- `TestMarkIsFileViewed_ContentHashSemantics` — same hash = viewed; different hash = not viewed.

### Electron tests (`electron/tests/unit/`)

- `PatchsetTimeline.test.tsx` — renders P1/P2/P3 from fixture data; pair selector
  emits correct `REQUEST_RANGE_DIFF` payload.
- `RangeDiffView.test.tsx` — degraded=false renders diff block without footer
  indicator; degraded=true mounts `FooterCenterSlot` with indicator text.

## Acceptance criteria

- [ ] Migration 014 applies cleanly on top of 012 in an in-memory test DB.
- [ ] `go test ./internal/git/ ./internal/store/ -race ./...` all pass.
- [ ] Capability probe result logged at startup (`git range-diff available: true/false`).
- [ ] No os/exec call anywhere in the codebase except `rangediff.go`
      (verified by `grep -rn "os/exec" go/internal/ | grep -v rangediff.go` returning nothing).
- [ ] Non-hex SHA input to `RunRangeDiff` returns error without exec (unit tested).
- [ ] Fallback path produces a non-empty diff and sets `Degraded=true` (unit tested without system git).
- [ ] `last_reviewed_sha` is updated after a successful review submission (integration test).
- [ ] `files_viewed` row with same `content_hash` survives a synthetic rebase (content-hash round-trip test).
- [ ] Footer degraded indicator appears when `RANGE_DIFF_READY.degraded == true` (Electron unit test).
- [ ] `pnpm type-check && pnpm lint` clean.
- [ ] `pnpm test` passes (149+ tests).

## Out of scope

- **Pure-Go range-diff engine** — Stage 2 (`docs/STAGE-2-TEAM.md` §S2.2).
- **True push-time `pushed_at`** — Stage 2 §S2.1.
- **Comment anchoring across rebase** — Phase 2.3.
- **Resolve/unresolve write-back** — Phase 2.4.
- **Multi-user / shared DB** — `current_user` is single-user local; no multi-login
  design here.
- **Auto-scroll-to-mark-viewed** — deferred UX nicety; `MARK_FILE_VIEWED` message
  is the primitive; auto-triggering on scroll is a follow-on.
