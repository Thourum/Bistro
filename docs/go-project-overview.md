# Bistro Go Backend: Project Overview

## What Is This?

Bistro is a local-first desktop GitHub PR review app. The in-process Go engine (repo-root module) authenticates via OAuth, syncs PR metadata/comments/CI/diffs into local SQLite, exposes typed services and registered Wails events, and queues replayable write actions offline for retry. Review submissions with drafts bypass the queue and keep unsubmitted drafts on failure. Go owns all SQLite writes; the database is a cache, not a shared server.

## Current Directory Structure

The module is `github.com/itterno/bistro` (`go.mod`). This map reflects the source tree as of 2026-10-29; test files and generated SQL files are omitted.

```text
./
├── cmd/bistro-benchseed/          # Benchmark cache seeding
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
│   │   ├── options.go · review.go # With* builders; draft-absorbing review submission
│   │   ├── workerpool/pool.go    # Bounded concurrent fetch tasks
│   │   └── writer/               # SQLite batch persistence
│   │       └── writer.go · fetched_pr.go · rows.go · patchset.go · threads.go · diffs.go
│   ├── scan/scanner.go           # Developer-folder repository discovery
│   ├── engine/                   # Application wiring and lifecycle
│   │   ├── app.go · options.go    # App, stores, Start/Stop, defaults
│   │   ├── provider.go · config.go · llm.go · expertise.go · bridge.go
│   │   ├── repo_location.go · services.go · supervise.go · syncloop.go
│   │   └── repo_location_resolver.go · interdiff_repo.go
│   ├── services/                 # Typed service methods and Wails events
│   │   └── servicestest/recorder.go
│   ├── store/                    # DB handle, migrations, DB query methods
│   │   ├── db.go · migrate.go · patchsets.go · files_viewed.go · last_reviewed.go
│   │   ├── sync_cursor.go · pr_checkout.go · review_threads.go
│   │   ├── actionqueue/queue.go
│   │   ├── currentuser/currentuser.go
│   │   ├── dismissal/dismissal.go
│   │   ├── prdiff/prdiff.go
│   │   ├── repolocation/repolocation.go
│   │   ├── reviewdraft/store.go
│   │   ├── storetest/storetest.go # Migrated in-memory test databases
│   │   ├── db/                   # Generated sqlc code; never edit by hand
│   │   ├── migrations/           # Embedded versioned SQL migrations
│   │   └── queries/              # sqlc query sources
├── sqlc.yaml
├── go.mod
└── go.sum
```

## Data Flow

```text
Wails shell / renderer (entrypoint pending Task 14)
  └─ typed services.Set calls
       ├─ internal/services → provider / stores / git operations
       └─ manual sync → prsync.Orchestrator ← engine/syncloop (periodic sync)
                          ├─ provider → remote PR details
                          ├─ workerpool → concurrent fetches
                          ├─ git.Client → clone/fetch/merge base
                          ├─ git/diff + git/anchor → diffs/fingerprints/anchors
                          ├─ writer → SQLite cache
                          └─ actions + store/actionqueue → write actions/replay

events.EventEmitter → engine/bridge.go → typed services events → Wails renderer
```

## Package-by-Package Summary

### `internal/engine` -- Application Core

`New(Options)` constructs the engine; `Start(ctx)` opens and migrates the cache, prepares stores, restores keychain credentials, builds services, and starts supervised background work. `Stop()` cancels/waits before closing SQLite. `options.go` owns defaults, `supervise.go` restarts panicking loops with bounded backoff, and `provider.go` owns provider-generation lifetimes. `bridge.go`, `expertise.go`, `llm.go`, and `syncloop.go` emit the registered typed payloads. The shell will drive Start/Stop and own single-instance enforcement in Task 14. There is no standalone sync command or WebSocket transport.

### `internal/services` -- Typed Application API

