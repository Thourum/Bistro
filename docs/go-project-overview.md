# Bistro Go Backend: Project Overview

## What Is This?

Bistro is a local-first PR dashboard. The Go backend (`go/`) is a sync daemon (`bistro-sync`) that:

1. Authenticates with GitHub via OAuth
2. Periodically syncs PR data (metadata, comments, CI status, diffs) into a local SQLite database
3. Exposes a WebSocket API for an Electron frontend to query and interact with PRs
4. Supports offline queueing of write actions (approve, request changes, comment) with automatic retry

---

## Current Directory Structure

```
go/
├── cmd/
│   └── bistro-sync/
│       └── main.go                     # CLI entrypoint: flags, logging, keychain bootstrap
│
├── internal/
│   ├── actions/                        # PR write-action execution & retry logic
│   │   ├── types.go                    #   ActionCommand, ActionRequest, ActionResult, IsRecoverable
│   │   ├── handlers.go                 #   Handler interface + ApproveHandler, RequestChangesHandler, PostCommentHandler
│   │   ├── executor.go                 #   ActionExecutor: dispatches to handlers, emits events, updates review_status in DB
│   │   ├── retry.go                    #   ExponentialBackoff, RetryWithBackoff
│   │   ├── integration_test.go
│   │   ├── executor_test.go
│   │   ├── handlers_test.go
│   │   ├── retry_test.go
│   │   └── types_test.go
│   │
│   ├── auth/                           # OAuth authorization code flow
│   │   ├── oauth.go                    #   ProviderConfig, BuildAuthURL
│   │   ├── callback.go                 #   CallbackServer (temp HTTP server), ExchangeToken, scope validation
│   │   ├── oauth_test.go
│   │   └── callback_test.go
│   │
│   ├── cache/                          # Filesystem-backed provider username cache
│   │   ├── providers.go                #   ProviderCache (JSON file, 30-day expiry)
│   │   └── providers_test.go
│   │
│   ├── events/                         # Shared event types & broadcaster
│   │   └── events.go                   #   Event, EventEmitter, NewProgressEvent, NewCompleteEvent, NewErrorEvent
│   │
│   ├── expertise/                      # Local git-history expertise scoring
│   │   ├── engine.go                   #   Enable/disable, restart, touches-you resolver, shutdown
│   │   ├── gitcli.go                   #   Bounded system-git wrapper for blame/log/check-attr
│   │   ├── indexer.go                  #   Full-repo indexing pass with progress events
│   │   ├── updater.go                  #   Incremental PR-touched-file rescoring
│   │   ├── score.go                    #   Pure scoring math and expert/dormancy thresholds
│   │   ├── identity.go                 #   Current-user alias and noreply matching
│   │   ├── excludes.go                 #   Built-in and user exclude glob matching
│   │   ├── nudge.go                    #   10-reviewed-PR enable nudge
│   │   └── store.go                    #   Thin wrapper over sqlc expertise queries
│   │
│   ├── git/                            # Git clone, diff generation, repo caching, repo identity
│   │   ├── types.go                    #   RepoEntry, CloneResult, DiffFile, DiffResult
│   │   ├── clone.go                    #   Client.Clone (go-git based)
│   │   ├── diff.go                     #   Client.GenerateDiff (unified diff from two SHAs)
│   │   ├── patchid.go                  #   Rebase-stable per-file patch fingerprints
│   │   ├── mergebase.go                #   MergeBase and shallow-clone deepening
│   │   ├── retention.go                #   Pin observed PR heads across force-pushes
│   │   ├── interdiff.go                #   Client.Interdiff (one file between patchset heads)
│   │   ├── cache.go                    #   In-memory repo cache with TTL
│   │   ├── cachemaint.go               #   Cache stats, orphan sweep, targeted cache deletion
│   │   ├── identity.go                 #   ParseRemoteURL and SafeRepoID for stable repo identities
│   │   ├── localfetch.go               #   Fetch PR refs into user-owned local clones
│   │   ├── locks.go                    #   Per-repo operation locks for clone/fetch/cache maintenance
│   │   ├── paths.go                    #   Cache directory management
│   │   ├── clone_test.go
│   │   ├── clone_local_test.go
│   │   ├── clone_targeted_test.go
│   │   ├── diff_test.go
│   │   ├── cache_test.go
│   │   ├── cachemaint_test.go
│   │   ├── identity_test.go
│   │   ├── localfetch_test.go
│   │   ├── locks_test.go
│   │   ├── paths_test.go
│   │   ├── git_test.go
│   │   └── data/                       #   Test fixtures (patches, repos)
│   │
│   ├── keychain/                       # OS keychain token storage (via go-keyring)
│   │   ├── keychain.go                 #   StoreToken, GetToken, DeleteToken (low-level)
│   │   ├── token.go                    #   TokenManager (high-level: Store/Get/Delete per provider)
│   │   └── keychain_test.go
│   │
│   ├── provider/                       # Provider abstraction layer
│   │   ├── interface.go                #   Provider interface (ListPRs, GetPR, Approve, etc.) + PRFilters
│   │   ├── models.go                   #   Domain types: PR, Comment, Repo, CIStatus, Check
│   │   ├── errors.go                   #   ProviderError, ErrorType, WrapHTTPError, IsRecoverable
│   │   ├── rate_limit.go               #   RateLimitState, DetectRateLimitFrom429
│   │   ├── interface_test.go
│   │   ├── errors_test.go
│   │   ├── rate_limit_test.go
│   │   │
│   │   ├── github/                     #   GitHub implementation of Provider
│   │   │   ├── client.go               #     Client struct, NewClient, GetUsername
│   │   │   ├── provider_impl.go        #     Interface method implementations (ListPRs, GetPR, etc.) + parsePRID
│   │   │   ├── prs.go                  #     listPRs, getPR, normalizePR
│   │   │   ├── comments.go             #     getComments, normalizeComment
│   │   │   ├── ci.go                   #     getCIStatus, normalizeCheckRun, aggregateStatus
│   │   │   ├── actions.go              #     approve, requestChanges, postComment
│   │   │   ├── list_repos.go           #     ListUserRepos
│   │   │   ├── client_auth_test.go
│   │   │   ├── provider_impl_test.go
│   │   │   ├── prs_test.go
│   │   │   ├── comments_test.go
│   │   │   ├── ci_test.go
│   │   │   ├── actions_test.go
│   │   │   ├── list_repos_test.go
│   │   │   ├── list_prs_test.go
│   │   │   └── user_test.go
│   │   │
│   │   └── mock/                       #   Mock provider for tests
│   │       ├── provider_mock.go
│   │       └── provider_mock_test.go
│   │
│   ├── scan/                           # Developer-folder scanning for local Git repos
│   │   ├── scanner.go                  #   ValidateDevFolder, Scan, repo identity extraction
│   │   └── scanner_test.go
│   │
│   ├── server/                         # App orchestrator + WebSocket handlers
│   │   ├── app.go                      #   App struct, Run(), dependency wiring, handler registration
│   │   ├── ready.go                    #   Engine readiness event payload
│   │   ├── repo_location_resolver.go   #   Chooses local clone vs Bistro cache for repo operations
│   │   ├── syncloop.go                 #   Periodic sync loop and watched-repo sync
│   │   └── handler/                    #   Domain WebSocket handlers
│   │       ├── auth.go                 #     AUTH_START, AUTH_CHECK, AUTH_DISCONNECT
│   │       ├── comments.go             #     Comment CRUD / thread operations
│   │       ├── diff.go                 #     REQUEST_DIFF → DIFF_READY handler
│   │       ├── patchsets.go            #     Patchset metadata handlers
│   │       ├── pr.go                   #     REQUEST_PR_LIST → PR_LIST_READY handler
│   │       ├── triage.go               #     REQUEST_TRIAGE → TRIAGE_READY (since-last-review file triage)
│   │       ├── interdiff.go             #     REQUEST_INTERDIFF → INTERDIFF_READY (per-file interdiff)
│   │       ├── repo.go                 #     REQUEST_REPO_LIST + TOGGLE_WATCH_REPO handlers
│   │       ├── repolocation.go         #     Repo-location scan/add/remove/recover/cache-maintenance handlers
│   │       ├── sync.go                 #     Manual sync handlers
│   │       └── threads.go              #     Review-thread handlers
│   │
│   ├── store/                          # SQLite database layer
│   │   ├── db.go                       #   DB wrapper (OpenDB, pragmas: FK, WAL)
│   │   ├── action_queue.go             #   Offline write-action queue persistence
│   │   ├── current_user.go             #   Current authenticated user persistence
│   │   ├── diff_store.go               #   DiffStore: SaveDiffs, GetDiffs, GetUnifiedDiff
│   │   ├── files_viewed.go             #   Viewed-file state
│   │   ├── last_reviewed.go            #   Last-reviewed commit/patchset state
│   │   ├── patchsets.go                #   Patchset persistence
│   │   ├── repo_location.go            #   RepoLocationStore: repo source/local/cache columns
│   │   ├── review_thread_store.go      #   Review-thread persistence
│   │   ├── sync_cursor.go              #   Sync cursor persistence
│   │   ├── migration_runner.go         #   Embedded SQL migrations, version tracking
│   │   └── migrations/                 #   Numbered .sql migration files, including repo-location columns
│   │
│   ├── sync/                           # Sync engine: orchestration, concurrency, DB writes, queueing
│   │   ├── types.go                    #   SyncRequest, SyncResult, SyncProgress
│   │   ├── orchestrator.go             #   Orchestrator: Sync(), ApplyAction, ReplayQueuedActions, syncPRList
│   │   ├── concurrent.go              #   WorkerPool, PRFetchResult, FetchPRDetails, SyncPRsWithDetails
│   │   ├── writer.go                  #   Writer: WriteSyncResults (batch upsert PRs + comments)
│   │   ├── queue.go                   #   ActionQueue: Enqueue, GetPending, MarkCompleted/Failed, cleanup
│   │   ├── events.go                  #   Thin aliases re-exporting events.EventEmitter for backward compat
│   │   ├── orchestrator_test.go
│   │   ├── orchestrator_integration_test.go
│   │   ├── integration_test.go
│   │   ├── concurrent_test.go
│   │   ├── writer_test.go
│   │   ├── queue_test.go
│   │   └── events_test.go
│   │
│   └── ws/                             # WebSocket transport layer
│       ├── server.go                   #   Server: Start, Stop, Broadcast, conn management
│       ├── dispatch.go                 #   Dispatcher, InboundMessage, MessageHandler, Register/Dispatch
│       └── server_test.go
│
├── go.mod
├── go.sum
└── logs/
```

