# Bistro Go backend: project overview

Bistro's root Go module is `github.com/itterno/bistro` (Go 1.26.1). One Wails
v3.0.0 process hosts the native shell and in-process engine. Go authenticates,
syncs PR metadata/comments/CI, generates local diffs, and owns all SQLite writes.
The renderer uses generated typed bindings and registered Wails events, not a
network transport. SQLite is a cache, not a shared server.

For build and verification commands, see [CLAUDE.md](../CLAUDE.md#how-to-work);
for setup and configuration, see [DEVELOPMENT.md](../DEVELOPMENT.md).

## Current directory structure

```text
main.go, window.go          Wails entrypoint, lifecycle, native window/menu
cmd/bistro-benchseed/       Benchmark cache seeding
internal/
  engine/                  Store preparation, service wiring, supervised loops
  services/                Typed domain methods, errors, events, test recorder
  shell/                   Settings, editor/platform, telemetry, updater
  actions/                 Provider write actions and retry classification
  auth/                    OAuth URL, callback listener/page, token exchange
  cache/                   Filesystem-backed provider username cache
  classify/                Rules, bot classification, deduplication
    builtin/               Embedded rule data
    llm/                   Optional BYOK clients and async classification tier
  events/                  Internal pub/sub emitter
  expertise/               Local Git-history indexing, scoring, persistence
  git/                     Clone/fetch/checkout and merge-base operations
    anchor/                Comment anchor resolution
    clonecache/            Clone paths, staging, maintenance, PR-head retention
    diff/                  Diffs, interdiffs, patch fingerprints
    remote/                Canonical repository identities
    repolock/              Shared per-repository locks
    gittest/               On-disk test commit fixtures
  keychain/                OS credential storage
  logging/                 slog setup/context and SHA helpers
  provider/                Provider interface and domain models
    github/                REST/GraphQL implementation
    mock/                  Test provider
  prsync/                  Sync orchestration and action replay
    workerpool/            Bounded concurrent fetches
    writer/                Fetched results and SQLite batch persistence
  scan/                    Developer-folder repository discovery
  store/                   DB handle, DB-bound queries, embedded migrations
    actionqueue/           Offline action persistence
    currentuser/           Authenticated login per provider
    dismissal/             Local comment dismissals
    prdiff/                Cached diff rows
    repolocation/          Local/cache paths and recovery state
    reviewdraft/           Current-user line/range drafts
    storetest/             Migrated in-memory test DBs
    db/                    Generated sqlc code
    migrations/            Embedded versioned SQL
    queries/               sqlc sources
frontend/bindings/          Committed generated Wails TypeScript output
build/                     Platform Taskfiles and packaging resources
sqlc.yaml, go.mod, go.sum   Root generation/module configuration
```

## Lifecycle and data flow

`main.go` configures logging/settings, creates the Wails app, and sets the
engine emitter and panic reporter before `Start(ctx)`. Start opens/migrates
SQLite, prepares stores, restores keychain credentials, builds `services.Set`,
and launches supervised background work. Wails enforces single-instance
behavior. `OnShutdown` cancels the shared engine/shell context and stops update
checks; deferred `engine.Stop()` waits for work and closes SQLite after the
Wails run loop returns. `--smoke` uses isolated
temporary state, verifies the native window/runtime, and exits without sync.

```text
React services → call/callWrite → generated Wails bindings
  → internal/services → provider / stores / git / shell
  → manual sync → prsync.Orchestrator ← engine periodic sync
      → workerpool → provider fetches + local Git operations
      → writer → SQLite cache
      → actions + store/actionqueue → write actions / replay

internal/events → engine bridge → registered typed Wails events
  → frontend/src/services/events/index.ts → query/store updates
```

## Package responsibilities

### `internal/engine` — application core

`New(Options)`, `Start(ctx)`, and `Stop()` separate construction from lifecycle.
`app.go` prepares stores; `options.go` owns defaults; `services.go` constructs
services; `supervise.go` restarts panicking loops with bounded backoff.
`provider.go` owns provider-generation lifetimes. `config.go`, `llm.go`,
`expertise.go`, `repo_location.go`, `bridge.go`, and `syncloop.go` wire domains
and typed notifications. `ApplySyncSettings` clamps intervals to 30–3600 seconds.

### `internal/services` — typed API

Each domain exposes context-aware concrete request/response methods.
`guard`/`guardErr` recover/report panics and normalize errors to `ServiceError`.
Codes are `not_found`, `unauthenticated`, `rate_limited`, `invalid`, `cancelled`,
`timeout`, and `internal`; optional reasons identify domain failures.
IDs are normalized at the service boundary. `events.go` registers typed payloads;
`servicestest.Recorder` captures emissions in behavior tests.

`Auth.Start` blocks until OAuth completion/cancellation. `ReviewQueue.Get`,
`DevFolderScan.Scan`, and `Repos.List` support onboarding; a new scan cancels
its predecessor and `cache_only` allows provider-free repo reads.
`Threads.SubmitAction` dispatches review actions; draft submission never queues.

### `internal/shell` — native integration

`SettingsStore` validates and atomically persists JSON settings, polls external
edits, and owns shell-only window bounds. Editor/platform helpers enforce
contained paths and supported external URL schemes. Sentry and PostHog honor
the privacy setting. The built-in Wails updater checks GitHub pre-releases on
launch and every six hours with `SHA256SUMS`; dev builds do not update.

### `internal/provider` and `provider/github` — remote API

The provider interface defines PR, review, comment, CI, repo, rate-limit, and
review-state models. GitHub files group REST implementations by domain;
`graphql.go` and `threads.go` handle review threads, `inbox_discovery.go` and
`review_queue.go` discover candidates, and `logging_transport.go` instruments
requests. `provider/mock` supports behavior tests.

### `internal/prsync` — sync orchestration

`Orchestrator` coordinates list/fetch/write phases, single-PR sync, actions,
offline replay, inbox discovery, and anchors. `request.go` and `fetch.go` own
request/result and fetch logic; `options.go` owns builders. `review.go` absorbs
current-user drafts into approval/request-changes. `submit_review` bypasses
replay and removes drafts only after GitHub accepts a commit group.

`workerpool` bounds fetch concurrency; panic errors return from Wait and invoke
an optional reporter. `writer` persists metadata, comments, threads, CI, diffs,
patchsets, and post-commit classification events. Neither imports the parent.

Observed patchsets change when the head or known merge base changes, not merely
the base tip. Per-file patch fingerprints distinguish recorded snapshots from
pending/pre-upgrade rows. Since-last-review baselines track the latest submitted
review or qualifying viewed-file action.

### `internal/actions`, `auth`, and `events`

`actions` executes provider writes and classifies retryable failures. `auth`
builds URLs, runs the temporary loopback callback listener, and exchanges codes
with GitHub or Bistro's OAuth relay. `events.EventEmitter` is internal pub/sub;
the engine bridges those events into the typed renderer surface.

### `internal/store` — SQLite cache

`DB` uses mattn/go-sqlite3 with foreign-key/WAL pragmas. The root owns DB-bound
patchsets, viewed files, review baselines, sync cursors, checkout metadata, and
review threads. `migrate.go` applies embedded versioned migrations. SQL sources
live in `queries/`; generated output lives in `db/` and is never hand-edited.

Independent stores cover action queues, current user, dismissals, cached diffs,
repo locations, and review drafts. Subpackages may import `store`/`store/db`,
never the reverse. Expertise persistence stays in `internal/expertise`.
Tests outside `store` use `storetest.Open(t)`; migration tests can use
`store.MigrationFS()` with goose.

### `internal/git` — local operations

`Client` owns go-git clone/fetch/checkout/merge-base work. Focused subpackages
own anchors, clone cache maintenance, canonical remotes, shared locks, diff
algorithms, and test commit fixtures. `clonecache` depends on `remote`, not its
parent. `repolock` depends on `clonecache` for the non-empty `BISTRO_REPO_DIR`
override check. Interdiff shares `*repolock.Locks` with clone/fetch.

PR diffs use `merge-base(base, head)..head`. Observed heads are pinned under
`refs/bistro/<safeID>/pull/<N>/heads/<sha>` across force-pushes. `diff.PatchID`
computes SHA-256 fingerprints for rebase-stable viewed marks. Expertise's
bounded system-git calls are a separate exception to go-git-based operations.

### Remaining packages

- `classify`: embedded YAML rules plus user overrides, severity/category, and
  duplicate grouping. `classify/llm` provides optional BYOK classification.
- `expertise`: bounded local blame/log/check-attr indexing, exclusions, scoring,
  incremental PR-touched-file updates, `touches_you`, and opt-in nudges.
- `scan`: validates developer folders, scans one nested level, skips symlinks
  and unsafe/system folders, and matches canonical repo identities.
- `keychain`: go-keyring token manager. The historical credential service name
  is retained for compatibility; it does not imply a separate process.
- `cache`: JSON provider username cache with 30-day expiry.
- `logging`: private file/stderr slog setup, scoped context, and `ShortSHA`.

Historical package-layout discussion remains in
[go-refactoring-notes.md](go-refactoring-notes.md).