Domain service structs expose context-aware calls, normalize identities, translate failures into `ServiceError`, and recover/report service panics. `services.Set` retains shared instances; `events.go` registers payload types for generated Wails bindings. `servicestest.Recorder` captures events for tests. Normal verification commands live in [the agent guide](../AGENTS.md#how-to-work).

### `internal/provider` -- Provider Abstraction

Defines `Provider`, shared domain models (`PR`, `Comment`, `Repo`, `CIStatus`), provider errors, rate-limit handling, review-state normalization, and the `provider.GitHub` constant. `provider/github` implements the interface; `provider/mock` supplies test providers.

### `internal/provider/github` -- GitHub API Client

`client.go` initializes the API clients. `pull_requests.go`, `comments.go`, `ci.go`, `reviews.go`, and `repos.go` group interface methods with their implementations; `prid.go` handles PR identities. `graphql.go` and `threads.go` cover review-thread operations; `inbox_discovery.go` and `review_queue.go` discover inbox/review candidates; `logging_transport.go` instruments API requests.

### `internal/prsync` -- Sync Engine

Renamed from `internal/sync` to avoid shadowing the standard library. `Orchestrator` coordinates list/fetch/write phases and single-PR sync, action dispatch, offline replay, inbox discovery, and anchor updates. Request/result types live in `request.go`; fetch logic lives in `fetch.go`; `With*` builders live in `options.go`. `review.go` handles draft-absorbing review submission: approve / request changes include the current user's drafts; `submit_review` bypasses offline/rate-limited replay and removes drafts only after GitHub accepts each commit group.

- `prsync/workerpool`: `Pool`, `New`, and `DefaultWorkers` bound concurrent fetch tasks; panic errors return from Wait and invoke an optional reporting callback.
- `prsync/writer`: `FetchedPR` is the writer input; `Writer`/`New` persist fetched metadata, comments, threads, CI, diffs, and patchsets, with classification and post-commit events.
- `store/actionqueue` owns queue persistence; `internal/events` owns events directly (no sync alias layer).

A new observed patchset is recorded when the head or known merge base changes, not when only the base tip moves. Patchsets store `merge_base_sha` and per-file `patch_id`s in `patchset_files`; `has_fingerprints` distinguishes recorded fingerprints from pending/pre-upgrade snapshots. The since-last-review baseline is the newest user's submitted GitHub review (from any client), Bistro approve/request-changes/top-level comment, or mark-viewed action leaving no file unviewed.

### `internal/actions` -- PR Action Execution

Executes approve/request-changes/comment and thread actions through handlers, emitting completion/failure events and supporting retry with exponential backoff.

### `internal/auth` -- OAuth Flow

`BuildAuthURL` constructs authorization URLs; `CallbackServer` accepts a temporary localhost redirect, exchanges the code, and validates scopes. With a configured client secret (development), `ExchangeToken` posts to GitHub directly. Without one (packaged builds), it posts `{code, redirect_uri}` to `https://get-bistro.com/api/oauth/token`, which holds the secret. `engine.Options.AuthCallbackTimeout` controls the callback wait.

### `internal/events` -- Event System

Buffered pub/sub `EventEmitter`, shared by sync and action execution. The engine event bridge translates consumed orchestrator events into registered typed payloads.

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
| `store/reviewdraft` | `Draft`, `Store`, `New`, `ErrNotFound`: current-user line/range review drafts (create, edit body, list, delete) |
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

Loads/seeds embedded YAML rules and user overrides, classifies bot comments by severity/category, and groups duplicates. `classify/llm` supplies optional BYOK classification and task-tier logic; engine wiring selects the live client/settings.

### `internal/expertise` -- Local Expertise Scoring

Indexes user-owned local checkouts using bounded system-git blame/log/check-attr calls. Applies exclusion rules, scores surviving lines/recent substantial commits/original authorship, persists components, updates PR-touched files incrementally, resolves `touches_you`, and derives the opt-in enable nudge.

### `internal/scan` -- Developer Folder Scanner

Validates developer-folder input, scans child directories and one nested level for repositories, rejects unsafe/system folders, skips symlinks, and returns canonical remote identities for matching local clones to provider repos.

### `internal/keychain` -- Token Storage

`keychain.go` wraps go-keyring and provides the per-provider `TokenManager`; low-level and high-level token operations now share one file.

### `internal/cache` -- Username Cache

JSON-backed provider username cache (`~/.cache/bistro/providers.json`) with 30-day expiry.

### `internal/logging` -- Structured Logging Helpers

`context.go` propagates scoped slog loggers; `sha.go` provides the shared `ShortSHA` formatter used by git and sync logging.

## Refactoring History

Earlier proposals are preserved in [Go refactoring notes](go-refactoring-notes.md). They describe the old layout, not current implementation requirements; this overview is the current package map.