---

## Data Flow

```
                         ┌──────────────┐
                         │   Electron   │
                         │   Frontend   │
                         └──────┬───────┘
                                │ WebSocket
                                ▼
                         ┌──────────────┐
                         │  ws.Server   │
                         │  Dispatcher  │
                         └──────┬───────┘
                                │ routes messages by type
                ┌───────────────┼───────────────┐
                ▼               ▼               ▼
         ┌─────────────┐ ┌───────────┐  ┌────────────┐
         │ Auth         │ │ Handlers  │  │ Sync Loop  │
         │ Handlers     │ │ (diff,    │  │ (periodic  │
         │ (in app.go)  │ │  PR list, │  │  + trigger)│
         └──────┬───────┘ │  repo,    │  └─────┬──────┘
                │         │  watch)   │        │
                ▼         └─────┬─────┘        ▼
         ┌───────────┐         │       ┌──────────────┐
         │  keychain  │         │       │ Orchestrator │
         │  auth/     │         │       │  (sync/)     │
         │  cache/    │         │       └──────┬───────┘
         └───────────┘         │              │
                               │    ┌─────────┼──────────┐
                               │    ▼         ▼          ▼
                               │ provider  git.Client  actions/
                               │ (GitHub)  (clone+diff) (approve,
                               │                        comment)
                               ▼
                        ┌──────────────┐
                        │   SQLite DB  │
                        │   (store/)   │
                        └──────────────┘
```

