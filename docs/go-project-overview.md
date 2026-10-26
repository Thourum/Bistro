# Bistro Go Backend: Project Overview

## What Is This?

Bistro is a local-first desktop GitHub PR review app. The Go engine (`go/`, executable `bistro-sync`) authenticates via OAuth, syncs PR metadata/comments/CI/diffs into local SQLite, serves the Electron UI over a localhost WebSocket, and queues offline write actions for retry. Go owns all SQLite writes; the database is a cache, not a shared server.

## Current Directory Structure

The module is `github.com/itterno/bistro` (`go/go.mod`). This map reflects the source tree as of 2026-10-26; test files and generated SQL files are omitted.

```text
go/
├── cmd/bistro-sync/main.go        # CLI flags, logging, engine bootstrap
├── internal/
│   ├── actions/                  # PR write actions and retry
│   │   ├── executor.go
│   │   ├── handlers.go
│   │   ├── retry.go
│   │   └── types.go
│   ├── auth/                     # OAuth callback and token exchange
│   │   ├── oauth.go
│   │   ├── callback.go
│   │   ├── callback_page.go
│   │   └── callback.html
│   ├── cache/providers.go        # Filesystem-backed provider username cache
│   ├── classify/                 # Rules, bot classification, deduplication
│   │   ├── engine.go · provider.go · rules.go · loader.go · seed.go · dedup.go
│   │   ├── builtin/              # Embedded rule data
│   │   └── llm/client.go · tier.go
│   ├── events/events.go          # Pub/sub event emitter
│   ├── expertise/                # Local-only git-history expertise indexing
│   │   └── engine.go · gitcli.go · indexer.go · updater.go · score.go
│   │       identity.go · excludes.go · nudge.go · store.go
│   ├── git/                      # Client: clone, fetch, checkout, merge base
│   │   ├── client.go
│   │   ├── clone.go · broad_clone.go · targeted_clone.go · refresh.go
│   │   ├── checkout.go · localfetch.go · mergebase.go
│   │   ├── anchor/               # Comment-anchor resolution
│   │   │   └── resolver.go · position.go · hash.go
│   │   ├── clonecache/           # Cache entries, staging, maintenance, head pins
│   │   │   └── cache.go · paths.go · staging.go · maintenance.go · retention.go
│   │   ├── diff/                 # Local diffs, interdiffs, patch fingerprints
│   │   │   └── diff.go · model.go · interdiff.go · patchid.go
│   │   ├── remote/identity.go    # Canonical repo identity from remote URLs
│   │   ├── repolock/locks.go     # Shared per-repo operation locks
│   │   └── gittest/commit.go     # On-disk commit fixtures for tests
│   ├── keychain/keychain.go      # OS credential storage and TokenManager
│   ├── logging/context.go · sha.go
│   ├── provider/                 # Provider interface and domain models
│   │   ├── interface.go · models.go · errors.go · rate_limit.go · review_status.go
│   │   ├── github/
│   │   │   └── client.go · pull_requests.go · comments.go · ci.go · reviews.go
│   │   │       repos.go · prid.go · graphql.go · threads.go · inbox_discovery.go
│   │   │       review_queue.go · logging_transport.go
│   │   └── mock/provider_mock.go
│   ├── prsync/                   # PR sync orchestration (formerly internal/sync)
│   │   ├── orchestrator.go · request.go · fetch.go · inbox.go
│   │   ├── anchors.go · actions.go · replay.go
│   │   ├── workerpool/pool.go    # Bounded concurrent fetch tasks
│   │   └── writer/               # SQLite batch persistence
│   │       └── writer.go · fetched_pr.go · rows.go · patchset.go · threads.go · diffs.go
│   ├── scan/scanner.go           # Developer-folder repository discovery
│   ├── server/                   # Application wiring and lifecycle
│   │   ├── app.go                # App, constructors, stores, Run
│   │   ├── provider.go · config.go · llm.go · expertise.go · bridge.go
│   │   ├── repo_location.go · handlers.go
│   │   ├── ready.go · wsaddr.go · syncloop.go
│   │   ├── repo_location_resolver.go · interdiff_repo.go
│   │   └── handler/              # Domain-specific WebSocket handlers
│   │       └── auth.go · classification_settings.go · comments.go · dev_folder_scan.go
│   │           diff.go · dismiss.go · editor.go · expertise.go · interdiff.go
│   │           patchsets.go · pr.go · readstate.go · repo.go · review_queue.go
│   │           repolocation.go · repolocation_add.go · repolocation_cache.go
│   │           repolocation_clone.go · repolocation_remove.go · sync.go
│   │           threads.go · triage.go · viewed_baseline.go
│   ├── store/                    # DB handle, migrations, DB query methods
│   │   ├── db.go · migrate.go · patchsets.go · files_viewed.go · last_reviewed.go
│   │   ├── sync_cursor.go · pr_checkout.go · review_threads.go
│   │   ├── actionqueue/queue.go
│   │   ├── currentuser/currentuser.go
│   │   ├── dismissal/dismissal.go
│   │   ├── prdiff/prdiff.go
│   │   ├── repolocation/repolocation.go
│   │   ├── storetest/storetest.go # Migrated in-memory test databases
│   │   ├── db/                   # Generated sqlc code; never edit by hand
│   │   ├── migrations/           # Embedded versioned SQL migrations
│   │   └── queries/              # sqlc query sources
│   └── ws/server.go · dispatch.go
├── sqlc.yaml
├── go.mod
└── go.sum
```

