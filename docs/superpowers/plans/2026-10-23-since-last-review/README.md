# Since Last Review After Rebases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Each task lives in its own file next to this one; read this README plus your task file.

**Goal:** Make `s` triage files changed/new/removed/unchanged since the user's last review, survive rebases and force-pushes, and let `i` show a per-file interdiff.

**Architecture:** The Go sync diffs against the merge base, records a patchset when head or merge base changes, and stores a per-file `patch_id` (line-shift-insensitive fingerprint) per patchset. A baseline (latest of GitHub review / Bistro action / all-viewed) selects the patchset to compare against. Two new WS requests (`REQUEST_TRIAGE`, `REQUEST_INTERDIFF`) replace `REQUEST_RANGE_DIFF`. PR heads are pinned under `refs/bistro/…` so interdiffs work after force-pushes.

**Tech Stack:** Go 1.26, go-git v5.19.3, SQLite + goose migrations + sqlc v1.31.1; Electron/React 19, TypeScript, zod, `@thourum/fault`, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-23-since-last-review-rebases-design.md` (read its "Implementation notes" section — it records where this plan deliberately deviates from the spec text).

## Global Constraints

- Go owns all SQLite writes; the UI never writes the DB.
- Electron TS: **never** `try`/`catch`; use `@thourum/fault` `Result`/`ResultAsync` chains. Validate WS payloads with zod schemas + `parseResponse` (`utils/error.ts`), never `as` casts.
- Hotkeys are config in `electron/src/renderer/utils/keybindings.ts`, handlers in `utils/keybindingActions.ts`; never register hotkeys outside `useAppHotkeys`.
- No React component tests. Utility/hook tests only. Go tests only where they would fail if logic broke.
- Never hand-edit `go/internal/store/db/`. After editing `go/internal/store/queries/*.sql` or adding a migration run: `cd go && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`.
- Migrations are versioned, idempotent, embedded; the next number is `00035`.
- Pre-commit hook blocks staged files over ~20 KB (5,000 tokens). Put new code and new tests in **new files**. If a commit is blocked only because an already-oversized file (`go/internal/server/app.go`, `go/internal/git/clone.go`, `go/internal/sync/writer_test.go`) received a small edit, commit with `--no-verify` and write "pre-existing oversized file" in the commit body.
- Commit messages: `<type>: <description>` (feat, fix, refactor, docs, test, chore).
- Verify before each commit: `cd go && go vet ./... && go test -race ./...`; for Electron tasks `cd electron && pnpm type-check && pnpm lint && pnpm test`.
- Use the project's design tokens/utility classes (Tailwind tokens like `text-primary`, `text-outline`, `border-outline-variant/30`); never raw hex.
- Viewed marks reset once on upgrade (hash changes) — must be noted in release notes (Task 11).

## Shared names (all tasks)

| Name | Package / file | Signature |
|---|---|---|
| `PatchID` | `go/internal/git/patchid.go` | `func PatchID(patch string) string` |
| `git.DiffFile.PatchID` | `go/internal/git/types.go` | `PatchID string` |
| `MergeBase` | `go/internal/git/mergebase.go` | `func MergeBase(localPath, baseSHA, headSHA string) (string, error)` |
| `ErrMergeBaseNotFound` | same | sentinel |
| `(*Client).Deepen` | same | `func (c *Client) Deepen(ctx context.Context, localPath string, opts *CloneOptions) error` |
| `(*Client).fetchIntoRepo` | `go/internal/git/refresh.go` | `func (c *Client) fetchIntoRepo(ctx context.Context, repoDir string, opts *CloneOptions, depth int) error` |
| `PRFetchResult.MergeBaseSHA`, `.Reviews` | `go/internal/sync/fetcher.go` | `string`, `[]*provider.Review` |
| `provider.Review.CommitID` | `go/internal/provider/models.go` | `string` |
| `store.Patchset` | `go/internal/store/patchsets.go` | adds `MergeBaseSHA string json:"merge_base_sha"`, `HasFingerprints bool json:"has_fingerprints"` |
| `store.DiffFile.PatchID` | `go/internal/store/diff_store.go` | `string` |
| `(*store.DB).ListPatchsetFiles` | `go/internal/store/patchsets.go` | `(ctx, prID string, number int) (map[string]string, error)` (path → patch_id) |
| `(*store.DB).CurrentPatchset` | same | `(ctx, prID string) (Patchset, bool, error)` |
| `store.ReviewSource` + consts | `go/internal/store/last_reviewed.go` | `ReviewSourceGitHubReview="github_review"`, `ReviewSourceBistroAction="bistro_action"`, `ReviewSourceAllViewed="all_viewed"` |
| `store.ReviewEvent` | same | `{PRID, Login, HeadSHA string; PatchsetNum int; ReviewedAt time.Time; Source ReviewSource}` |
| `store.LastReviewedRecord` | same | `{SHA string; PatchsetNum int; ReviewedAt time.Time; Source ReviewSource}` |
| `store.RecordLastReviewed` | same | `func RecordLastReviewed(ctx context.Context, q storedb.DBTX, event ReviewEvent) error` |
| `(*store.DB).RecordLastReviewed` | same | `(ctx, event ReviewEvent) error` |
| `(*store.DB).GetLastReviewed` | same | `(ctx, prID, login string) (LastReviewedRecord, bool, error)` |
| `viewedHash` | `go/internal/server/handler/diff.go` | `func viewedHash(file store.DiffFile) string` |
| `git.InterdiffRequest/Result`, `(*Client).Interdiff`, `ErrHeadUnavailable` | `go/internal/git/interdiff.go` | see Task 8 |
| WS `REQUEST_TRIAGE` → `TRIAGE_READY` | `go/internal/server/handler/triage.go` | see Task 4 |
| WS `REQUEST_INTERDIFF` → `INTERDIFF_READY` | `go/internal/server/handler/interdiff.go` | see Task 8 |

## Tasks (execute in order)

1. [task-01-patch-id.md](task-01-patch-id.md) — `PatchID` + `DiffFile.PatchID`
2. [task-02-merge-base.md](task-02-merge-base.md) — merge-base diffs in sync
3. [task-03-schema-store.md](task-03-schema-store.md) — migration 00035, sqlc, store API, executor; delete Go range-diff path
4. [task-04-triage-handler.md](task-04-triage-handler.md) — `REQUEST_TRIAGE`
5. [task-05-writer.md](task-05-writer.md) — patchset identity, fingerprints, GitHub-review baseline, head-only current patchset
6. [task-06-viewed.md](task-06-viewed.md) — viewed hash = patch_id; all-viewed baseline
7. [task-07-head-retention.md](task-07-head-retention.md) — refresh cache clone in place; pin PR heads
8. [task-08-interdiff.md](task-08-interdiff.md) — go-git interdiff + `REQUEST_INTERDIFF`
9. [task-09-ui-triage.md](task-09-ui-triage.md) — `s` triage UI; delete range-diff UI
10. [task-10-ui-interdiff.md](task-10-ui-interdiff.md) — `i` interdiff UI
11. [task-11-docs.md](task-11-docs.md) — docs, roadmap, release note

Between Task 3 and Task 9 the UI's `s` request (`REQUEST_RANGE_DIFF`) has no Go handler. That is expected; Task 9 switches the UI over.