---

## Package-by-Package Summary

### `cmd/bistro-sync` -- Entrypoint
Parses CLI flags, configures logging, checks the OS keychain for an existing token, and calls `server.App.Run()`.

### `internal/server` -- Application Core
`app.go` owns the application dependency graph, lifecycle, WebSocket server setup, and handler registration. Background sync logic lives in `syncloop.go`; readiness payloads live in `ready.go`; repo path resolution lives in `repo_location_resolver.go`.

The `handler/` subpackage contains focused `ws.MessageHandler` factories grouped by domain: auth, comments, diff, patchsets, PR list, triage, interdiff, repos, repo-location management, sync, and review threads. Repo-location handlers cover dev-folder validation/scanning, add-by-URL, removal preflight/removal, local-path recovery, cache recovery, cache stats, prune, clear, and targeted cache deletion.

### `internal/ws` -- WebSocket Transport
Clean, focused package. `Server` manages connections and lifecycle; `Dispatcher` routes JSON messages to registered `MessageHandler` functions. No business logic.

### `internal/provider` -- Provider Abstraction
Defines the `Provider` interface and domain models (`PR`, `Comment`, `CIStatus`, `Repo`). Also contains error typing (`ProviderError`) and rate-limit detection. The `github/` subpackage implements all interface methods.