## Data Flow

```text
Electron UI
  └─ localhost WebSocket → ws.Server → ws.Dispatcher
       ├─ server/handler → provider / stores / git operations
       └─ manual sync → prsync.Orchestrator ← server/syncloop (periodic sync)
                          ├─ provider → remote PR details
                          ├─ workerpool → concurrent fetches
                          ├─ git.Client → clone/fetch/merge base
                          ├─ git/diff + git/anchor → diffs/fingerprints/anchors
                          ├─ writer → SQLite cache
                          └─ actions + store/actionqueue → write actions/replay

events.EventEmitter → server/bridge.go → WebSocket push → Electron UI
```

## Package-by-Package Summary

### `cmd/bistro-sync` -- Entrypoint

Parses CLI flags, configures structured logging, bootstraps credentials, and calls `server.App.Run()`.

### `internal/server` -- Application Core

`app.go` owns the App struct, constructors, store preparation, and Run lifecycle. The split files remain in package `server`: `provider.go` owns provider/orchestrator lifecycle; `config.go` owns configuration setters; `llm.go` wires BYOK classification; `expertise.go` wires indexing and handlers; `bridge.go` forwards events to WebSocket clients; `repo_location.go` wires clone/location handlers; `handlers.go` registers auth, review, diff, and checkout handlers.

`server/handler` contains WebSocket handlers grouped by domain, including auth, comments/threads, diffs, patchsets, triage/interdiff, PR/repo lists, read state, classification, expertise, editor actions, developer-folder scanning, and review-queue discovery. Repo-location handlers cover add/remove/recover and cache maintenance. `syncloop.go` schedules background sync; `ready.go` emits readiness payloads; `wsaddr.go` formats the listener address; `repo_location_resolver.go` and `interdiff_repo.go` resolve usable clones.

