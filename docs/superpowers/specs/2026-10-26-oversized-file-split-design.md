# Oversized file split — proposal

Status: proposal (2026-10-26). Line ranges are as of `2df8342`. Target paths follow `2026-10-26-go-package-layout-design.md`. Run these splits **first**, in place (the hook blocks any later import edit to an oversized file). Files that later move into a subpackage get a temporary prefix, e.g. `sync/writer_mention_test.go` becomes `prsync/writer/mention_test.go`.

## Why

`.githooks/pre-commit` blocks staged files over 5,000 tokens (bytes/4), so that an agent can read any file in one pass. 25 tracked files are over that limit. The `--no-verify` exception for `server/app.go` has been revoked, so these files must be split before anyone edits them, not bypassed.

Vocabulary comes from refactoring.guru ([smells](https://refactoring.guru/refactoring/smells), [techniques](https://refactoring.guru/refactoring/techniques)).

## Ground rules

1. **Two commits per file.** Commit A moves code verbatim (Move Method / Extract Class at file level). Commit B, when needed, applies the cleanups listed under "after".
2. **File names describe content and don't repeat the package** (`writer/rows_test.go`). Tests sit beside the code they cover.
3. **Target ≤ ~3,000 tokens per new file.** Extend an existing sibling only if it stays under ~4,000.
4. Verify every commit: `cd go && go vet ./... && go test -race ./...`.

Order: `server/app.go` (the revoked exception) → `sync/orchestrator.go` → `git/clone.go` → tests → hook config.

---

## Production code

### `server/app.go` (~7,955 tok)

Smells: **Large Class** (`App` has ~40 fields) and **Divergent Change**. All files stay in package `server`. The `app.go` entry keeps the struct, `New*`, `openDatabase`, `prepareStores` and `Run` (~2,200 tok).

| File | Contents | ~tok |
|---|---|---|
| `provider.go` | teardown/attach orchestrator, `SetProvider`, replay, `current*` snapshots, `initializeStartupProvider` | 1,900 |
| `config.go` | Setters called by `main`: `SetReadyWriter`, `SetGitClient`, `SetRulesDir`, `SetSyncInterval`, `SetAutoSync`, `SetGitHubClient*`, `SetAuthCallbackTimeout`, `githubToken` | 700 |
| `llm.go` | `slogPrintf`, `liveByokClient`, `prepareLLMTier` | 500 |
| `expertise.go` | identity, prepare, broadcast/nudge, `registerExpertiseHandlers` | 1,000 |
| `bridge.go` | `wsEventMsg`, `startEventBridge`; pairs with `bridge_test.go` | 400 |
| `repo_location.go` | configure store, clone, `registerRepoLocationHandlers` | 700 |
| `handlers.go` | `register{Auth,Review,DiffAndCheckout}Handlers` | 1,100 |

After (optional): the provider-generation fields (`prov`, `orchestrator`, `provMu`, `provCtx*`, `bridgeCancel`) form a **Data Clump**. Extract Class → a `providerSession` struct, but only if `provider.go` keeps growing.

### `sync/orchestrator.go` (~5,106 tok)

Move inbox discovery (`SupportsInboxDiscovery`, `DiscoverAndSyncInbox`, `inboxDiscoveryOptions`, `writeDiscoveredPRs`, `recordInboxCursorError`; lines 97–103 and 415–515) to `sync/inbox.go` (~1,000). That leaves ~4,100. The next candidate, if it grows: the `With*` block → `sync/options.go`.

### `git/clone.go` (~6,744 tok)

Smell: **Divergent Change**. The broad and targeted strategies change independently.

| File | Contents (lines) | ~tok |
|---|---|---|
| `git/client.go` | `LocalRepoResolver`, `Client`, `NewClient`, `Set*`, `auth`, `tokenFromEnv` (21–89, 717–727) | 800 |
| `git/clone.go` | consts, `CloneOptions`, public `Clone*` / `WithPRClone`, `cloneWithOptions*`, `resolveLocal`, `recordCacheCloned` (90–309) | 2,000 |
| `git/broad_clone.go` | `broadClone`, `reuseOnDiskClone`, `validateCacheHit` (310–464) | 1,400 |
| `git/targeted_clone.go` | `requiredSHAs` … `firstMissingCommitInRepo` (465–659); pairs with `targeted_clone_test.go` (was `clone_targeted_test.go`) | 1,800 |
| `git/refresh.go` (604 → ~1,300) | `FetchRefs`, `localRepoExists` (660–716) | — |

`truncateSHAForLog` becomes `logging.ShortSHA` (see the layout doc), and `repoLockKey` becomes `repolock.Key`.

---

## Test code

The main smell is **Large Class**: one test file covering several production responsibilities. Move Method puts each test next to its subject.

### `sync/writer_test.go` (~16,479 tok) → `sync/writer/`

| File | Contents (lines) | ~tok |
|---|---|---|
| `writer_test.go` | `mustWritePR`, creation / validation / aggregation, `TestWriteSyncResults*` (21–27, 298–364, 380–592) | 2,600 |
| `mention_test.go` | `mentionedTest*` helpers + 10 `TestWriter*Mentioned*` (28–297) | 2,400 |
| `threads_test.go` | `TestWriteReviewThreads_*` (593–699, 800–938) | 2,200 |
| `diffs_test.go` | `makePR`, `countDiffFiles`, `TestWriteDiffs_*` (939–1137) | 1,750 |
| `savepoint_test.go` (was `writer_savepoint_test.go`, 265 → ~1,400) | savepoint rollback tests (1138–1261) | — |
| `record_patchset_test.go` | `TestRecordPatchsetIfChanged_*` (700–799). Not merged into `patchset_test.go`, which is at 3,756. | 900 |
| `anchors_test.go` | `TestWriteAnchorUpdates_*` (1262–1409, 1617–1766) | 2,600 |
| `comment_position_test.go` | `TestWriteComment_*`, `TestWriteThreadComment_*` (1410–1616) | 1,800 |
| `rows_test.go` | `mustTime`, `stringValue`, `TestWritePRRowListOnlyRevisionSemantics` (1767–1873) | 950 |

The other existing files drop their prefix: `writer_classify_test.go` → `classify_test.go`, `writer_dedup_test.go` → `dedup_test.go`, `writer_patchset_test.go` → `patchset_test.go`.

After: `openTestDB` and its **Middle Man** wrapper `newSyncTestDB` both become `storetest.Open`.

### `sync/blocker_fixes_test.go` (~7,923 tok)

Smell: **Comments**. The file name is a history label, so dissolve it by subject:

| File | Contents (lines) | ~tok |
|---|---|---|
| `sync/helpers_test.go` | `mustExecSync`, `insertPRForSync`, `insertThreadSync`, `prepareReplyThreadFixture` | 500 |
| `sync/actions_test.go` | `TestQueuedActionID_*`, `TestApplyAction_*`, liveness and thread-success tests (69–206, 292–360, 707–807) | 2,900 |
| `sync/replay_test.go` | the remaining `TestReplayQueuedActions_*`, `TestReplayTimestamp*`, `blockingPostCommentProvider` (215–291, 575–706, 808–852) | 2,400 |
| `sync/replay_ambiguous_test.go` | `…_Ambiguous*`, `…_ContextCancelledProviderError_IsTerminal` (361–574) | 1,900 |

After: delete `var _ = time.Now` (line 207, **Dead Code**). Replace `containsStr` with `strings.Contains` (**Substitute Algorithm**).

### `actions/executor_test.go` (~9,211 tok)

| File | Contents (lines) | ~tok |
|---|---|---|
| `executor_test.go` | basic execute / retry / events / multi (18–157) | 1,300 |
| `review_tracking_test.go` | stubs + `TestExecutor_ReviewTracking_*` (158–350) | 1,800 |
| `cancel_test.go` | context cancellation and side-effect guards (351–566) | 2,000 |
| `transport_errors_test.go` | ambiguous / rate-limit / permanent classification (567–821, 857–985) | 3,500 |
| `types_test.go` (912 → ~1,400) | `TestIsRateLimit_*` (822–856, 986–1005); `IsRateLimit` lives in `types.go` | — |

After: the near-identical GitHub 500/502/503 tests and the per-action `RecoverableTransportError` trio are **Duplicate Code**. Apply **Parameterize Method** to turn them into table tests, which cuts about a third.

### `store/db_test.go` (~8,332 tok)

| File | Contents | ~tok |
|---|---|---|
| `db_test.go` | `newTestDB`, `mustExec`, `migrateEmbeddedSchemaToVersion`, `mustInsertPR`, `tableColumns`, `assertColumnExists`, `TestOpenDB` | 1,100 |
| `migrate_test.go` | `TestMigrations*`, goose metadata / version / seeding tests, `assert{All,No}EmbeddedVersionsApplied` | 2,000 |
| `reconcile_test.go` | `TestMigrationRunnerReconciles*` / `Repairs*` / `DoesNotReconcilePre31`, `TestMigration033IsIdempotent…`, `TestCurrentSchemaExistsRejectsPre31Schema`, `TestReconcilePreservesMigrationsNewerThanBaseline` (214–412, 449–519) | 2,500 |
| `migration_versions_test.go` | `TestMigration0{12,14,15,16,20,23,33,34}*`, `TestRateLimitStateProviderKeyMigration` | 3,000 |

### `server/handler/comments_new_columns_test.go` (~9,660 tok)

Every test here is `TestRequestComments_*` covering anchor visibility. "new columns" is a history name.

| File | Contents (lines) | ~tok |
|---|---|---|
| `comments_anchor_test.go` | `ReturnsNewColumns`, `Anchored{True,False}*`, `OutdatedComment` (28–253) | 2,500 |
| `comments_anchor_cache_test.go` | stale/fresh cache, `StaleOutdatedTrue_NotLeaked`, retarget stale/fresh (254–491) | 2,600 |
| `comments_anchor_original_test.go` | `OriginalPositionAnchor_*` (537–624, 748–833) | 1,900 |
| `comments_anchor_current_test.go` | `CurrentPositionAnchor_Fresh`, `SameHeadRetarget_*`, `Null{Anchor,Current}PatchsetNumber_*` (492–536, 625–747, 834–867) | 2,200 |

After:
- `newHandlerTestDB` becomes `storetest.Open`.
- `mustExecDB` duplicates `mustExec` in `dismiss_test.go`; keep one.
- `NullAnchorPatchsetNumber_Stale` and `…_AlwaysStale` overlap; merge them if they assert the same thing.

### `git/anchor_test.go` (~5,283 tok) → `git/anchor/`

| File | Contents | ~tok |
|---|---|---|
| `position_test.go` | `build{Two,Three}CommitRepo`, `TestPositionToLine_*`, `TestParseHunkHeader` | 2,500 |
| `hash_test.go` | `TestFileLineHash_*` | 500 |
| `resolver_test.go` | `resolveCases`, `TestAnchorResolver_*`, `TestResolve_WithDiffBaseSHA_*` | 2,300 |

---

## Non-code files

Splitting prose doesn't serve the hook's purpose, so handle these in config:

| File(s) | Proposal |
|---|---|
| `Bistro Design System/_ds_bundle.js`, `_ds_manifest.json`, `_adherence.oxlintrc.json` | Generated bundle output (`@ds-bundle` header). Add `^Bistro Design System/_` to `IGNORE_REGEX`. |
| `LICENSE` | Add to `IGNORE_REGEX`. |
| `docs/superpowers/{plans,specs}/*` (9 files) | Write-once historical records. Add `^docs/superpowers/(plans\|specs)/` to `IGNORE_REGEX`. New plans should still aim for under 5k tokens. |
| `docs/ROADMAP.md` (~14.7k) | Move the finished phases 0–5 to `docs/roadmap/phase-N.md`. The index keeps a status line per phase, the open phases 6–7, out-of-scope, open questions and the checklist. |
| `docs/go-project-overview.md` (~7.7k) | Move "Issues and Refactoring Suggestions" and "Proposed Final Layout" to `docs/go-refactoring-notes.md`. Rewrite the layout section to match the new package layout. |
| `2026-07-12-nih-audit.md` | Move to `docs/audits/` with `--no-verify` and a reason ("prose audit"), as the hook allows. |

## Not in scope

- `store/diff_store_test.go:368` `containsString` also hand-rolls `strings.Contains`.
- `isRateLimitError` (`sync`) and `ActionResult.IsRateLimit` (`actions`) are not duplicates. One reads typed errors and the other matches strings.