### `internal/provider/github` -- GitHub API Client
The `Client` wraps `go-github` and is split across 7 files:
- `client.go` -- constructor + `GetUsername`
- `provider_impl.go` -- interface method routing + `parsePRID`
- `prs.go` -- PR list/get + normalize
- `comments.go` -- comment list + normalize
- `ci.go` -- check runs + normalize + aggregate
- `actions.go` -- approve, request changes, post comment
- `list_repos.go` -- list authenticated user repos

### `internal/sync` -- Sync Engine
Orchestrates the full sync cycle (list PRs, fetch details concurrently, write to DB). Contains:
- Orchestrator (main sync + action dispatch)
- WorkerPool (bounded concurrency)
- Writer (batch DB upserts)
- ActionQueue (offline queue with retry/cleanup)
- Event aliases (thin wrappers around `events` package)

A new observed patchset is recorded when the head or a known merge base changes, not when only the base tip moves. Patchsets store `merge_base_sha` and per-file `patch_id`s in `patchset_files`; `has_fingerprints` marks snapshots whose fingerprints were recorded after a merge-base diff. The since-last-review baseline is the newest of the user's submitted GitHub review (from any client), Bistro approve/request-changes/top-level comment, or mark-viewed action leaving no file unviewed.

### `internal/actions` -- PR Action Execution
Handles approve/request-changes/post-comment actions through a `Handler` interface pattern with exponential backoff retry.

### `internal/auth` -- OAuth Flow
`BuildAuthURL` constructs the authorization URL; `CallbackServer` is a temporary localhost HTTP server that receives the redirect, exchanges the code for a token, and validates scopes. `ExchangeToken` has two paths: with a client secret (dev builds) it form-posts to GitHub's token URL; without one (packaged builds) it posts JSON `{code, redirect_uri}` to `https://get-bistro.com/api/oauth/token`, which holds the secret.

### `internal/events` -- Event System
Simple pub/sub `EventEmitter` with buffered channels. Used by both sync and actions packages.

### `internal/store` -- Database Layer
SQLite via `mattn/go-sqlite3`. `DB` is a thin wrapper applying pragmas (FK, WAL). Store types own persistence for diffs, review threads, patchsets, viewed files, current user, sync cursors, queued write actions, and repo locations. `RepoLocationStore` manages the repo-location columns added by migration 026 (`repo_source`, `local_path`, `local_path_verified_at`, `cache_path`) so the engine can prefer user-owned clones while retaining Bistro cache fallback. `MigrationRunner` applies embedded `.sql` migrations.

