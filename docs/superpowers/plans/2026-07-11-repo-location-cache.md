# Repo Location, Dev Folder Scan, and Cache Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reuse the user's existing dev-folder Git repos for PR diffs (read-only), add repos by URL to dev folder or managed cache, and expose cache size/prune controls in Settings.

**Architecture:** Go owns all SQLite writes and all filesystem operations (scan, fetch, clone, prune). New location columns extend the existing `repos` table (migration 026). A dev-folder scanner registers `source='local'` repos; the git client resolves local repos before falling back to cache clones, fetching only into `refs/bistro/<hash>/*`. Electron owns the `repositories.devFolderPath` setting, sends scan requests over the existing WebSocket, and renders onboarding/settings/recovery UI.

**Tech Stack:** Go 1.26 (go-git, mattn/go-sqlite3), Electron + React 19 + TypeScript (TanStack, neverthrow, Vitest), WebSocket protocol with SCREAMING_SNAKE message types.

**Spec:** `docs/superpowers/specs/2026-07-11-repo-location-cache-design.md` — read it before starting. Key invariants:

- Bistro NEVER checks out, never touches `HEAD`, user branches, tags, remote-tracking refs, index, or worktree of a dev-folder repo. Fetches write only to `refs/bistro/<safe-repo-id>/*` with tag following disabled.
- Broken local repos do NOT silently fall back to cache — they go degraded and the user picks a recovery action.
- Cache actions never touch `source='local'` repos.
- `BISTRO_REPO_DIR` env (dev/testing only) overrides everything: cache root AND dev-folder resolution.
- Message naming follows the existing codebase convention (`SCREAMING_SNAKE`), not the spec's illustrative `repo.scanDevFolder` names.

**Verification commands** (run per task; full sweep before final commit):

```bash
cd go && go vet ./... && go test -race ./...
cd electron && pnpm type-check && pnpm lint && pnpm test
```

---

## File map

| File | Responsibility |
|---|---|
| `go/internal/store/migrations/026_add_repo_location.sql` | Create: location columns on `repos` |
| `go/internal/store/repo_location.go` (+`_test.go`) | Create: all reads/writes of location columns |
| `go/internal/git/identity.go` (+`_test.go`) | Create: remote URL → identity normalization, path-safe validation, safe-repo-id hash |
| `go/internal/git/locks.go` (+`_test.go`) | Create: keyed per-repo mutex + global cache-maintenance lock |
| `go/internal/git/localfetch.go` (+`_test.go`) | Create: read-only fetch into `refs/bistro/*` on a local repo |
| `go/internal/git/paths.go` | Modify: export `CacheRoot()`; owner/repo cache layout helper |
| `go/internal/git/clone.go` | Modify: local-repo resolution before cache clone |
| `go/internal/scan/scanner.go` (+`_test.go`) | Create: dev-folder scan (bounded, canonical, symlink-safe) |
| `go/internal/git/cachemaint.go` (+`_test.go`) | Create: cache size stats, prune, clear-all, orphan sweep |
| `go/internal/server/handler/repolocation.go` (+`_test.go`) | Create: WS handlers (validate/scan dev folder, add by URL, cache stats/prune/clear, recover, remove) |
| `go/internal/server/app.go` | Modify: wire handlers + startup scan |
| `electron/src/types/settings.ts` | Modify: `repositories.devFolderPath` |
| `electron/src/renderer/hooks/useWebSocket.ts` | Modify: new event payload types |
| `electron/src/renderer/pages/OnboardingPage.tsx` | Modify: dev-folder step |
| `electron/src/main/ipc.ts` + `preload.ts` | Modify: `dialog:pick-folder` IPC |
| `electron/src/renderer/components/CommandPalette.tsx` | Modify: "Add repository by URL" command |
| `electron/src/renderer/components/AddRepoDialog.tsx` (+test) | Create: URL input + destination prompt |
| `electron/src/renderer/pages/SettingsPage.tsx` | Modify: Repositories section (dev folder + cache card) |
| `electron/src/renderer/components/CacheUsageCard.tsx` (+test) | Create: progress bar, accordion, prune/clear |
| `electron/src/renderer/components/RepoRecoveryDialog.tsx` (+test) | Create: degraded repo recovery actions |

Structured error machine codes (fixed list, used by Go and TS): `path_missing`, `remote_mismatch`, `permission_denied`, `invalid_url`, `invalid_path`, `destination_required`, `destination_conflict`, `clone_failed`, `auth_failed`, `cancelled`, `delete_failed`, `scan_partial`.

Error envelope (every location-op failure response):

```json
{ "repo_id": "github:acme/api", "operation": "fetch", "code": "path_missing", "message": "folder not found: /Users/x/dev/api" }
```

---

### Task 1: Migration 026 — repo location columns

**Files:**
- Create: `go/internal/store/migrations/026_add_repo_location.sql`
- Test: `go/internal/store/repo_location_test.go` (started here, grown in Task 2)

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"context"
	"testing"
)

