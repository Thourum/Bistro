# Local Expertise Scoring (Phase 4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-file expertise scores computed from local git history, powering a "Touches Your Code" PR filter — opt-in per repo, full-repo index on enable, incremental updates on sync.

**Architecture:** New contained package `go/internal/expertise/` (identity, git CLI wrapper, scoring math, excludes, indexer, updater, nudge) writing three new SQLite tables via sqlc-generated queries; WS handlers in `internal/server/handler/expertise.go`; orchestrator hook post-diff (classify precedent); renderer gets a filter, sidebar count, per-PR "why" data, nudge banner, palette commands, and a Settings section.

**Tech Stack:** Go 1.26, system git via bounded `os/exec` (range-diff precedent), sqlc + goose (post-NIH-audit conventions), gorilla/websocket handlers, React 19 + TanStack, Vitest.

**Authoritative spec:** `docs/superpowers/specs/2026-07-15-local-expertise-scoring-design.md` — read it before starting. Constants, formulas, message names, and degradation rules there are normative.

**Conventions you MUST follow (post-rebase, 2026-07-15):**
- Store SQL lives in `go/internal/store/queries/*.sql` with sqlc annotations. Run `cd go && sqlc generate` (config `go/sqlc.yaml`, output `internal/store/db/` — NEVER hand-edit that dir). Wrapper methods go in a thin store file.
- Migrations: `go/internal/store/migrations/000NN_description.sql` with `-- +goose Up` / `-- +goose Down` headers. Next number: **00031**. Schema changes require bumping `reconcileBaselineVersion` in `go/internal/store/migration_runner.go` and adding a matching check in `currentSchemaExists` (the comment on the constant explains this).
- Error handling: Go errors idiomatically; renderer TS uses `neverthrow` — NEVER `try`/`catch` in committed code.
- Immutability in TS (new objects, no mutation).
- **Do NOT test individual React components.** Vitest covers pure utility functions/hooks logic only.
- Verify commands: `cd go && go vet ./... && go test -race ./...`; `cd electron && pnpm type-check && pnpm lint && pnpm test`.

## File map

| File | Action | Responsibility |
|---|---|---|
| `go/internal/store/migrations/00031_add_expertise.sql` | Create | Three tables: `file_expertise`, `expertise_repo_state`, `expertise_settings` |
| `go/internal/store/migration_runner.go` | Modify | Bump baseline to 31; add `file_expertise` existence check |
| `go/internal/store/queries/expertise.sql` | Create | sqlc queries for all three tables |
| `go/internal/store/db/*` | Generated | `sqlc generate` output — commit, never edit |
| `go/internal/expertise/store.go` (+`store_test.go`) | Create | Thin wrapper over generated queries; owns expertise DB access |
| `go/internal/expertise/score.go` (+`score_test.go`) | Create | Pure scoring math + constants |
| `go/internal/expertise/identity.go` (+`identity_test.go`) | Create | "Who is me" resolution |
| `go/internal/expertise/excludes.go` (+`excludes_test.go`) | Create | Built-in + user globs + linguist-generated |
| `go/internal/expertise/gitcli.go` (+`gitcli_test.go`) | Create | Bounded system-git wrapper (probe, ls-files, blame, log passes) |
| `go/internal/expertise/indexer.go` (+`indexer_test.go`) | Create | Full-repo pass, progress, cancellation, restart-on-boot |
| `go/internal/expertise/updater.go` (+`updater_test.go`) | Create | Incremental re-score of PR-touched stale files |
| `go/internal/expertise/nudge.go` (+`nudge_test.go`) | Create | 10-reviewed-PRs nudge derivation |
| `go/internal/sync/orchestrator.go` | Modify | `WithExpertise(...)` option + post-diff call |
| `go/internal/server/handler/expertise.go` (+`expertise_test.go`) | Create | All EXPERTISE_* WS handlers + GET_EXPERTISE |
| `go/internal/server/handler/pr.go` | Modify | `touches_you` on PR list rows |
| `go/internal/server/app.go` | Modify | Construct engine, register handlers, boot-restart of interrupted indexing |
| `electron/src/renderer/types/index.ts` | Modify | `touches_you` on `PR`; expertise payload types |
| `electron/src/renderer/hooks/useWebSocket.ts` | Modify | New event cases |
| `electron/src/renderer/utils/expertise.ts` (+test) | Create | Pure helpers (filter predicate, why-line formatting) |
| `electron/src/renderer/components/FilterMenu.tsx` | Modify | "Touches Your Code" filter |
| `electron/src/renderer/components/StatusSidebar.tsx` | Modify | Count + indexing status |
| `electron/src/renderer/components/ExpertiseNudge.tsx` | Create | Dismissable banner |
| `electron/src/renderer/components/CommandPalette.tsx` | Modify | `enable expertise` / `disable expertise` |
| PR detail view (component that renders file list) | Modify | Per-file expertise line + expandable "why" |
| Settings page component | Modify | Alias emails, min-lines, per-repo excludes |

---

### Task 1: Migration 00031 + runner baseline