The engine acquires a lock on `<db>.lock`, migrates SQLite before announcing `{"event":"ready","wsPort":N}`, and supports `-exit-on-stdin-close` so Electron owns its lifetime. Normal app startup and verification commands live in [the agent guide](../AGENTS.md#how-to-work).

### `internal/ws` -- WebSocket Transport

`Server` owns listener/connections/broadcast and enforces local origins. `Dispatcher` routes JSON messages to registered `MessageHandler` functions; business logic stays in handlers/services.

### `internal/provider` -- Provider Abstraction

Defines `Provider`, shared domain models (`PR`, `Comment`, `Repo`, `CIStatus`), provider errors, rate-limit handling, review-state normalization, and the `provider.GitHub` constant. `provider/github` implements the interface; `provider/mock` supplies test providers.

### `internal/provider/github` -- GitHub API Client

`client.go` initializes the API clients. `pull_requests.go`, `comments.go`, `ci.go`, `reviews.go`, and `repos.go` group interface methods with their implementations; `prid.go` handles PR identities. `graphql.go` and `threads.go` cover review-thread operations; `inbox_discovery.go` and `review_queue.go` discover inbox/review candidates; `logging_transport.go` instruments API requests.

### `internal/prsync` -- Sync Engine

Renamed from `internal/sync` to avoid shadowing the standard library. `Orchestrator` coordinates list/fetch/write phases and single-PR sync, action dispatch, offline replay, inbox discovery, and anchor updates. Request/result types live in `request.go`; fetch logic lives in `fetch.go`.

- `prsync/workerpool`: `Pool`, `New`, and `DefaultWorkers` bound concurrent fetch tasks.
- `prsync/writer`: `FetchedPR` is the writer input; `Writer`/`New` persist fetched metadata, comments, threads, CI, diffs, and patchsets, with classification and post-commit events.
- `store/actionqueue` owns queue persistence; `internal/events` owns events directly (no sync alias layer).

A new observed patchset is recorded when the head or known merge base changes, not when only the base tip moves. Patchsets store `merge_base_sha` and per-file `patch_id`s in `patchset_files`; `has_fingerprints` distinguishes recorded fingerprints from pending/pre-upgrade snapshots. The since-last-review baseline is the newest user's submitted GitHub review (from any client), Bistro approve/request-changes/top-level comment, or mark-viewed action leaving no file unviewed.

### `internal/actions` -- PR Action Execution

Executes approve/request-changes/comment and thread actions through handlers, emitting completion/failure events and supporting retry with exponential backoff.

### `internal/auth` -- OAuth Flow

`BuildAuthURL` constructs authorization URLs; `CallbackServer` accepts a temporary localhost redirect, exchanges the code, and validates scopes. With a configured client secret (development), `ExchangeToken` posts to GitHub directly. Without one (packaged builds), it posts `{code, redirect_uri}` to `https://get-bistro.com/api/oauth/token`, which holds the secret. `-auth-callback-timeout` controls the callback wait.

### `internal/events` -- Event System

Buffered pub/sub `EventEmitter`, shared by sync and action execution. The server event bridge translates events into WebSocket pushes.

### `internal/store` -- Database Layer

`DB` wraps SQLite (`mattn/go-sqlite3`) with foreign-key/WAL pragmas. The root retains DB-bound methods for patchsets, viewed files, review baselines, sync cursors, checkout metadata, and review threads. `migrate.go` applies embedded versioned SQL migrations. Query sources stay in `queries/`; generated sqlc code stays in `db/`.

Independent persistence surfaces live in subpackages:

| Package | API / responsibility |
|---|---|
| `store/actionqueue` | `Queue`, `New`, `Action`, `MaxRetryCount`: offline action persistence, replay state and retries |
| `store/currentuser` | `Store`, `New`: authenticated login per provider |
| `store/dismissal` | `Store`, `New`: per-account local comment dismissals |
| `store/prdiff` | `File`, `Store`, `New`: cached PR diff rows and unified diff |
| `store/repolocation` | `Location`, `Store`, `New`: local/cache clone paths and recovery state |
| `store/storetest` | `Open(t) *store.DB`: migrated in-memory databases for tests outside `store` |

Store subpackages may import the root or `store/db`, never the reverse. Expertise persistence remains in `internal/expertise`, not repo-location storage.

### `internal/git` -- Git Operations

The root `Client` owns clone/fetch/checkout and merge-base operations using go-git. Broad/targeted cloning, cached-clone refresh, and local-checkout PR-ref fetching remain methods in the root package. Stateless operations and shared state have focused homes:

| Package | API / responsibility |
|---|---|
| `git/anchor` | `Resolver`, `NewResolver`, `Request`, `Result`: comment anchors, positions and context hashes |
| `git/clonecache` | `Cache`, `New`, `RepoEntry`: cache entries; paths/staging, stats/deletion/orphan sweep, observed-head pins |
| `git/diff` | `Generate`, `SingleFilePatch`, `Interdiff`, `PatchID`: diffs, per-file interdiffs, rebase-stable fingerprints |
| `git/remote` | `Identity`, `LocationError`, `ParseURL`, `SafeRepoID`: canonical identities from remote URLs |
| `git/repolock` | `Locks`, `New`, `Key`: shared per-repo and maintenance locks |
| `git/gittest` | On-disk commit fixtures using go-git and the standard library |

`git` imports `clonecache`, `remote`, and `repolock`; `clonecache` imports `remote`; `diff` imports `repolock`; `anchor` imports `diff`; `repolock` imports `remote`. None imports the parent `git` package. `diff.Interdiff` uses the same `*repolock.Locks` as clone/fetch, without depending on `git.Client`.

PR diffs use `merge-base(base, head)..head`. Observed heads are pinned under `refs/bistro/<safeID>/pull/<N>/heads/<sha>` across force-pushes. `diff.PatchID` computes a SHA-256 changed/context fingerprint; triage compares stored patch IDs and per-file interdiffs compare the two heads with go-git. The old `REQUEST_RANGE_DIFF`/system `git range-diff` comparison path is removed. Expertise's bounded system-git exception is separate.

### `internal/classify` -- Comment Classification

Loads/seeds embedded YAML rules and user overrides, classifies bot comments by severity/category, and groups duplicates. `classify/llm` supplies optional BYOK classification and task-tier logic; server wiring selects the live client/settings.

### `internal/expertise` -- Local Expertise Scoring

Indexes user-owned local checkouts using bounded system-git blame/log/check-attr calls. Applies exclusion rules, scores surviving lines/recent substantial commits/original authorship, persists components, updates PR-touched files incrementally, resolves `touches_you`, and derives the opt-in enable nudge.

### `internal/scan` -- Developer Folder Scanner

Validates developer-folder input, scans immediate child repositories, rejects unsafe/system folders, skips symlinks, and returns canonical remote identities for matching local clones to provider repos.

### `internal/keychain` -- Token Storage

`keychain.go` wraps go-keyring and provides the per-provider `TokenManager`; low-level and high-level token operations now share one file.

### `internal/cache` -- Username Cache

JSON-backed provider username cache (`~/.cache/bistro/providers.json`) with 30-day expiry.

### `internal/logging` -- Structured Logging Helpers

`context.go` propagates scoped slog loggers; `sha.go` provides the shared `ShortSHA` formatter used by git and sync logging.

## Refactoring History

Earlier proposals are preserved in [Go refactoring notes](go-refactoring-notes.md). They describe the old layout, not current implementation requirements; this overview is the current package map.
