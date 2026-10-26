# Go package layout — `git`, `prsync` (was `sync`), `store`

Status: proposal (2026-10-26), based on `2df8342`. This is a companion to `2026-10-26-oversized-file-split-design.md`, which uses the paths defined here.

## Why

`internal/git` (17 files + 21 tests), `internal/sync` (11 + 19) and `internal/store` (15 + 13) are crowded. The goal is a tree that reads **scope → responsibility → module → file**, e.g. `internal/store/actionqueue/queue.go`. A folder with only 1–2 files (plus tests) is fine.

Naming follows [The Complete Guide to Go Naming](https://dev.to/valeria_muhembele_b276d49/the-complete-guide-to-go-naming-from-conventions-to-the-art-of-good-names-2efg) and Effective Go:
- Package names are short, lowercase, one word, and say what the package provides (`clonecache`, not `utils`).
- File names are lowercase with underscores and name their content. **Files don't repeat their package name**: `writer/rows.go`, not `writer/writer_rows.go`.
- Exported names don't stutter: `actionqueue.Queue`, not `actionqueue.ActionQueue`. Every caller has to change its import anyway, so the rename is free.

## The Go constraint that shapes this

A folder is a package, and **methods must live in their type's package**. So:
- Everything that is a method on `*git.Client`, `*sync.Orchestrator` or `*store.DB` stays in that root package. Those roots shrink, but they don't disappear.
- Independent types (`Writer`, `Cache`, `AnchorResolver`, `ActionQueue`, …) move out cleanly.
- A subpackage may import its parent, but **never the reverse**. Every edge below was checked against the current file-level reference graph, and the layout is acyclic.

## `internal/git` — getting PR commits onto disk

```
git/                    package git: Client and its clone/fetch/checkout methods
├── client.go           Client, NewClient, Set*, auth, tokenFromEnv       (from clone.go)
├── clone.go            Clone, CloneForPR, WithPRClone, CloneOptions, CloneResult
├── broad_clone.go      broadClone, reuseOnDiskClone, validateCacheHit    (from clone.go)
├── targeted_clone.go   targetedClone, refspecs, depth, required-commit checks (from clone.go)
├── refresh.go          refreshOnDiskClone, fetchIntoRepo, + FetchRefs
├── mergebase.go · localfetch.go · checkout.go                            (unchanged)
├── anchor/             package anchor: comment-anchor resolution
│   ├── resolver.go     Resolver, NewResolver, Request, Result  (was AnchorResolver, ResolveRequest, AnchorResult)
│   ├── position.go     PositionToLine, parseHunkHeader
│   └── hash.go         FileLineHash, anchorHashes
├── clonecache/         package clonecache: the on-disk clone cache
│   ├── cache.go        Cache, New, RepoEntry                    (cache.go + RepoEntry from types.go)
│   ├── paths.go        Root                                     (was CacheRoot)
│   ├── staging.go      temp/stage dir helpers, now exported     (was clonecache.go)
│   ├── maintenance.go  Stats, DeleteDir, SweepOrphans, RepoStat, EntryInput (was cachemaint.go)
│   └── retention.go    PinPRHead(log, path, url, prNumber, head) — takes prNumber, not *CloneOptions, which breaks the cycle
├── diff/               package diff: diff generation and interdiff
│   ├── diff.go         Generate (was Client.GenerateDiff; never used c), FilePatches, SingleFilePatch
│   ├── model.go        File, Options, DefaultOptions, Profile, Result, Status*  (diff types from types.go)
│   ├── interdiff.go    Interdiff(locks, req), InterdiffRequest/Result, HeadTree, ErrHeadUnavailable
│   └── patchid.go      PatchID
├── remote/             package remote: repo identity from a remote URL
│   └── identity.go     Identity, LocationError, ParseURL, SafeRepoID     (was identity.go)
└── repolock/           package repolock: per-repo mutexes
    └── locks.go        Locks, New, Key                                    (was RepoLocks, repoLockKey)
```

Imports: `git → clonecache, diff, remote, repolock`. `clonecache → remote`. `diff → repolock`. `anchor → diff`. `repolock → remote` (`Key` parses the remote URL). `remote` is the only leaf.

- `Client.GenerateDiff` has an unused receiver (smell: **Feature Envy**). Apply Move Method → `diff.Generate`.
- `Client.Interdiff` reads only `c.locks`, so it becomes `diff.Interdiff(locks, req)`. Its only caller is `server/handler/interdiff.go`.
- `git/types.go` is dissolved. Its types go to `clone.go` and `diff/model.go`, and `RepoEntry` goes to `clonecache`.

## `internal/prsync` (was `internal/sync`) — the PR sync orchestrator

Renamed so it no longer shadows stdlib `sync` (`server/app.go` imported that as `stdsync`).

```
prsync/                 package prsync: Orchestrator and its methods
├── orchestrator.go     struct, With* options, Sync phases, SyncPR, syncPRList
├── request.go          SyncRequest, SyncResult                (was types.go)
├── fetch.go            fetch-phase methods, prFetchOptions    (was fetcher.go)
├── anchors.go · actions.go · replay.go                        (drop the orchestrator_ prefix; same package)
├── inbox.go            DiscoverAndSyncInbox & helpers         (from orchestrator.go)
├── workerpool/         package workerpool
│   └── pool.go         Pool, New, DefaultWorkers              (was WorkerPool, NewWorkerPool)
└── writer/             package writer: persists fetch results to SQLite
    ├── writer.go       Writer, New, batch types
    ├── fetched_pr.go   FetchedPR ((was PRFetchResult; the writer's input, so the writer owns it))
    ├── rows.go · patchset.go
    ├── threads.go      review threads + comment dedup         (from writer_details.go)
    └── diffs.go        diff rows + anchor updates             (from writer_details.go)
```

Imports: `prsync → writer, workerpool`. `writer` doesn't import `prsync`, so it needs two shared bits:
- `githubProvider = "github"` (8 uses) becomes `provider.GitHub` (Replace Magic Number with Symbolic Constant). Both packages already import `provider`.
- `sync.truncateSHA` and `git.truncateSHAForLog` are duplicate code. Replace both with one `logging.ShortSHA`.

## `internal/store` — local SQLite cache

```
store/                  package store: DB handle, migrations, *DB query methods
├── db.go               DB, OpenDB, boolToInt (moved here from diff_store.go; used by review_threads.go)
├── migrate.go          MigrationRunner                         (was migration_runner.go)
├── patchsets.go · files_viewed.go · last_reviewed.go · sync_cursor.go · pr_checkout.go
├── review_threads.go   (was review_thread_store.go)
├── db/ · migrations/ · queries/                               (unchanged; sqlc)
├── actionqueue/        queue.go:        Queue, New, Action, MaxRetryCount (was ActionQueue, NewActionQueue, QueuedAction)
├── currentuser/        currentuser.go:  Store, New                      (was CurrentUserStore)
├── dismissal/          dismissal.go:    Store, New                      (was Dismissals)
├── prdiff/             prdiff.go:       File, Store, New                (was DiffFile, DiffStore; avoids clashing with git/diff)
├── repolocation/       repolocation.go: Location, Store, State, New     (was RepoLocation, RepoLocationStore, RepoState)
└── storetest/          storetest.go:    Open(t) *store.DB — migrated in-memory DB
```

- Subpackages import `store` (for `*store.DB`) and `store/db`. `store` imports none of them.
- `localReviewAccountProvider` is shared by `actionqueue` and `last_reviewed.go`, so it becomes `store.LocalReviewAccountProvider`.
- `storetest` follows the `net/http/httptest` idiom. It replaces **7 copies** of "open migrated test DB": `sync/writer_test.go` (×2), `expertise/store_test.go`, `server/blockers_test.go`, `server/repo_location_resolver_test.go`, `handler/readstate_test.go` and `handler/repolocation_scan_test.go`. Store's own internal tests keep `newTestDB`, because importing `storetest` from inside `store` would be a cycle.
- The migration runner stays in the root. Moving it out would force every root test into the external `store_test` package.

## Migration order

**Splits first:** any commit that stages a file over 5k tokens is blocked, and package moves edit imports in `server/app.go`, `git/clone.go`, `sync/writer_test.go`, … So every file split from the split doc runs **before** step 1, in place, under its final name where the package is staying.

Each step is its own commit and must stay green (`go vet ./... && go test -race ./...`). The hook's "move verbatim" rule still applies: renames and package moves are compiler-checked, but logic changes are not allowed in the same commit.

1. Leaves: `logging.ShortSHA`, `provider.GitHub`, `git/remote`, `git/repolock`, `store/storetest`.
2. `git/diff`, then `git/clonecache`, then `git/anchor`.
3. `store/actionqueue`, `currentuser`, `dismissal`, `prdiff`, `repolocation` (one commit each).
4. `sync/workerpool`, then `sync/writer`, then rename `sync` → `prsync` (the subfolders move with it).
5. In-package file renames (`fetcher.go` → `fetch.go`, drop the `orchestrator_` prefix, …) and the "after" cleanups from the split doc.
6. Update `docs/go-project-overview.md` and the `CLAUDE.md` paths (e.g. `store/queries/` stays put, but the handler and repo-ID notes change).

## Decided

- 2026-10-26: `internal/sync` is renamed to `internal/prsync`.