**Files:**
- Create: `go/internal/store/migrations/00031_add_expertise.sql`
- Modify: `go/internal/store/migration_runner.go`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- Migration: 00031_add_expertise
-- Description: Phase 4 local expertise scoring — per-file scores, per-repo
-- indexing state, and global/per-repo expertise settings.

CREATE TABLE file_expertise (
    repo_id         TEXT NOT NULL,            -- 'provider:owner/repo'
    file_path       TEXT NOT NULL,
    author_email    TEXT NOT NULL,
    author_login    TEXT,                     -- resolved when matchable, else NULL
    surviving_lines INTEGER NOT NULL,
    total_lines     INTEGER NOT NULL,
    commit_score    REAL NOT NULL,
    is_original     INTEGER NOT NULL,
    score           REAL NOT NULL,
    last_commit_at  TIMESTAMP,
    dormant         INTEGER NOT NULL DEFAULT 0,
    computed_at     TIMESTAMP NOT NULL,
    computed_at_sha TEXT NOT NULL,
    PRIMARY KEY (repo_id, file_path, author_email)
);
CREATE INDEX idx_file_expertise_repo_path ON file_expertise(repo_id, file_path);

CREATE TABLE expertise_repo_state (
    repo_id            TEXT PRIMARY KEY,
    enabled            INTEGER NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'idle',
    indexed_sha        TEXT,
    indexed_at         TIMESTAMP,
    error_code         TEXT,
    error              TEXT,
    files_total        INTEGER,
    files_done         INTEGER,
    nudge_dismissed_at TIMESTAMP
);

CREATE TABLE expertise_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE expertise_settings;
DROP TABLE expertise_repo_state;
DROP INDEX idx_file_expertise_repo_path;
DROP TABLE file_expertise;
```

No FK to `repos` — deliberate, see spec ("Note on `repos` FK").

- [ ] **Step 2: Bump the reconcile baseline**

In `go/internal/store/migration_runner.go`: change `const reconcileBaselineVersion int64 = 30` to `31`, and in `currentSchemaExists` add a table-existence check for `file_expertise` (use the same `tableExists`-style helper the function already uses for whole-table checks — read the function first; a migration that creates tables needs a table check, not a column check).

- [ ] **Step 3: Run store tests**

Run: `cd go && go test ./internal/store/`
Expected: PASS (migration applies cleanly; the runner's own tests cover goose application).

- [ ] **Step 4: Commit**

```bash
git add go/internal/store/migrations/00031_add_expertise.sql go/internal/store/migration_runner.go
git commit -m "feat(expertise): add migration 00031 — file_expertise, repo state, settings"
```

---

### Task 2: sqlc queries + expertise store wrapper

**Files:**
- Create: `go/internal/store/queries/expertise.sql`
- Generated: `go/internal/store/db/expertise.sql.go` (via `sqlc generate`)
- Create: `go/internal/expertise/store.go`
- Test: `go/internal/expertise/store_test.go`

- [ ] **Step 1: Write the annotated queries**

`go/internal/store/queries/expertise.sql` — follow the style of `queries/current_user.sql`:

```sql
-- name: UpsertFileExpertise :exec
INSERT INTO file_expertise (
    repo_id, file_path, author_email, author_login, surviving_lines,
    total_lines, commit_score, is_original, score, last_commit_at,
    dormant, computed_at, computed_at_sha
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id, file_path, author_email) DO UPDATE SET
    author_login = excluded.author_login,
    surviving_lines = excluded.surviving_lines,
    total_lines = excluded.total_lines,
    commit_score = excluded.commit_score,
    is_original = excluded.is_original,
    score = excluded.score,
    last_commit_at = excluded.last_commit_at,
    dormant = excluded.dormant,
    computed_at = excluded.computed_at,
    computed_at_sha = excluded.computed_at_sha;

-- name: DeleteFileExpertiseForFile :exec
DELETE FROM file_expertise WHERE repo_id = ? AND file_path = ?;

-- name: GetFileExpertise :many
SELECT * FROM file_expertise
WHERE repo_id = ? AND file_path = ?
ORDER BY score DESC;

-- name: GetExpertiseRepoState :one
SELECT * FROM expertise_repo_state WHERE repo_id = ?;

-- name: UpsertExpertiseRepoState :exec
INSERT INTO expertise_repo_state (
    repo_id, enabled, status, indexed_sha, indexed_at, error_code, error,
    files_total, files_done, nudge_dismissed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id) DO UPDATE SET
    enabled = excluded.enabled,
    status = excluded.status,
    indexed_sha = excluded.indexed_sha,
    indexed_at = excluded.indexed_at,
    error_code = excluded.error_code,
    error = excluded.error,
    files_total = excluded.files_total,
    files_done = excluded.files_done,
    nudge_dismissed_at = excluded.nudge_dismissed_at;

-- name: UpdateExpertiseIndexProgress :exec
UPDATE expertise_repo_state SET files_done = ?, files_total = ? WHERE repo_id = ?;