### `internal/git` -- Git Operations
Uses `go-git` for cloning repos and generating unified diffs between two SHAs. Includes an in-memory cache with TTL for cloned repos, remote identity normalization (`ParseRemoteURL`, `SafeRepoID`), per-repo operation locks, local-clone PR ref fetching, and cache-maintenance helpers for stats, orphan sweeping, and safe deletion under the cache root.

PR diffs use `merge-base(base, head)..head`; shallow clones deepen once, then fall back to the base tip with a log warning if no merge base is found. Sync pins observed heads in the repository it used as `refs/bistro/<safeID>/pull/<N>/heads/<sha>` (keyed by head SHA, not patchset number); cache clones refresh in place so pins survive force-pushes. `PatchID` is SHA-256 of the file patch with `index` lines removed and `@@` hunk headers normalized, preserving hunk boundaries and changed/context lines; viewed marks use the same fingerprint. Triage compares stored patch IDs; per-file interdiffs compare the two heads with go-git. `REQUEST_RANGE_DIFF` and the `git range-diff` / `os/exec` comparison path have been removed.

### `internal/expertise` -- Local Expertise Scoring
Computes per-file expertise from user-owned local checkouts. The package wraps bounded system `git` calls for blame/log/check-attr, applies built-in and user exclude rules, scores surviving lines/recent substantial commits/original authorship, writes raw components to SQLite, performs incremental updates for PR-touched files, powers the `touches_you` resolver, and derives the opt-in enable nudge.

### `internal/scan` -- Developer Folder Scanner
Validates developer-folder inputs and scans immediate child repositories. It rejects unsafe/system folders, skips symlinks, extracts parseable Git remotes, and returns stable repo identities for matching local clones to provider repositories.

### `internal/keychain` -- Token Storage
Wraps `go-keyring` for OS-level credential storage. `TokenManager` provides a per-provider high-level API.

### `internal/cache` -- Username Cache
JSON file-backed cache (`~/.cache/bistro/providers.json`) storing provider usernames with 30-day expiry.

---

## Issues and Refactoring Suggestions

### 1. `server/app.go` Is a God File -- Split by Responsibility

`app.go` currently owns auth handlers, the sync loop, provider/username resolution, and the application lifecycle. These should be separated.

**Proposed structure:**

```
internal/server/
├── app.go                    # App struct, constructors (New/NewWithLogger),
│                             # Run(), SetProvider, SetGitClient, getProvider,
│                             # handler registration wiring, signal handling
│
├── handler/                  # New subpackage for all WebSocket handlers
│   ├── auth.go               # handleAuthStart, handleAuthCheck, handleAuthDisconnect,
│   │                         # runAuthCodeFlow, fetchUsername
│   ├── diff.go               # newRequestDiffHandler (moved from diff_handler.go)
│   ├── pr.go                 # newRequestPRListHandler (moved from pr_list_handler.go)
│   └── repo.go               # newRequestRepoListHandler, newToggleWatchHandler
│                             #   (moved from repo_handler.go)
│
└── syncloop.go               # runSyncLoop, syncWatchedRepos, syncSingleRepo
                              #   (extracted from app.go)
```

This gives you the grouping you described:
- **Auth** -- all 3 auth handlers + the OAuth flow goroutine + username fetch
- **Diff** -- diff request handler
- **PR** -- PR list handler
- **Repo** -- repo list + watch toggle handlers

The sync loop methods (`runSyncLoop`, `syncWatchedRepos`, `syncSingleRepo`) are not WebSocket handlers but background goroutine logic, so they belong in their own file alongside `app.go`.

### 2. `provider/github/` -- Consolidate Small Files

The GitHub client is currently split across 7 source files. Several are very small and the split creates unnecessary navigation overhead:

| Current File | Lines | Responsibility |
|---|---|---|
| `client.go` | 48 | constructor + GetUsername |
| `provider_impl.go` | 134 | interface routing + parsePRID + Authenticate stub |
| `prs.go` | 73 | listPRs, getPR, normalizePR |
| `comments.go` | 48 | getComments, normalizeComment |
| `ci.go` | 87 | getCIStatus, normalizeCheckRun, aggregateStatus |
| `actions.go` | 73 | approve, requestChanges, postComment |
| `list_repos.go` | 43 | ListUserRepos |

**Suggested consolidation:**

```
internal/provider/github/
├── client.go           # Client struct, NewClient, Authenticate, GetUsername
│                       #   (merge current client.go + Authenticate from provider_impl.go)
│
├── pull_requests.go    # listPRs, getPR, normalizePR, ListPRs, GetPR
│                       #   (merge prs.go + PR-related methods from provider_impl.go)
│
├── comments.go         # getComments, normalizeComment, GetComments
│                       #   (unchanged, but absorb GetComments from provider_impl.go)
│
├── ci.go               # getCIStatus, normalizeCheckRun, aggregateStatus, GetCIStatus
│                       #   (unchanged, but absorb GetCIStatus from provider_impl.go)
│
├── reviews.go          # approve, requestChanges, postComment, Approve, RequestChanges, PostComment
│                       #   (rename actions.go; absorb interface methods from provider_impl.go)
│
├── repos.go            # ListUserRepos
│                       #   (rename list_repos.go for consistency)
│
└── prid.go             # parsePRID (shared utility, currently buried in provider_impl.go)
```

The key change: **eliminate `provider_impl.go`** by moving each interface method into the file that owns its implementation. `parsePRID` is a shared utility used by 6 methods, so it gets its own tiny file. This way, when you look at `pull_requests.go` you see both the low-level GitHub API calls and the `Provider` interface methods together.

### 3. `sync/events.go` -- Remove the Alias Layer

`sync/events.go` exists solely to re-export types from `internal/events`:

```go
type SyncEvent = events.Event
type EventEmitter = events.EventEmitter
func NewEventEmitter() *EventEmitter { return events.NewEventEmitter() }
// ...
```

All callers within `sync/` should import `internal/events` directly. Delete `sync/events.go` and update the 3-4 call sites.

### 4. `sync/` Package -- Too Many Concerns

The `sync/` package mixes four distinct responsibilities:

| Concern | Files |
|---|---|
| Orchestration logic | `orchestrator.go` |
| Concurrent fetching | `concurrent.go` |
| Database writes | `writer.go` |
| Offline action queue | `queue.go` |

The orchestrator, worker pool, and writer are tightly coupled and fine together. But `ActionQueue` is a persistence concern (SQL queries, retry counts, status management) that belongs in `store/`:

```
internal/store/
├── db.go
├── diff_store.go
├── action_queue.go    # moved from sync/queue.go
├── migration_runner.go
└── migrations/
```

### 5. `sync/concurrent.go` -- Mixed Concerns

`concurrent.go` contains both a general-purpose `WorkerPool` (reusable) and `PRFetchResult` + `FetchPRDetails` + `SyncPRsWithDetails` (sync-specific business logic). Split into:

- `workerpool.go` -- the generic `WorkerPool` (Submit/Wait/safeRun)
- `fetcher.go` -- `PRFetchResult`, `FetchPRDetails`, `SyncPRsWithDetails`

### 6. `keychain/` -- Two Files for One Thing

`keychain.go` has three free functions (`StoreToken`, `GetToken`, `DeleteToken`) and `token.go` has `TokenManager` which wraps them. These are 70 lines total. Merge into a single `keychain.go`.

### 7. Naming Inconsistencies

- `actions.go` in the `github/` package does approve/request-changes/post-comment (reviews + comments). Rename to `reviews.go` to match the domain concept and avoid confusion with `internal/actions/`.
- `list_repos.go` should be `repos.go` for consistency with `prs.go`, `comments.go`, `ci.go`.
- `provider_impl.go` is a meaningless name. Its contents should be distributed to the files that own each method.