// openMigratedDB helper: open in-memory sqlite, run MigrationRunner — copy the
// pattern from existing tests in db_test.go.
func TestMigration026AddsLocationColumns(t *testing.T) {
	db := openMigratedDB(t)
	defer db.Close()

	// Existing row survives and new columns are readable with NULL defaults.
	mustExecT(t, db, `INSERT INTO repos (id, provider, owner, repo) VALUES ('github:acme/api','github','acme','api')`)
	var localPath, source, lastUsed, status, code, msg any
	err := db.QueryRow(`SELECT local_path, source, last_used_at, location_status, location_error_code, location_error FROM repos WHERE id='github:acme/api'`).
		Scan(&localPath, &source, &lastUsed, &status, &code, &msg)
	if err != nil {
		t.Fatalf("select new columns: %v", err)
	}
	if localPath != nil || source != nil {
		t.Fatalf("expected NULL defaults, got %v %v", localPath, source)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/store/ -run TestMigration026 -v`
Expected: FAIL — `no such column: local_path`

- [ ] **Step 3: Write the migration**

```sql
-- Migration 026: Add repo location columns
-- Description: dev-folder/local vs managed-cache location tracking per repo

ALTER TABLE repos ADD COLUMN local_path          TEXT;
ALTER TABLE repos ADD COLUMN source              TEXT;     -- 'local' | 'cache' | NULL
ALTER TABLE repos ADD COLUMN last_used_at        DATETIME;
ALTER TABLE repos ADD COLUMN location_status     TEXT;     -- 'ok'|'missing'|'remote_mismatch'|'permission_denied'|NULL
ALTER TABLE repos ADD COLUMN location_error_code TEXT;
ALTER TABLE repos ADD COLUMN location_error      TEXT;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go && go test ./internal/store/ -run TestMigration026 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/internal/store/migrations/026_add_repo_location.sql go/internal/store/repo_location_test.go
git commit -m "feat(store): add repo location columns (migration 026)"
```

---

### Task 2: RepoLocationStore

**Files:**
- Create: `go/internal/store/repo_location.go`
- Test: `go/internal/store/repo_location_test.go`

All SQL touching the new columns lives here. Methods (all take `context.Context`, return wrapped errors):

```go
package store

// RepoLocation mirrors the location columns of one repos row.
type RepoLocation struct {
	RepoID            string
	Provider, Owner, Repo string
	LocalPath         string // "" when NULL
	Source            string // "local" | "cache" | ""
	LastUsedAt        *time.Time
	LocationStatus    string
	LocationErrorCode string
	LocationError     string
}

type RepoLocationStore struct{ db *sql.DB }

func NewRepoLocationStore(db *sql.DB) *RepoLocationStore

// UpsertLocal registers/updates a local repo. Creates the repos row when
// missing (watched=1). Preserves an existing healthy local_path: if the row
// already has source='local', location_status='ok', and a different path that
// still exists on disk, the existing path wins (spec: duplicate checkouts).
func (s *RepoLocationStore) UpsertLocal(ctx context.Context, provider, owner, repo, canonicalPath string) error

// SetCache marks a repo as cache-cloned at path, last_used_at=now, status ok.
func (s *RepoLocationStore) SetCache(ctx context.Context, repoID, path string) error

// Touch sets last_used_at=now.
func (s *RepoLocationStore) Touch(ctx context.Context, repoID string) error

// MarkDegraded sets location_status + error code/message. Keeps local_path.
func (s *RepoLocationStore) MarkDegraded(ctx context.Context, repoID, status, code, msg string) error

// ClearDegraded resets status to 'ok' and clears error fields.
func (s *RepoLocationStore) ClearDegraded(ctx context.Context, repoID string) error

// ClearCacheLocation nulls local_path/source/location fields (post-prune).
func (s *RepoLocationStore) ClearCacheLocation(ctx context.Context, repoID string) error

// Get returns the row or (nil, nil) when absent.
func (s *RepoLocationStore) Get(ctx context.Context, repoID string) (*RepoLocation, error)

// ListBySource returns all rows with the given source ("local"/"cache").
func (s *RepoLocationStore) ListBySource(ctx context.Context, source string) ([]RepoLocation, error)
```

- [ ] **Step 1: Write failing tests** — one test per method in `repo_location_test.go`, AAA style, in-memory DB via `openMigratedDB`. Cover: upsert creates row; upsert updates existing row's path when unhealthy; upsert preserves existing healthy path that exists on disk (use `t.TempDir()`); Touch sets timestamp; MarkDegraded/ClearDegraded round-trip; ClearCacheLocation nulls fields; ListBySource filters.
- [ ] **Step 2: Run** `cd go && go test ./internal/store/ -run TestRepoLocation -v` — expect FAIL (types undefined).
- [ ] **Step 3: Implement** `repo_location.go` per signatures above. Plain `database/sql`, parameterized queries only.
- [ ] **Step 4: Run** same command — expect PASS. Also `go vet ./internal/store/`.
- [ ] **Step 5: Commit** `git commit -m "feat(store): RepoLocationStore for repo location columns"`

---

### Task 3: Remote identity normalization + safe repo id

**Files:**
- Create: `go/internal/git/identity.go`
- Test: `go/internal/git/identity_test.go`

```go
package git

// Identity is a normalized repo identity.
type Identity struct {
	Provider string // "github"
	Owner    string
	Repo     string
}

func (i Identity) ID() string // "github:acme/api" (lowercased)

// ParseRemoteURL normalizes the accepted GitHub forms (spec):
//   https://github.com/owner/repo[.git]
//   git@github.com:owner/repo[.git]
//   ssh://git@github.com/owner/repo[.git]
// Rejects: non-github hosts, missing/extra path parts, owner/repo that are
// not path-safe identifiers (allowed: [A-Za-z0-9._-]; reject ".", "..",
// separators, percent-escapes).
func ParseRemoteURL(raw string) (Identity, error)

// SafeRepoID returns the first 16 hex chars of sha256 of Identity.ID().
// Used as the refs/bistro/<id>/ namespace segment.
func SafeRepoID(id Identity) string
```

- [ ] **Step 1: Write failing table-driven test** covering every accepted form, `.git` suffix, uppercase → lowercase ID, and each rejection case (gitlab host, 1 or 3 path parts, `..` owner, `a/b` repo, `%2e%2e` repo, empty). Plus `TestSafeRepoID_DeterministicAndHex`.
- [ ] **Step 2: Run** `cd go && go test ./internal/git/ -run 'TestParseRemoteURL|TestSafeRepoID' -v` — FAIL.
- [ ] **Step 3: Implement** using `net/url` + `strings`; SSH scp-form (`git@host:path`) handled by a small prefix rewrite before `url.Parse`. `crypto/sha256` + `encoding/hex` for SafeRepoID.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(git): remote identity normalization and safe repo id"`

---

### Task 4: Per-repo locks + cache-maintenance lock

**Files:**
- Create: `go/internal/git/locks.go`
- Test: `go/internal/git/locks_test.go`

```go
package git

// RepoLocks provides a mutex per repo identity plus one global
// cache-maintenance lock that excludes ALL per-repo operations.
// ponytail: RWMutex-over-map; fine for a desktop app's repo counts.
type RepoLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	maint sync.RWMutex // per-repo ops take RLock; clear-all takes Lock
}

func NewRepoLocks() *RepoLocks
// WithRepo runs fn holding the repo's mutex AND maint.RLock.
func (l *RepoLocks) WithRepo(repoID string, fn func() error) error
// WithMaintenance runs fn holding the exclusive maintenance lock.
func (l *RepoLocks) WithMaintenance(fn func() error) error
```

- [ ] **Step 1: Failing test** — two goroutines on same repoID serialize (guard a counter); different repoIDs run concurrently; `WithMaintenance` blocks until in-flight `WithRepo` finishes (use channels + `sync/atomic`; run with `-race`).
- [ ] **Step 2: Run** `cd go && go test ./internal/git/ -run TestRepoLocks -race -v` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(git): per-repo and cache-maintenance locks"`

---

### Task 5: Read-only local fetch into refs/bistro

**Files:**
- Create: `go/internal/git/localfetch.go`
- Test: `go/internal/git/localfetch_test.go`

```go
package git

// FetchIntoLocal fetches the required commits into a user-owned local repo,
// writing ONLY refs/bistro/<safeID>/. Never checks out; tag following
// disabled. Validates first that some remote of the repo matches identity
// (returns code remote_mismatch otherwise). opts carries the PR hints
// (PR number, base branch, RequiredSHAs) that drive the targeted refspecs —
// same data cloneWithOptions already receives; nil opts = broad fetch
// (still mapped under refs/bistro/).
func (c *Client) FetchIntoLocal(ctx context.Context, localPath string, id Identity, baseSHA, headSHA string, opts *CloneOptions) error
```

Implementation notes for the engineer:

- Open with `gitlib.PlainOpen(localPath)`.
- Remote validation: iterate `repo.Remotes()`, `ParseRemoteURL` each URL, compare `.ID()` — any match passes.
- Skip fetch entirely if all `shas` already resolve locally (`repo.Storer.EncodedObject`... or reuse `firstMissingCommit` from `clone.go:428`).
- Fetch via an anonymous in-memory remote config against the GitHub URL with `RefSpecs: []config.RefSpec{"+refs/*:refs/bistro/<safeID>/tmp/*"}`? NO — fetching refs/* is broad. Instead reuse the targeted approach from `targetedRefSpecs` (`clone.go:376`): fetch `refs/pull/<n>/head` and base branch when hints exist, else fall back to a broad fetch — but ALWAYS map destinations under `refs/bistro/<safeID>/`, e.g. `+refs/pull/123/head:refs/bistro/ab12cd34ef567890/pull/123/head`. `Tags: gitlib.NoTags`.
- go-git fetch on a repo opened with PlainOpen does not touch worktree/HEAD — the safety test still asserts it.

- [ ] **Step 1: Failing tests** using two `t.TempDir()` repos (an "upstream" bare-ish origin and a "user clone"), built with go-git in-test (commit files programmatically; no network):
  - `TestFetchIntoLocal_WritesOnlyBistroRefs` — new commit in upstream; fetch; assert SHA resolvable, and ref list contains it only under `refs/bistro/`.
  - `TestFetchIntoLocal_NeverTouchesUserState` — record HEAD target, all `refs/heads/*`, `refs/tags/*`, `refs/remotes/*`, the index file bytes (`.git/index`), and worktree file hashes before; fetch; assert all identical after.
  - `TestFetchIntoLocal_RemoteMismatch` — repo whose origin parses to a different identity → error with code `remote_mismatch`.
  - `TestFetchIntoLocal_SkipsWhenPresent` — SHAs already local → no error, no new refs.
  - `TestFetchIntoLocal_TargetedRefspecsOnly` — with opts carrying PR number + base branch, only those refs land under `refs/bistro/<safeID>/` (no unrelated upstream branches appear).
- [ ] **Step 2: Run** `cd go && go test ./internal/git/ -run TestFetchIntoLocal -race -v` — FAIL.
- [ ] **Step 3: Implement.** Reuse `c.auth()` for transport auth.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(git): read-only fetch into refs/bistro on local repos"`

---

### Task 6: Cache root helper + local-repo resolution in clone path

**Files:**
- Modify: `go/internal/git/paths.go`
- Modify: `go/internal/git/clone.go` (inside `cloneWithOptions`, after the cache check ~line 140)
- Test: `go/internal/git/clone_local_test.go`

Wiring: the git client can't import `store` (avoid cycle); define a narrow resolver interface in `git`:

```go
// LocalRepoResolver reports a registered local checkout for a repo URL.
// Implemented by an adapter over store.RepoLocationStore in package server.
type LocalRepoResolver interface {
	// LocalPath returns (path, true) only for source='local' rows.
	LocalPath(repoURL string) (string, bool)
	// OnLocalDegraded records a broken local repo (missing path / mismatch).
	OnLocalDegraded(repoURL, status, code, msg string)
	// OnUsed bumps last_used_at.
	OnUsed(repoURL string)
	// OnCacheCloned records a cache clone: source='cache', local_path=path,
	// last_used_at=now (wraps RepoLocationStore.SetCache). Called after every
	// successful cache clone AND on cache-hit reuse (idempotent upsert).
	OnCacheCloned(repoURL, path string)
}

func (c *Client) SetLocalResolver(r LocalRepoResolver) // nil = disabled
func (c *Client) SetLocks(l *RepoLocks)
```

New logic in `cloneWithOptions` — the local-resolver check runs FIRST, before the existing in-memory cache lookup (a cache hit must never shadow a registered local repo, or a broken local repo would silently fall back to a stale cache clone). Entire operation wrapped in `locks.WithRepo`. Order: (a) `BISTRO_REPO_DIR` set → skip resolver, existing behavior; (b) resolver reports a local repo → local path branch below, return early (success or LocationError — never fall through); (c) no local registration → existing in-memory cache check, then cache clone:

1. If resolver returns a local path:
   - path missing on disk → `OnLocalDegraded(..., "missing", "path_missing", ...)`, return a `*LocationError` (`{RepoID, Operation, Code, Message}` — new error type in `identity.go` or `localfetch.go`). Do NOT fall through to cache.
   - else `FetchIntoLocal` with base/head/required SHAs; on `remote_mismatch` degrade + return LocationError; on success `OnUsed`, return `CloneResult{LocalPath: localPath}`.
2. `BISTRO_REPO_DIR` set → skip resolver entirely (env override wins; existing behavior otherwise unchanged).
3. Cache clone path: unchanged, but destination becomes `CacheRoot()/<owner>/<repo>` (add `func CacheRoot() (string, error)` exported wrapper of today's `getRepoDir` fallback logic, and derive owner/repo dirs from the parsed URL). Legacy flat paths still work because resolution goes through the registered `local_path` / in-memory cache first.
4. After a successful cache clone (and on a validated cache hit), call `resolver.OnCacheCloned(repoURL, localPath)` so SQLite reflects `source='cache'` + `last_used_at` — this is what feeds the settings cache card.

- [ ] **Step 1: Failing tests** with a fake resolver struct:
  - local path exists → result LocalPath == local path, `OnUsed` called, no clone dir created.
  - local path missing → LocationError code `path_missing`, degraded callback called, cache root untouched.
  - local registration wins over a pre-warmed in-memory cache entry for the same repo/SHAs (seed the cache, then register local: result must be the local path); broken local + warm cache entry still returns LocationError, not the cached clone.
  - resolver nil → behavior identical to today (existing tests keep passing).
  - `BISTRO_REPO_DIR` set (t.Setenv) → resolver never consulted.
  - cache clone succeeds (no local registration) → `OnCacheCloned` called with the clone path; called again (idempotent) on a subsequent cache hit.
- [ ] **Step 2: Run** `cd go && go test ./internal/git/ -run TestCloneLocal -race -v` — FAIL.
- [ ] **Step 3: Implement.** Keep the diff inside `cloneWithOptions` small: one `if c.resolveLocal(...)` block that returns early.
- [ ] **Step 4: Run full package** `cd go && go test ./internal/git/ -race` — PASS (including all pre-existing clone tests).
- [ ] **Step 5: Commit** `git commit -m "feat(git): resolve registered local repos before cache clone"`

---

### Task 7: Dev folder scanner

**Files:**
- Create: `go/internal/scan/scanner.go`
- Test: `go/internal/scan/scanner_test.go`

```go
package scan

const (
	MaxCandidates = 500
	MaxDuration   = 30 * time.Second
)

type Found struct {
	Identity git.Identity
	Path     string // canonical
}

type Result struct {
	Found   []Found
	Partial bool
	Skipped int
}

// ValidateDevFolder canonicalizes and rejects root/home/system dirs,
// non-dirs, unreadable dirs. Returns the canonical path.
func ValidateDevFolder(path string) (string, error)

// Scan walks depth<=2 (children + one nested level) for .git dirs.
// Symlinked dirs are not followed. Bare repos/submodules skipped
// (a .git *file* = submodule/worktree pointer → skip).
func Scan(ctx context.Context, devFolder string) (Result, error)
```

- [ ] **Step 1: Failing tests** building layouts in `t.TempDir()` with real `git` via go-git PlainInit + remote config:
  - finds repo at depth 1 and depth 2; ignores depth 3.
  - skips symlinked dir (create with `os.Symlink`).
  - skips repo with non-github remote; skips repo with no remote.
  - prefers `origin` when multiple remotes exist; falls back to first parseable remote when `origin` is absent.
  - `ValidateDevFolder` rejects `/`, `os.UserHomeDir()`, missing path, file path, and unreadable dir (chmod 000; skip on Windows).
  - partial: with `MaxCandidates` overridden to 2 via a package-level var, 3 candidates → `Partial=true, Skipped>=1`.
- [ ] **Step 2: Run** `cd go && go test ./internal/scan/ -v` — FAIL.
- [ ] **Step 3: Implement** with `os.ReadDir` two levels (no `filepath.WalkDir` — depth control is trivial with two loops), `filepath.EvalSymlinks` for canonicalization, `git.ParseRemoteURL` on `origin` (fall back to first parseable remote).
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(scan): bounded dev folder repo scanner"`

---

### Task 8: Cache maintenance (stats, prune, clear-all)

**Files:**
- Create: `go/internal/git/cachemaint.go`
- Test: `go/internal/git/cachemaint_test.go`

```go
package git

type CacheRepoStat struct {
	RepoID     string
	Path       string
	SizeBytes  int64
	LastUsedAt *time.Time
	Stale      bool // last used > 30 days ago (or never)
}

const StaleAfter = 30 * 24 * time.Hour

// CacheStats sizes each registered cache repo dir (WalkDir sum).
// ponytail: O(files) walk per call; fine for a settings page open.
func CacheStats(repos []CacheEntryInput) ([]CacheRepoStat, error)

// DeleteCacheDir removes one cache dir after safety checks:
// canonical path, not a symlink, inside canonical CacheRoot,
// matches <root>/<owner>/<repo> or a registered legacy local_path.
func DeleteCacheDir(path string) error

// SweepOrphans removes dirs under CacheRoot()/<owner>/<repo> depth-2 layout
// that are not in `known` (registered canonical cache paths). Never follows
// symlinks; skips (and reports) anything odd rather than deleting it.
func SweepOrphans(known map[string]bool) (removed []string, err error)
```

Prune/clear orchestration lives in the handler (Task 9) — it owns DB + locks; this file is pure filesystem.

- [ ] **Step 1: Failing tests** in `t.TempDir()` acting as cache root (set `BISTRO_REPO_DIR` via `t.Setenv` — it IS the cache-root override in tests):
  - `CacheStats` sums nested file sizes; stale flag at 31 days; never-used = stale.
  - `DeleteCacheDir` refuses: symlink path, path outside root, root itself; deletes a valid `<root>/<owner>/<repo>`.
  - `SweepOrphans` removes unknown repo dir, keeps known, skips symlink.
- [ ] **Step 2: Run** `cd go && go test ./internal/git/ -run TestCache -v` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(git): cache stats, safe delete, orphan sweep"`

---

### Task 9: WebSocket handlers

**Files:**
- Create: `go/internal/server/handler/repolocation.go`
- Test: `go/internal/server/handler/repolocation_test.go`

Follow the existing handler pattern (`repo.go`): constructor takes narrow interfaces + `*slog.Logger`, returns `ws.MessageHandler`; parse `msg.Data` with `json.Unmarshal`; respond with a typed struct. Error responses use the structured envelope from the file map (marshal a `LocationErrorPayload{RepoID, Operation, Code, Message}` as the response with `"ok": false` rather than returning a bare Go error, so the UI gets machine codes).

Message types and payloads:

| Type | Request | Response |
|---|---|---|
| `VALIDATE_DEV_FOLDER` | `{path}` | `{ok, canonical_path}` or error envelope (`invalid_path`) |
| `SCAN_DEV_FOLDER` | `{path, reason}` | `{ok, discovered, healthy, degraded, skipped, partial}` |
| `ADD_REPO_BY_URL` | `{url, destination: "dev_folder"\|"cache"\|null, dev_folder_path: string\|null}` | found-local: `{ok, repo_id, source:"local"}`; needs-destination: envelope `destination_required`; cloned: `{ok, repo_id, source}`; conflict: envelope `destination_conflict` |
| `REMOVE_REPO_PREFLIGHT` | `{repo_id}` | `{ok, counts:{prs,comments,threads,patchsets,actions}}` — read-only, feeds the confirm dialog |
| `GET_CACHE_STATS` | `{}` | `{ok, total_bytes, active_bytes, stale_bytes, repos:[{repo_id,size_bytes,last_used_at,stale}]}` |
| `PRUNE_CACHE` | `{}` (30-day rule server-side) | `{ok, removed:[repo_id], failed:[{repo_id,code,message}]}` |
| `CLEAR_CACHE` | `{}` | same shape + `orphans_removed` |
| `DELETE_CACHE_REPO` | `{repo_id}` | `{ok}` or envelope |
| `RECOVER_REPO_PATH` | `{repo_id, path}` | `{ok}` or envelope (`remote_mismatch`/`path_missing`) |
| `RECOVER_REPO_TO_CACHE` | `{repo_id}` | `{ok}` or envelope (`clone_failed`) — degraded cleared only on success |
| `REMOVE_REPO` | `{repo_id}` | `{ok, deleted:{prs,comments,threads,patchsets,actions}}` — counts, single DB tx commits before cache-dir delete |

`SCAN_DEV_FOLDER` handler behavior: `scan.ValidateDevFolder` → `scan.Scan` → for each Found `UpsertLocal`; then reconcile EVERY `source='local'` row by health-checking its stored path directly (not just scan membership):
- `os.Stat` fails with not-exist → `MarkDegraded("missing","path_missing",...)`
- `os.Stat`/open fails with permission → `MarkDegraded("permission_denied","permission_denied",...)`
- path opens but no remote matches the row's identity (reuse `ParseRemoteURL` over `repo.Remotes()`) → `MarkDegraded("remote_mismatch","remote_mismatch",...)`
- healthy → `ClearDegraded`.

The synchronous response carries the final tallies (the spec's "progress and completion events" contract is satisfied by this single completion response — the scan is bounded at 30s/500 candidates, so incremental progress events are YAGNI; note this deviation consciously). Triggers the existing sync loop (`a.syncTrigger`) once at the end.

`ADD_REPO_BY_URL` handler behavior (Go side, all tested here — not just UI):
1. `ParseRemoteURL` → envelope `invalid_url` on failure.
2. Upsert-check: registered local row → return found-local.
3. Targeted dev-folder check: if devFolderPath known (passed in request payload as `dev_folder_path` from settings), stat `<dev>/<owner>/<repo>` and validate remote → register local + return found-local.
4. No destination in payload → envelope `destination_required`.
5. `destination="dev_folder"`: destination dir `<dev>/<owner>/<repo>`; exists+matching → register local; exists+non-matching/non-empty → envelope `destination_conflict`; else clone. Clone failure → remove partial dir only if Bistro created it this call → envelope `clone_failed`/`auth_failed`/`cancelled` (map from go-git transport errors; context.Canceled → `cancelled`).
6. `destination="cache"`: clone under `CacheRoot()/<owner>/<repo>` with same conflict/cleanup rules; `SetCache` on success.
7. Post-clone remote validation; mismatch → delete Bistro-created dir, envelope `remote_mismatch`.

`REMOVE_REPO` deletion order (spec): count rows → single transaction deleting `action_queue` rows by pr_id, `patchsets`, `last_reviewed_sha`, `files_viewed`, then `prs` (cascades cover comments/files/pr_diffs/review_threads), then the `repos` row → commit → then `DeleteCacheDir` if `source='cache'` (failure logged, not rolled back).

- [ ] **Step 1: Failing tests** per handler with in-memory migrated DB + `t.TempDir()` fixtures (fake clone client via small interface for ADD_REPO_BY_URL / RECOVER_REPO_TO_CACHE). Include: error envelope shape assertions for each operation type (decode JSON; assert `repo_id`, `operation`, `code`, `message` all present); SCAN_DEV_FOLDER marks missing/permission_denied/remote_mismatch correctly (chmod a dir 000 for permission — skip on Windows; point a row at a repo whose origin was rewritten for mismatch); ADD_REPO_BY_URL covers found-local via targeted check, destination_required, destination_conflict, clone_failed with partial-dir cleanup, cancelled via cancelled context; REMOVE_REPO_PREFLIGHT counts match what REMOVE_REPO then deletes; REMOVE_REPO leaves sibling repos' PRs intact and rolls back on injected tx failure; PRUNE_CACHE skips `source='local'` rows entirely, deletes only rows with `last_used_at` older than 30 days (row at 29 days survives, row at 31 days deleted, NULL last_used_at deleted), and clears `source`/`local_path` only for successfully deleted dirs — an injected `DeleteCacheDir` failure leaves that row's DB fields unchanged and reports it in `failed`; CLEAR_CACHE removes registered cache rows AND sweeps orphan dirs (assert `orphans_removed`), holding the maintenance lock (assert a concurrent `WithRepo` op blocks until done); DELETE_CACHE_REPO deletes one repo's dir + clears its fields, refuses `source='local'` rows with an envelope.
- [ ] **Step 2: Run** `cd go && go test ./internal/server/handler/ -run TestRepoLocation -race -v` — FAIL.
- [ ] **Step 3: Implement** handlers.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(server): repo location and cache maintenance handlers"`

---

### Task 10: Wire into app.go + startup scan

**Files:**
- Modify: `go/internal/server/app.go` (handler registration block ~line 360; startup hook near sync loop start)
- Test: `go/internal/server/app_repolocation_test.go` (registration smoke test only)

- [ ] **Step 1:** Register all Task 9 handlers with `a.wsServer.RegisterHandler(...)`, constructing `RepoLocationStore`, `scan` funcs, `RepoLocks`, and the `LocalRepoResolver` adapter (adapter struct in `app.go` or a small `go/internal/server/localresolver.go`: wraps `RepoLocationStore`, maps repoURL → identity via `git.ParseRemoteURL`). Call `gitClient.SetLocalResolver(...)`/`SetLocks(...)` where the client is built.
- [ ] **Step 2:** Startup scan: Electron sends `SCAN_DEV_FOLDER` on engine-ready (renderer side, Task 12) — no Go-side timer needed. Nothing else to add.
- [ ] **Step 3: Run** `cd go && go vet ./... && go test -race ./...` — all PASS.
- [ ] **Step 4: Commit** `git commit -m "feat(server): wire repo location handlers and local resolver"`

---

### Task 11: Electron settings field + folder picker IPC

**Files:**
- Modify: `electron/src/types/settings.ts` (add `RepositoriesSettings { devFolderPath: string | null }`, add to `AppSettings` + `DEFAULT_SETTINGS`)
- Modify: `electron/src/main/ipc.ts` (`dialog:pick-folder` via `dialog.showOpenDialog({properties:['openDirectory']})`)
- Modify: `electron/src/main/preload.ts` + `electron/src/renderer/types/window.d.ts` (expose `pickFolder(): Promise<string | null>`)
- Test: `electron/src/types/settings.test.ts` (or nearest existing settings test file — check `pnpm test -- settings` first)

- [ ] **Step 1: Failing test** — `DEFAULT_SETTINGS.repositories.devFolderPath === null`; settings normalise keeps unknown-field discard behavior (existing `mergeSettings` handles it — assert a settings object with `repositories` round-trips).
- [ ] **Step 2: Run** `cd electron && pnpm test -- settings` — FAIL.
- [ ] **Step 3: Implement** type + default + IPC + preload. No schema bump needed (additive field, `mergeSettings` fills defaults).
- [ ] **Step 4: Run** `cd electron && pnpm type-check && pnpm test -- settings` — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(electron): repositories.devFolderPath setting and folder picker"`

---

### Task 12: Renderer WS plumbing + startup scan send

**Files:**
- Modify: `electron/src/renderer/hooks/useWebSocket.ts` (payload interfaces for scan results + location errors; on connection-ready: if `settingsStore.state.repositories.devFolderPath`, `send('SCAN_DEV_FOLDER', {path, reason:'startup'})`)
- Create: `electron/src/renderer/types/repoLocation.ts` — TS mirror of message payloads + the machine-code union type:

```ts
export type LocationErrorCode =
  | 'path_missing' | 'remote_mismatch' | 'permission_denied'
  | 'invalid_url' | 'invalid_path' | 'destination_required'
  | 'destination_conflict' | 'clone_failed' | 'auth_failed'
  | 'cancelled' | 'delete_failed' | 'scan_partial';

export interface LocationError {
  repo_id: string;
  operation: 'scan' | 'fetch' | 'diff' | 'clone' | 'prune' | 'recover';
  code: LocationErrorCode;
  message: string;
}
```

- Test: `electron/src/renderer/hooks/useWebSocket.test.ts` (extend existing test file if present; else create)

- [ ] **Step 1: Failing test** — with devFolderPath set in a test settings store, connection-open triggers exactly one SCAN_DEV_FOLDER send; with null, none.
- [ ] **Step 2: Run** `cd electron && pnpm test -- useWebSocket` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS. `pnpm type-check`.
- [ ] **Step 5: Commit** `git commit -m "feat(renderer): scan dev folder on engine ready"`

---

### Task 13: Onboarding dev-folder step

**Files:**
- Modify: `electron/src/renderer/pages/OnboardingPage.tsx`
- Test: `electron/src/renderer/pages/OnboardingPage.test.tsx`

Behavior: before the existing finish footer, add a card: "Use your existing projects folder" with two actions — **Choose folder** (calls `window.electron.pickFolder()`, then `send('VALIDATE_DEV_FOLDER')`; on ok persists via `updateSettings({repositories:{devFolderPath}})` and sends `SCAN_DEV_FOLDER` with reason `settings-change`; shows discovered count) and **Skip** (Space/finish keeps working unchanged, devFolderPath stays null). Disclosure line per spec: Bistro will fetch PR objects into these repos under `refs/bistro/*` and never touches branches or files.

- [ ] **Step 1: Failing tests** (Vitest + Testing Library, mock `window.electron.pickFolder` and `useWebSocket().send`): skip leaves setting null and completes; choose-folder happy path persists canonical path from VALIDATE response and fires scan; validation error shows message and persists nothing.
- [ ] **Step 2: Run** `cd electron && pnpm test -- OnboardingPage` — FAIL.
- [ ] **Step 3: Implement** (match existing stitch styling classes used in the file).
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(renderer): onboarding dev folder step"`

---

### Task 14: Command palette — Add repository by URL

**Files:**
- Create: `electron/src/renderer/components/AddRepoDialog.tsx`
- Test: `electron/src/renderer/components/AddRepoDialog.test.tsx`
- Modify: `electron/src/renderer/components/CommandPalette.tsx` (add to `COMMANDS`, id `add-repo-by-url`, opens the dialog via `OverlayContext`)
- Modify: `electron/src/renderer/overlays/OverlayContext.tsx` (new overlay id)

Dialog flow: paste URL → `send('ADD_REPO_BY_URL', {url, destination: null, dev_folder_path: settingsStore.state.repositories.devFolderPath})` (always include the setting — Go needs it for the targeted dev-folder check) → on `destination_required`, show two options — **Dev folder** (default-focused when configured) / **Bistro cache** (default when not) → resend with destination (same `dev_folder_path`) → success toast + close; `destination_conflict`/`invalid_url`/`auth_failed` render by machine code.

- [ ] **Step 1: Failing tests**: renders input; found-local response closes with toast; destination prompt defaults per devFolderPath presence; error codes map to distinct messages (assert on code-driven strings, not server text).
- [ ] **Step 2: Run** `cd electron && pnpm test -- AddRepoDialog` — FAIL.
- [ ] **Step 3: Implement** dialog + palette command + overlay wiring.
- [ ] **Step 4: Run** — PASS. Also `pnpm test -- CommandPalette`.
- [ ] **Step 5: Commit** `git commit -m "feat(renderer): add repository by URL from command palette"`

---

### Task 15: Settings — Repositories section with cache card

**Files:**
- Create: `electron/src/renderer/components/CacheUsageCard.tsx`
- Test: `electron/src/renderer/components/CacheUsageCard.test.tsx`
- Modify: `electron/src/renderer/pages/SettingsPage.tsx` (new `RepositoriesSection` in `SECTIONS` + render list)

RepositoriesSection: dev folder row (current path or "Not set", Change via picker+validate+persist+scan, Clear), scan status line (last result incl. partial notice), then `CacheUsageCard`.

CacheUsageCard (per spec, no paths shown): fetches `GET_CACHE_STATS` on mount; horizontal bar — blue segment = active bytes, gray = stale bytes — total size text right-aligned; "Prune repos older than 30 days" label + Prune button (`PRUNE_CACHE`); gray "Clear all cached repos" button with confirm dialog (`CLEAR_CACHE`); accordion table: repo identity, size, last used, trash icon (`DELETE_CACHE_REPO` + refetch). Failed deletions → generic "Something went wrong, check the logs" toast.

- [ ] **Step 1: Failing tests**: segment widths proportional to active/stale bytes; prune button sends PRUNE_CACHE and refetches; clear-all requires confirm; row trash sends DELETE_CACHE_REPO; delete_failed code → generic error toast.
- [ ] **Step 2: Run** `cd electron && pnpm test -- CacheUsageCard` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS. `pnpm type-check && pnpm lint`.
- [ ] **Step 5: Commit** `git commit -m "feat(renderer): repositories settings section with cache usage card"`

---

### Task 16: Degraded repo UI + recovery dialog

**Files:**
- Create: `electron/src/renderer/components/RepoRecoveryDialog.tsx`
- Test: `electron/src/renderer/components/RepoRecoveryDialog.test.tsx`
- Modify: `electron/src/renderer/components/PRListTable.tsx` (gray out rows whose repo is degraded; row action opens recovery dialog)
- Modify: repo list plumbing — `REQUEST_REPO_LIST` response already flows through `useWebSocket`; extend Go's `NewRequestRepoListHandler` (`go/internal/server/handler/repo.go:94`) to also select `source, location_status, location_error_code, location_error` and include them in the payload (small Go diff + test update in `repo_test.go`, do it in this task).

RepoRecoveryDialog: shows `Repo owner/name could not sync: <message>` and three actions — **Choose new path** (picker → `RECOVER_REPO_PATH`; remote_mismatch shown inline), **Move to Bistro cache** (warning copy → `RECOVER_REPO_TO_CACHE`), **Remove repo and PR history** (send `REMOVE_REPO_PREFLIGHT` first, show its counts in the strong-confirm copy — "This deletes N PRs, M comments…" — then send `REMOVE_REPO` only after explicit confirm).

- [ ] **Step 1: Failing tests** (Go): repo list payload carries location fields. (TS): degraded repo renders grayed with error text; each action sends the right message; UI state driven by `location_error_code`, not message text (assert a wrong-text/right-code payload still renders the mismatch state).
- [ ] **Step 2: Run** Go handler test + `pnpm test -- RepoRecoveryDialog` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `git commit -m "feat: degraded repo states and recovery actions"`

---

### Task 17: Full verification + docs

- [ ] **Step 1:** `cd go && go vet ./... && go test -race ./...` — all PASS.
- [ ] **Step 2:** `cd electron && pnpm type-check && pnpm lint && pnpm test` — all PASS.
- [ ] **Step 3:** Update `docs/go-project-overview.md` (new `scan` package, repos location columns, new handlers) and `CLAUDE.md` only if commands/conventions changed (they didn't — skip unless something moved).
- [ ] **Step 4:** Commit `git commit -m "docs: update go project overview for repo location feature"`

---

## Execution notes

- Tasks 1–10 (Go) are strictly ordered. Tasks 11–16 (Electron) depend on 9's message contract but not on each other beyond 11→12→13; 14/15/16 can go in any order after 12.
- neverthrow for all new renderer async paths; no try/catch in committed code.
- Immutability + early returns per repo conventions; parameterized SQL only.
- Commit after every task; never batch tasks into one commit.