-- name: ListInterruptedExpertiseRepos :many
SELECT repo_id FROM expertise_repo_state WHERE enabled = 1 AND status = 'indexing';

-- name: GetExpertiseSetting :one
SELECT value FROM expertise_settings WHERE key = ?;

-- name: UpsertExpertiseSetting :exec
INSERT INTO expertise_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: CountReviewedPRsForRepo :one
-- last_reviewed_sha is account-namespaced (migration 022); filter by the
-- current account, not just login, or another account on the same machine
-- inflates the count.
SELECT COUNT(DISTINCT lrs.pr_id)
FROM last_reviewed_sha lrs
JOIN prs p ON p.id = lrs.pr_id
WHERE p.repo_id = ?
  AND lrs.account_provider = ?
  AND lrs.account_login = ?
  AND lrs.login = ?;

-- name: GetTopExpertsForFiles :many
SELECT repo_id, file_path, author_email, author_login, score, dormant
FROM file_expertise
WHERE repo_id = ? AND dormant = 0
ORDER BY file_path, score DESC;
```

- [ ] **Step 2: Generate and build**

Run: `cd go && sqlc generate && go build ./...`
Expected: new methods appear in `internal/store/db/`; build passes. Commit generated files, never edit them.

- [ ] **Step 3: Write failing store-wrapper test**

`go/internal/expertise/store_test.go` — open an in-memory DB the way `internal/store` tests do (read `go/internal/store/current_user_test.go` first and mirror its setup helper, which runs migrations). Test: upsert two authors' rows for one file, read back via `GetFileExpertise`, assert ordering by score DESC; upsert repo state, flip status, read back; `CountReviewedPRsForRepo` returns 0 on empty DB.

- [ ] **Step 4: Run test to verify it fails**

Run: `cd go && go test ./internal/expertise/ -run TestStore -v`
Expected: FAIL (package doesn't exist yet).

- [ ] **Step 5: Implement `store.go`**

`Store` struct holding `*db.Queries` (constructor takes `*sql.DB`, calls `db.New`). Thin methods: `UpsertFileRows(ctx, rows []FileExpertiseRow)` (loop upsert inside a tx via `queries.WithTx`), `ReplaceFileRows(ctx, repoID, path, rows)` (delete-then-insert in one tx — an index/update recomputes a whole file atomically), `RepoState`, `SetRepoState`, `SetProgress`, `InterruptedRepos`, `Setting`/`SetSetting` (JSON-encoded values), `ReviewedPRCount`, `TopExpertsByFile(ctx, repoID) (map[string][]Expert, error)`.

- [ ] **Step 6: Run tests, verify pass; go vet**

Run: `cd go && go vet ./... && go test ./internal/expertise/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add go/internal/store/queries/expertise.sql go/internal/store/db/ go/internal/expertise/
git commit -m "feat(expertise): sqlc queries + store wrapper"
```

---

### Task 3: Scoring math (`score.go`)

**Files:**
- Create: `go/internal/expertise/score.go`
- Test: `go/internal/expertise/score_test.go`

- [ ] **Step 1: Write failing table-driven tests**

Cover (formulas are normative in the spec §Scoring algorithm):
- `CommitScore`: 4 substantial commits today → 1.0 (saturated); one commit 6 months ago → 0.125; zero commits → 0; 12-month window enforcement is the caller's job (document).
- `Score`: surviving 312/480, commit_score 0.4, original → `55*0.65 + 30*0.4 + 15*1 = 35.75+12+15 = 62.75`; empty file (total 0) → surviving term 0.
- `IsExpert`: surviving ≥ minLines → true; score ≥ 40 with few lines → true (EXPERT_SCORE_FLOOR); neither → false.
- `Dormant`: last commit 91 days before now → true; 89 → false.

```go
func TestScore(t *testing.T) {
    got := Score(312, 480, 0.4, true)
    if math.Abs(got-62.75) > 0.01 {
        t.Errorf("Score = %v, want 62.75", got)
    }
}
```

- [ ] **Step 2: Run to verify fail** — `go test ./internal/expertise/ -run TestScore -v` → FAIL (undefined).

- [ ] **Step 3: Implement**

Named constants: `WeightSurviving = 55.0`, `WeightCommits = 30.0`, `WeightOriginal = 15.0`, `CommitsSaturation = 4.0`, `HalfLifeMonths = 6.0`, `SubstantialLineThreshold = 5`, `DormantAfterDays = 90`, `DefaultMinSurvivingLines = 50`, `ExpertScoreFloor = 40.0`, `CommitWindowMonths = 12`. Pure functions: `CommitScore(commitAges []time.Duration) float64`, `Score(surviving, total int, commitScore float64, isOriginal bool) float64`, `IsExpert(surviving, minLines int, score float64) bool`, `IsDormant(lastCommit time.Time, now time.Time) bool`. No I/O in this file.

- [ ] **Step 4: Run to verify pass** — `go test ./internal/expertise/ -v` → PASS.

- [ ] **Step 5: Commit** — `git add go/internal/expertise/score* && git commit -m "feat(expertise): pure scoring math"`

---

### Task 4: Identity resolution (`identity.go`)

**Files:**
- Create: `go/internal/expertise/identity.go`
- Test: `go/internal/expertise/identity_test.go`

- [ ] **Step 1: Write failing tests**

`Identity{Login string, AccountEmail string, AliasEmails []string}` with `Matches(authorName, authorEmail string) bool`:
- `12345+samuelasvanyi@users.noreply.github.com` matches login `samuelasvanyi` (regex `^\d+\+(.+)@users\.noreply\.github\.com$`, group == login, case-insensitive)
- bare `samuelasvanyi@users.noreply.github.com` (old-style noreply) matches
- authorName exactly equal to login (case-insensitive) matches
- AccountEmail exact match (case-insensitive) matches
- alias email matches; unrelated email + name doesn't
- Note: `AccountEmail` comes from the authenticated GitHub account when available; the provider does not currently expose it (verified — no `Email` field in `internal/provider`), so wire `""` for now and rely on noreply/login/alias. Document this in a code comment; adding the API fetch is NOT in this plan's scope.

- [ ] **Step 2: Verify fail**, **Step 3: implement** (pure, no I/O), **Step 4: verify pass**, **Step 5: commit** `feat(expertise): identity resolution`.

---

### Task 5: Excludes (`excludes.go`)

**Files:**
- Create: `go/internal/expertise/excludes.go`
- Test: `go/internal/expertise/excludes_test.go`

- [ ] **Step 1: Write failing tests**

Built-in defaults exclude: `vendor/x.go`, `dist/app.js`, `node_modules/a/b.js`, `a/b.pb.go`, `pnpm-lock.yaml`, `go.sum`, `internal/testdata/f.json`. User globs (e.g. `website/public/**`) merge in. Non-matches pass. Use `github.com/bmatcuk/doublestar` ONLY if already in go.mod — check first; otherwise implement with `path.Match` per segment or a small `**` matcher (KISS: convert `**/` prefix + suffix matching; test-drive it).

Built-ins (constant slice): `vendor/**`, `dist/**`, `node_modules/**`, `**/*.pb.go`, `**/testdata/**`, `**/*.lock`, `pnpm-lock.yaml`, `package-lock.json`, `go.sum`, `yarn.lock`.

`linguist-generated` handling is NOT here — it's a git call (`check-attr`), lives in gitcli and is applied by the indexer after the glob filter.

- [ ] **Step 2–5:** fail → implement `Excluder` (`New(userGlobs []string)`, `Excluded(path string) bool`) → pass → commit `feat(expertise): exclude globs`.

---

### Task 6: System-git wrapper (`gitcli.go`)

**Files:**
- Create: `go/internal/expertise/gitcli.go`
- Test: `go/internal/expertise/gitcli_test.go`

Follow `go/internal/git/rangediff.go` exactly for the probe/availability/degradation pattern (read it first). All commands run with `exec.CommandContext`, `cmd.Dir = repoPath`, per-command timeout (blame: 30s/file; log passes: 120s), and validate SHA args against the `^[0-9a-f]{40}$` pattern where applicable.

- [ ] **Step 1: Write failing integration test with a fixture repo**

Test helper `makeFixtureRepo(t *testing.T) string` builds a repo in `t.TempDir()` with real git commands (skip the whole test file when `git` is absent — mirror how rangediff tests guard):

```go
func makeFixtureRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    run := func(args ...string) {
        cmd := exec.Command("git", args...)
        cmd.Dir = dir
        cmd.Env = append(os.Environ(),
            "GIT_AUTHOR_NAME=alice", "GIT_AUTHOR_EMAIL=alice@example.com",
            "GIT_COMMITTER_NAME=alice", "GIT_COMMITTER_EMAIL=alice@example.com",
            "GIT_AUTHOR_DATE=2026-01-15T10:00:00", "GIT_COMMITTER_DATE=2026-01-15T10:00:00")
        if out, err := cmd.CombinedOutput(); err != nil {
            t.Fatalf("git %v: %v\n%s", args, err, out)
        }
    }
    run("init", "-b", "main")
    // alice creates main.go (10 lines), commits; then bob (switch env)
    // appends 6 lines to main.go and creates util.go; then alice edits 1 line
    // (typo fix — NOT substantial). Use distinct GIT_*_DATE values so decay
    // and dormancy are testable deterministically.
    ...
    return dir
}
```

Assert:
- `LsFiles` returns both files.
- `BlameFile(ctx, dir, "main.go")` returns per-author surviving line counts summing to the file's line count, keyed by `(name, email)`.
- `LogNumstat(ctx, dir, since)` yields per-author-per-file events with changed-line counts and commit times (alice's 1-line edit present but caller filters substantial); result includes per-author last-commit-at across the repo.
- `LogFirstCommits(ctx, dir)` maps `main.go` → alice's email, `util.go` → bob's.
- `GeneratedFiles(ctx, dir, paths)` returns paths with `linguist-generated=true` (add a `.gitattributes` in the fixture marking one file; uses `git check-attr --stdin`).
- `HeadSHA(ctx, dir)` returns a 40-hex string.

- [ ] **Step 2: Verify fail.**

- [ ] **Step 3: Implement**

`GitCLI` struct with `NewGitCLI(log *slog.Logger) *GitCLI` probing `git --version` once (any modern version is fine — no 2.19 floor needed here; availability is binary). Methods:
- `Available() bool`, `UnavailableReason() string`
- `HeadSHA(ctx, repoPath) (string, error)` — `git rev-parse HEAD`
- `LsFiles(ctx, repoPath) ([]string, error)` — `git ls-files -z` (parse NUL-separated)
- `BlameFile(ctx, repoPath, path) (map[AuthorKey]int, int, error)` — `git blame --line-porcelain HEAD -- <path>`; parse `author `/`author-mail ` lines; returns per-author counts + total. Strip `<>` from mail.
- `LogNumstat(ctx, repoPath, since time.Time) (*LogResult, error)` — `git log --numstat --no-renames --pretty=format:%H%x00%ae%x00%an%x00%aI --since=<iso RFC3339>`; parse into per-author-per-file changed-line events with commit timestamps.
- `ActiveAuthors(ctx, repoPath, since time.Time) (map[string]bool, error)` — `git log --pretty=format:%ae --since=<iso RFC3339>`, dedup emails into a set. Dormancy = author email NOT in this set when called with `now - 90 days` (per spec: no commit in this repo within 90 days).
- `LogFirstCommits(ctx, repoPath) (map[string]string, error)` — `git log --diff-filter=A --name-only --no-renames --pretty=format:%x00%ae`; first (oldest) add wins: iterate with `--reverse`.
- `GeneratedFiles(ctx, repoPath, paths []string) (map[string]bool, error)` — `git check-attr linguist-generated --stdin -z`.

Keep each parse function separate and unit-testable. Binary files in numstat show `-` — skip those lines.

- [ ] **Step 4: Verify pass** — `go test ./internal/expertise/ -run TestGitCLI -v` (and confirm the skip path works: `PATH=/nonexistent go test ...` is not required; just assert `Available()==false` behavior via constructing with a bogus `exec` name is overkill — trust the rangediff-pattern probe).

- [ ] **Step 5: Commit** — `feat(expertise): bounded system-git wrapper`.

---

### Task 7: Indexer (`indexer.go`)

**Files:**
- Create: `go/internal/expertise/indexer.go`
- Test: `go/internal/expertise/indexer_test.go`

- [ ] **Step 1: Write failing integration test**

Using the Task 6 fixture repo + in-memory store: construct `Indexer` with fixture identity (`alice`), run `Index(ctx, repoID, repoPath)`, then assert:
- `file_expertise` has rows for both files; alice is original author of `main.go`; bob has surviving lines in `main.go`.
- scores in (0,100]; alice dormant/bob dormant per fixture dates (pass a fixed `now` via the Indexer's `Now func() time.Time` field for determinism).
- repo state ends `ready` with `indexed_sha` = fixture HEAD, `files_total` == indexed count.
- a `.gitattributes`-marked generated file and a `vendor/` file produce NO rows.
- cancelling the context mid-run leaves status `indexing` (restartable) — use a store hook or tiny file count; acceptable to assert post-cancel status only.

- [ ] **Step 2: Verify fail.**

- [ ] **Step 3: Implement**

```go
type Indexer struct {
    log      *slog.Logger
    store    *Store
    git      *GitCLI
    identity func(ctx context.Context) Identity // resolves current user at run time
    emit     func(eventType string, payload any) // nil-safe broadcast hook
    Now      func() time.Time
    Workers  int // default 4; blame worker pool size
}
```

`Index(ctx, repoID, repoPath) error` flow: set status `indexing` → `HeadSHA` → `LsFiles` → glob-exclude (user globs from settings key `excludes:<repoID>`) → `GeneratedFiles` filter → two log passes (`LogNumstat` 12-month, active-author set 90-day) → per-file blame via a bounded worker pool (`errgroup` if already a dep, else channels) → per file: build rows (score.go math, identity resolution for `author_login` of the current user only — other authors' logins stay NULL in v1), `ReplaceFileRows` → every 25 files: `SetProgress` + emit `EXPERTISE_INDEX_PROGRESS` → terminal: status `ready` + emit `EXPERTISE_READY` (include `skipped_files` count of blame failures), or on error status `error` + emit `EXPERTISE_ERROR`. Context cancellation: return without changing status (stays `indexing` for boot restart).

Concurrency guard: one `Index` per repo at a time — keep a `map[string]context.CancelFunc` behind a mutex on an `Engine` façade (see Task 9) rather than in Indexer itself; Indexer stays a plain function object.

- [ ] **Step 4: Verify pass + race** — `go test -race ./internal/expertise/ -v`.

- [ ] **Step 5: Commit** — `feat(expertise): full-repo indexer`.

---

### Task 8: Updater (`updater.go`)

**Files:**
- Create: `go/internal/expertise/updater.go`
- Test: `go/internal/expertise/updater_test.go`

- [ ] **Step 1: Write failing test**

Fixture repo indexed (Task 7 helper). Then: add a new commit editing `main.go` (new HEAD). Call `Update(ctx, repoID, repoPath, []string{"main.go", "util.go", "missing.go"})`. Assert:
- `main.go` rows recomputed (`computed_at_sha` == new HEAD)
- `util.go` rows NOT recomputed if their `computed_at_sha` was fresh? — No: HEAD moved, so per spec both stale files touched by the PR re-blame; but `util.go` wasn't in the PR list... it IS in the list here. Assert both listed+existing files recompute; `missing.go` (not on disk) produces no rows and no error.
- Repo not `ready` → `Update` is a no-op (assert on a fresh repoID).

- [ ] **Step 2: Verify fail.**

- [ ] **Step 3: Implement**

`Update`: bail unless state `ready`; `HeadSHA`; if HEAD == `indexed_sha`, still check per-file `computed_at_sha` (cheap skip); re-run the two log passes once per Update call when HEAD moved (then `SetRepoState` with new `indexed_sha`); re-blame only listed files that pass excludes and exist; `ReplaceFileRows` each. Errors per file: log + skip (never fail the sync).

- [ ] **Step 4: Verify pass + race.** **Step 5: Commit** — `feat(expertise): incremental updater`.

---

### Task 9: Engine façade + nudge (`engine.go`, `nudge.go`)

**Files:**
- Create: `go/internal/expertise/engine.go`, `go/internal/expertise/nudge.go`
- Test: `go/internal/expertise/nudge_test.go` (+ engine covered via handler/orchestrator tests)

- [ ] **Step 1: Write failing nudge test**

In-memory store with migrations; insert a `repos` row (`source='local'`), 10 `prs` rows + 10 `last_reviewed_sha` rows for login `alice` under account `('github','alice')`. `ShouldNudge(ctx, repoID, Account{Provider:"github", Login:"alice"})` → true. Then: mark state enabled → false; dismiss (`nudge_dismissed_at` set) → false; 9 PRs → false; repo `source='cache'` → false; rows written under a different account (`('github','bob')`) don't count toward alice's threshold.

- [ ] **Step 2: Verify fail.**

- [ ] **Step 3: Implement**

`nudge.go`: `ShouldNudge` runs `CountReviewedPRsForRepo` + repo-state + repos.source checks. Constant `NudgeThreshold = 10`.

`engine.go`: the wiring surface the rest of the app talks to:

```go
type Engine struct {
    log     *slog.Logger
    store   *Store
    git     *GitCLI
    indexer *Indexer
    updater *Updater
    mu      sync.Mutex
    running map[string]context.CancelFunc // repoID -> cancel
}
```

Methods: `Enable(ctx, repoID, repoPath) error` (validate git available + path exists; set enabled; spawn goroutine `Index` with its own cancellable context registered in `running`), `Disable(repoID)` (cancel if running, set enabled=0, keep rows), `OnPRSynced(ctx, repoID, repoPath, files []string)` (delegates to updater; called from orchestrator), `RestartInterrupted(ctx, resolvePath func(repoID) (string, bool))` (boot path: `InterruptedRepos` → re-Index each), `ShouldNudge`, `TouchesYou(ctx, repoID, prFiles []string, identity Identity, minLines int) (bool, error)` (top-3 non-dormant rule via `TopExpertsByFile`), `Shutdown()` (cancel all).

- [ ] **Step 4: Verify pass + race.** **Step 5: Commit** — `feat(expertise): engine façade + review nudge`.

---

### Task 10: Orchestrator wiring

**Files:**
- Modify: `go/internal/sync/orchestrator.go` (option + call site)
- Test: extend `go/internal/sync/orchestrator_test.go` or a new focused test file

- [ ] **Step 1: Write failing test**

Follow the `WithClassifier` precedent (read `orchestrator.go:135-138` and the writer wiring at `:1159`). Test: orchestrator with a fake expertise hook (interface, not the concrete Engine — define `type ExpertiseHook interface { OnPRSynced(ctx context.Context, repoID, repoPath string, files []string) }` in the sync package) records a call with the PR's repo and file list after a successful diff generate. Use the existing fetcher/orchestrator test scaffolding with the mock provider.

- [ ] **Step 2: Verify fail.**

- [ ] **Step 3: Implement**

`WithExpertise(h ExpertiseHook) *Orchestrator` storing the hook. Call site: in `fetcher.go`, after diff generation succeeds (near where the diff store is written, `fetcher.go:256` area), if hook non-nil and the PR's repo has a known local path — resolve via the repo-location store the same way rangediff wiring gets paths (pass a `resolveRepoPath func(repoID) (string, bool)` into the orchestrator options alongside the hook; app.go supplies it from `RepoLocationStore`). Extract file paths from the diff result manifest. Fire-and-forget with the task context; errors only logged inside the engine.

- [ ] **Step 4: Verify** — `go test -race ./internal/sync/` PASS. **Step 5: Commit** — `feat(expertise): sync post-diff hook`.

---

### Task 11: WS handlers + app wiring + PR-list `touches_you`

**Files:**
- Create: `go/internal/server/handler/expertise.go`, `go/internal/server/handler/expertise_test.go`
- Modify: `go/internal/server/handler/pr.go` (touches_you), `go/internal/server/app.go` (construction + registration + boot restart)

- [ ] **Step 1: Write failing handler tests**

Mirror `handler/classification_settings_test.go` / `handler/patchsets_test.go` style. Message names and payloads are normative in spec §WS API. Handlers (each a `ws.MessageHandler` built by a `NewXxxHandler(deps)`):

| Message | Behavior tested |
|---|---|
| `EXPERTISE_ENABLE` | repo `source='local'` + git available → state enabled + indexing kicked (assert via fake engine); cache-only repo → `EXPERTISE_ERROR{code:"repo_not_local"}`; git absent → `code:"git_unavailable"` |
| `EXPERTISE_DISABLE` | cancels + disables; rows kept |
| `GET_EXPERTISE` | `{pr_id}` → `EXPERTISE_DATA` with per-file top experts (raw components) + current-user rank; PR in un-indexed repo → `{status:"unavailable", reason}` |
| `EXPERTISE_NUDGE_DISMISS` | sets `nudge_dismissed_at` |
| `GET_EXPERTISE_SETTINGS` / `SET_EXPERTISE_SETTINGS` | round-trips `alias_emails []string`, `min_surviving_lines int`, `excludes map[repoID][]string`; validates min_lines > 0 |

Define handler deps as small interfaces (existing pattern: `rangeDiffRunner`, `lastReviewedReader`) so tests use fakes, not the real Engine.

- [ ] **Step 2: Verify fail.** **Step 3: Implement handlers.**

- [ ] **Step 4: `touches_you` on the PR list**

In `handler/pr.go`: add `TouchesYou bool \`json:"touches_you"\`` to `prRow`. Populate via a `touchesYouResolver` interface (`TouchesYouForPRs(ctx, prIDs []string) (map[string]bool, error)`) implemented on the Engine using one batched query joining **`pr_diffs`** (the live per-file diff metadata written by the sync writer — the legacy `files` table has no current writer, do NOT join it) against `file_expertise` top-3 — implement that query in `queries/expertise.sql` + regenerate (`sqlc generate`) rather than N per-PR lookups. Read `queries/diff_store.sql` first for the `pr_diffs` per-file column names (`pr_id`, `file_path`). The resolver must apply the spec's normative rule, not just top-3-by-score: filter to non-dormant rows passing the expert predicate (`surviving_lines >= min_surviving_lines OR score >= ExpertScoreFloor` — reuse `IsExpert` from score.go), rank by score per file, and return true only when the current `Identity` (Task 4) matches one of the top-3 qualifying experts for any of the PR's `pr_diffs.file_path` rows. Nil resolver (expertise not constructed) → field stays false, zero cost. Add a handler/resolver test whose fixture writes `pr_diffs` rows, with three cases: current user in top 3 and qualifying → true; current user in top 3 but below both expert thresholds → false; another user qualifies but current user doesn't → false.

