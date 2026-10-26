# Go package layout + oversized file split — plan

Specs (binding):
- `docs/superpowers/specs/2026-10-26-oversized-file-split-design.md` (**split spec**)
- `docs/superpowers/specs/2026-10-26-go-package-layout-design.md` (**layout spec**)

Each task names the spec section it implements. Read that section before starting; it holds the exact file names, symbol lists and line ranges.

## Global constraints

- Go module path: read `go/go.mod` (it's `github.com/itterno/bistro`). Never guess it.
- **Moves are verbatim.** File splits and package moves change no behaviour. Renames, exports and import edits are allowed because the compiler checks them. Logic changes happen only in Task 14.
- **Pre-commit hook is active** (`core.hooksPath=.githooks`): a staged file over 5,000 tokens (bytes/4) blocks the commit. **Never use `--no-verify`.** If a commit is blocked, split the offending file. That's why the splits (Tasks 2–5) run before the package moves.
- New files target ≤ ~3,000 tokens. Check with `echo $(( $(wc -c < FILE) / 4 ))`.
- File names describe their content and don't repeat their package name. Exported names don't stutter (layout spec "Why").
- Tests move with the code they cover. A test file that only tests a moved type moves to that type's package.
- Generated sqlc code (`go/internal/store/db/`) is never hand-edited.
- Verify before every commit: `cd go && go vet ./... && go test -race ./...`. Electron is untouched; don't run it.
- Commit message format: `refactor: <what>` (or `test:` / `docs:` / `chore:`), one commit per logical step.
- If a spec line range has drifted, go by the symbol names; they are authoritative.

## Task 1: Hook ignore list

Implements the split spec's "Non-code files" table, rows 1–3 only. In `.githooks/pre-commit`, extend `IGNORE_REGEX` with:
- `^Bistro Design System/_`
- `^LICENSE$`
- `^docs/superpowers/(plans|specs)/`

Keep the comment above it accurate. Verify by staging nothing and running the hook's loop logic mentally, or test with `git add -n`. One commit: `chore: exempt generated bundles, LICENSE and plan/spec records from size hook`.

## Task 2: Split `server/app.go`

Split spec, section "`server/app.go`". Stay in package `server`. Target files: `app.go`, `provider.go`, `config.go`, `llm.go`, `expertise.go`, `bridge.go`, `repo_location.go`, `handlers.go`. Move code verbatim; no Extract Class. One commit.

## Task 3: Split `sync/orchestrator.go` and `git/clone.go`

Split spec, sections "`sync/orchestrator.go`" and "`git/clone.go`", **file splits only**:
- `sync/inbox.go`
- `git/client.go`, `git/clone.go`, `git/broad_clone.go`, `git/targeted_clone.go`, and `FetchRefs` + `localRepoExists` into `git/refresh.go`
- Rename `git/clone_targeted_test.go` → `git/targeted_clone_test.go`

Leave `truncateSHAForLog` and `repoLockKey` where they land; Task 6 replaces the first and Task 7 the second. Two commits, one per package.

## Task 4: Split `sync/writer_test.go` and `sync/blocker_fixes_test.go`

Split spec, sections "`sync/writer_test.go`" and "`sync/blocker_fixes_test.go`". The writer tests stay in package `sync` for now and use a `writer_` prefix so Task 12 can drop it: `writer_test.go`, `writer_mention_test.go`, `writer_threads_test.go`, `writer_diffs_test.go`, `writer_savepoint_test.go` (extend the existing file), `writer_record_patchset_test.go`, `writer_anchors_test.go`, `writer_comment_position_test.go`, `writer_rows_test.go`. Don't create `writer_anchors_test.go` as `anchor_test.go`; `sync/anchor_test.go` already exists.

Dissolve `blocker_fixes_test.go` into `helpers_test.go`, `actions_test.go`, `replay_test.go` and `replay_ambiguous_test.go`. Verbatim; cleanups wait for Task 14. Two commits.

## Task 5: Split the remaining oversized test files

Split spec, sections for `actions/executor_test.go`, `store/db_test.go`, `server/handler/comments_new_columns_test.go` and `git/anchor_test.go`. Use the spec's file names, except for `git/anchor_test.go`: since it moves later, split it into `git/anchor_position_test.go`, `git/anchor_hash_test.go` and `git/anchor_resolver_test.go`. Verbatim. One commit per source file.

## Task 6: Shared leaves

Layout spec, "Migration order" step 1, the parts not under `git/`:
- `logging.ShortSHA` in `go/internal/logging/`, replacing `sync.truncateSHA` and `git.truncateSHAForLog`. Check that both bodies are identical first. If they differ, keep both and report it as a concern.
- `provider.GitHub = "github"`, replacing `sync`'s `githubProvider` const.
- `store/storetest` with `Open(t testing.TB) *store.DB` (in-memory, migrated, closed via `t.Cleanup`). It replaces the 7 helpers listed in the layout spec's store section. Keep a local helper wherever its body does more than open + migrate (e.g. migrate *through a version*). `newSyncTestDB` and `openTestDB` both go.

One commit per bullet.

## Task 7: `git/remote` and `git/repolock`

Layout spec, the `git` tree: the `remote/` and `repolock/` rows. Renames: `Identity` stays, `ParseRemoteURL` → `ParseURL`, `SafeRepoID` stays, `RepoLocks` → `Locks`, `NewRepoLocks` → `New`, `repoLockKey` → `Key`. Move `identity_test.go` and `locks_test.go`, and update every importer (`cmd/`, `provider/github`, `scan`, `server`, `handler`, `sync`). Two commits.

## Task 8: `git/diff`

Layout spec, the `diff/` row and its bullets. `Client.GenerateDiff` → `diff.Generate`. `Client.Interdiff` → `diff.Interdiff(locks *repolock.Locks, req)`. Export `HeadTree`, `FilePatches` and `SingleFilePatch` as needed by `git` root and anchor. The diff types from `git/types.go` go to `diff/model.go`, renamed: `DiffFile` → `File`, `DiffOptions` → `Options`, `DefaultDiffOptions` → `DefaultOptions`, `DiffProfile` → `Profile`, `DiffResult` → `Result`. `CloneResult` moves to `git/clone.go`. Move the matching tests. One commit.

## Task 9: `git/clonecache`

Layout spec, the `clonecache/` row. Includes `RepoEntry`, renames (`NewCache` → `New`, `CacheRoot` → `Root`, `CacheStats` → `Stats`, `DeleteCacheDir` → `DeleteDir`, `CacheRepoStat` → `RepoStat`, `CacheEntryInput` → `EntryInput`), and exporting the staging helpers that `git` root uses. `pinPRHead` → `PinPRHead` takes `prNumber int` instead of `*CloneOptions`; this is the only signature change, and it's needed to avoid an import cycle. After this task `git/types.go` must be gone. Move `cache_test.go`, `cachemaint_test.go` → `maintenance_test.go`, `paths_test.go` and `retention_test.go`. One commit.

## Task 10: `git/anchor`

Layout spec, the `anchor/` row. Split `anchor.go` into `resolver.go`, `position.go` and `hash.go`. Renames: `AnchorResolver` → `Resolver`, `NewAnchorResolver` → `NewResolver`, `AnchorResult` → `Result`, `ResolveRequest` → `Request`. Move the three `anchor_*_test.go` files from Task 5 as `position_test.go`, `hash_test.go` and `resolver_test.go`. One commit.

## Task 11: Store subpackages

Layout spec, the store tree and bullets. Five packages, one commit each: `actionqueue`, `currentuser`, `dismissal`, `prdiff`, `repolocation`, with the renames listed in the tree. Export `store.LocalReviewAccountProvider`, and move `boolToInt` into `store/db.go`. Rename `review_thread_store.go` → `review_threads.go` and `migration_runner.go` → `migrate.go` (with its test, if one exists by name). Move each store's tests. Tests that need a migrated DB use `storetest.Open`.

## Task 12: `sync/workerpool` and `sync/writer`

Layout spec, the prsync tree's `workerpool/` and `writer/` rows. Renames: `WorkerPool` → `Pool`, `NewWorkerPool` → `New`, `PRFetchResult` → `writer.FetchedPR`, `NewWriter` → `writer.New`. Writer files: `writer.go`, `fetched_pr.go`, `rows.go`, `patchset.go`, plus `writer_details.go` split into `threads.go` and `diffs.go`. Move the writer tests and drop the `writer_` prefix (e.g. `writer_mention_test.go` → `mention_test.go`). Two commits.

## Task 13: Rename `sync` → `prsync`

`git mv go/internal/sync go/internal/prsync` (subfolders come along), package `prsync`, and update all importers. Drop the `stdsync` alias in `server/app.go` and wherever else it was only needed for the clash. In-package renames: `fetcher.go` → `fetch.go`, `types.go` → `request.go`, `orchestrator_{actions,anchors,replay}.go` → `actions.go`, `anchors.go`, `replay.go`, and the matching test files where the names clash or carry the prefix. One commit for the move, one for the file renames.

## Task 14: Cleanups

These are the "After" items from the split spec. One commit per item. These are the only logic-touching changes, so each must keep the suite green.
1. Delete `var _ = time.Now` (now in a `prsync` test file).
2. Replace `containsStr` with `strings.Contains`.
3. Parameterize the duplicate executor transport-error tests into table tests (`actions/transport_errors_test.go`) without dropping any case.
4. Keep one of `mustExecDB` (handler) and `mustExec` (`dismiss_test.go`).
5. Compare `NullAnchorPatchsetNumber_Stale` and `…_AlwaysStale`. Merge them only if they assert the same thing; otherwise leave both and say why in the report.

## Task 15: Docs

- Split spec, "Non-code files" rows 4–6: split `docs/ROADMAP.md`, split and update `docs/go-project-overview.md` (the directory structure and package summary must match the new layout), and move `2026-07-12-nih-audit.md` to `docs/audits/`. The hook allows `--no-verify` for the audit move only, with the reason in the commit body.
- Update `CLAUDE.md` wherever it names moved Go paths or packages (`go/internal/sync`, store helpers, etc.).
- Every resulting doc must be under 5k tokens, except the nih-audit.
