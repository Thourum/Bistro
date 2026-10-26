# Go Refactoring Notes

2026-10-26: Historical proposals preserved verbatim below; see [Go project overview — Package-by-Package Summary](go-project-overview.md#package-by-package-summary) for the current layout.

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
│   │       ├── auth.go              # AUTH_START, AUTH_CANCEL, AUTH_CHECK, AUTH_DISCONNECT
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