- [ ] **Step 5: Wire in `app.go`**

Near the other constructions (~line 480): build `GitCLI`, `Store`, `Indexer`, `Updater`, `Engine`; `RegisterHandler` for the five message types (pattern at `app.go:486+`); pass Engine into orchestrator via `WithExpertise`; emit hook = the same broadcast path the orchestrator's event channel uses (find where `SYNC_COMPLETE` events reach `wsServer` broadcast and reuse it — read `app.go` around the event pump); on startup after migrations, call `engine.RestartInterrupted` in a goroutine; call `engine.Shutdown()` on app shutdown. Nudge check: call `ShouldNudge` in the review-submission success path (where `last_reviewed_sha` is written — `WithReviewTracking`'s writer) and broadcast `EXPERTISE_NUDGE`.

- [ ] **Step 6: Full Go verify**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 7: Commit** — `feat(expertise): WS handlers, app wiring, touches_you on PR list`.

---

### Task 12: Renderer — types, events, filter, sidebar

**Files:**
- Modify: `electron/src/renderer/types/index.ts`, `electron/src/renderer/hooks/useWebSocket.ts`, `electron/src/renderer/components/FilterMenu.tsx`, `electron/src/renderer/components/StatusSidebar.tsx`
- Create: `electron/src/renderer/utils/expertise.ts`, `electron/src/renderer/utils/expertise.test.ts`