---

## Summary: Proposed Final Layout

```
go/
├── cmd/bistro-sync/
│   └── main.go
│
├── internal/
│   ├── actions/
│   │   ├── types.go
│   │   ├── handlers.go
│   │   ├── executor.go
│   │   └── retry.go
│   │
│   ├── auth/
│   │   ├── oauth.go
│   │   └── callback.go
│   │
│   ├── cache/
│   │   └── providers.go
│   │
│   ├── events/
│   │   └── events.go
│   │
│   ├── git/
│   │   ├── types.go
│   │   ├── clone.go
│   │   ├── diff.go
│   │   ├── cache.go
│   │   └── paths.go
│   │
│   ├── keychain/
│   │   └── keychain.go              # merged token.go + keychain.go
│   │
│   ├── provider/
│   │   ├── interface.go
│   │   ├── models.go
│   │   ├── errors.go
│   │   ├── rate_limit.go
│   │   ├── github/
│   │   │   ├── client.go            # constructor + auth + GetUsername
│   │   │   ├── pull_requests.go     # listPRs, getPR, normalizePR, ListPRs, GetPR
│   │   │   ├── comments.go          # getComments, normalizeComment, GetComments
│   │   │   ├── ci.go                # getCIStatus, normalizeCheckRun, GetCIStatus
│   │   │   ├── reviews.go           # approve, requestChanges, postComment, Approve, etc.
│   │   │   ├── repos.go             # ListUserRepos
│   │   │   └── prid.go              # parsePRID
│   │   └── mock/
│   │       └── provider_mock.go
│   │
│   ├── server/
│   │   ├── app.go                   # App struct, constructors, Run(), wiring only
│   │   ├── syncloop.go              # runSyncLoop, syncWatchedRepos, syncSingleRepo
│   │   └── handler/
│   │       ├── auth.go              # AUTH_START, AUTH_CHECK, AUTH_DISCONNECT
│   │       ├── diff.go              # REQUEST_DIFF
│   │       ├── pr.go                # REQUEST_PR_LIST
│   │       └── repo.go              # REQUEST_REPO_LIST, TOGGLE_WATCH_REPO
│   │
│   ├── store/
│   │   ├── db.go
│   │   ├── diff_store.go
│   │   ├── action_queue.go          # moved from sync/queue.go
│   │   ├── migration_runner.go
│   │   └── migrations/
│   │
│   ├── sync/
│   │   ├── types.go
│   │   ├── orchestrator.go
│   │   ├── workerpool.go            # generic WorkerPool (split from concurrent.go)
│   │   ├── fetcher.go               # PRFetchResult, FetchPRDetails (split from concurrent.go)
│   │   └── writer.go
│   │
│   └── ws/
│       ├── server.go
│       └── dispatch.go
│
├── go.mod
└── go.sum
```

### Changes at a Glance

| Change | Why |
|---|---|
| Extract `server/handler/` subpackage with auth, diff, pr, repo files | Handlers grouped by domain instead of scattered across `app.go` + loose files |
| Extract `server/syncloop.go` from `app.go` | Separate background sync logic from app wiring |
| Eliminate `provider/github/provider_impl.go` | Move each interface method next to its implementation |
| Rename `github/actions.go` to `github/reviews.go` | Avoid confusion with `internal/actions/`; matches domain language |
| Rename `github/list_repos.go` to `github/repos.go` | Consistent naming |
| Extract `github/prid.go` | Shared utility deserves its own file |
| Move `sync/queue.go` to `store/action_queue.go` | Queue is a persistence concern, not sync logic |
| Split `sync/concurrent.go` into `workerpool.go` + `fetcher.go` | Separate generic concurrency primitive from business logic |
| Delete `sync/events.go` | Remove pointless alias layer; import `events` directly |
| Merge `keychain/keychain.go` + `keychain/token.go` | 70 lines total, no reason for two files |