- [ ] **Step 1: Types**

Add `touches_you: boolean` to `PR` (`types/index.ts:4+`). Add payload types: `ExpertiseFileData` (`file_path`, `experts: {author_email, author_login, surviving_lines, total_lines, commit_score, is_original, score, dormant}[]`, `your_rank: number | null`), `ExpertiseRepoStatus`, nudge payload.

- [ ] **Step 2: Write failing Vitest for pure helpers**

`utils/expertise.test.ts` (pure functions only — NO component tests, per spec):
- `touchesYouCount(prs)` counts rows with `touches_you`
- `formatWhy(expert)` → `"alice: 312/480 surviving lines, original author → 87"` (omit "original author" when false; round score)

- [ ] **Step 3: Verify fail, implement `utils/expertise.ts`, verify pass.**

- [ ] **Step 4: Wire events**

`useWebSocket.ts`: add cases `EXPERTISE_INDEX_PROGRESS`, `EXPERTISE_READY`, `EXPERTISE_ERROR`, `EXPERTISE_NUDGE`, `EXPERTISE_DATA`, `EXPERTISE_SETTINGS` following the existing case style (~line 421+) — update the relevant TanStack store slices / query caches the same way `SCAN_DEV_FOLDER_RESULT` does.

- [ ] **Step 5: Filter + sidebar**

`FilterMenu.tsx`: add "Touches Your Code" filter entry (predicate: `pr.touches_you`). `StatusSidebar.tsx`: count via `touchesYouCount` next to triage lanes; show per-repo indexing status line when a repo is `indexing` (`1,204/3,872`) or `error`.

- [ ] **Step 6: Verify** — `cd electron && pnpm type-check && pnpm lint && pnpm test` PASS.

- [ ] **Step 7: Commit** — `feat(expertise): renderer filter, sidebar count, event wiring`.

---

### Task 13: Renderer — detail view, nudge banner, palette, settings

**Files:**
- Create: `electron/src/renderer/components/ExpertiseNudge.tsx`
- Modify: PR detail component (locate the file-list rendering in the detail pane — likely `PierreDiffView.tsx`'s file header area or the detail overview component; follow existing structure), `CommandPalette.tsx`, the Settings page component (find via `grep -rn "Usage Telemetry\|Classification" electron/src/renderer/` to locate the settings sections pattern)

- [ ] **Step 1: PR detail "why" view**

On PR selection, send `GET_EXPERTISE` (new hook `useExpertise(prId)` following `usePatchsets.ts` shape). Render a per-file expertise line under the file header: top experts by score with `formatWhy` in an expandable section. Unavailable → render nothing (no noise) unless repo is `error`, then a one-line hint.

- [ ] **Step 2: Nudge banner**

`ExpertiseNudge.tsx`: renders when a nudge event for the current repo is in store — text "You've reviewed N PRs from this repo. Enable expertise indexing?", buttons Enable (`EXPERTISE_ENABLE`) / Dismiss (`EXPERTISE_NUDGE_DISMISS`). Place it where other banners/feedback live (`ActionFeedback.tsx` placement precedent). No component test.

- [ ] **Step 3: Palette commands**

`CommandPalette.tsx` (~line 27+ command list): `{ id: 'expertise-enable', name: 'Enable expertise', icon: 'psychology' }` and `expertise-disable`. Action: repo of the focused/selected PR; if none selected, open the repo picker first (reuse `repo-picker` command's flow). Send the WS message.

- [ ] **Step 4: Settings section**

"Expertise" section following the classification settings section pattern: alias emails (comma/list input), min surviving lines (number, default 50), per-repo exclude globs (textarea per watched repo or a simple `repo: globs` list — match existing settings form idioms). Load via `GET_EXPERTISE_SETTINGS`, save via `SET_EXPERTISE_SETTINGS`.

- [ ] **Step 5: Verify** — `pnpm type-check && pnpm lint && pnpm test` PASS.

- [ ] **Step 6: Commit** — `feat(expertise): detail why view, nudge, palette, settings`.

---

### Task 14: Docs + final verification

**Files:**
- Modify: `docs/ROADMAP.md` (mark 4.1/4.2/4.3 done), `docs/go-project-overview.md` (add `internal/expertise` to the package map)
- Create: `website/src/pages/docs/expertise.mdx` (user guide: what it does, opt-in, how scores work, settings; follow `comment-classification.mdx` structure)

- [ ] **Step 1: Write the user guide + update roadmap/package map.**

- [ ] **Step 2: Full verification suite**

```bash
cd go && go vet ./... && go test -race ./...
cd ../electron && pnpm type-check && pnpm lint && pnpm test
```
Expected: all PASS.

- [ ] **Step 3: Manual smoke (documented, not automated)**

Run the app against a real local repo: `enable expertise` from palette → watch progress → confirm "Touches Your Code" populates and the why view shows sane numbers. Record results in the PR description.

- [ ] **Step 4: Commit** — `docs(expertise): user guide + roadmap status`.

---

## Execution notes

- Tasks 3–5 (score, identity, excludes) are independent of each other and of Task 1–2; Tasks 6–9 depend on 2–5; Task 10 depends on 9; Task 11 on 9–10; Tasks 12–13 on 11.
- Commit after every task minimum; smaller commits within tasks are fine.
- If `sqlc generate` output drifts from what a test expects, fix the query SQL, never the generated Go.
- If a step's stated file location doesn't match reality (the codebase moves fast), grep for the named symbol and follow the existing pattern — the pattern reference is normative, the line number is advisory.
