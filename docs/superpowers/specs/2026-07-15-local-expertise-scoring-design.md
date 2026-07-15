# Local Expertise Scoring (Phase 4) — Design

**Date:** 2026-07-15
**Status:** Approved (user-reviewed design conversation)
**Roadmap:** Phase 4 — Local Expertise Scoring (FOSS layer)

## Summary

Bistro computes per-file expertise scores from the git history of the user's
**local checkout** of a watched repo, and uses them to surface PRs that touch
files the current user has meaningful history with ("Touches Your Code").
Scoring is **opt-in per repo**: the user enables indexing explicitly (palette
command or a nudge after reviewing 10 PRs from the same local repo). A full-repo
index is built once on enable; subsequent PR syncs update incrementally, only
for the files each PR touches.

All computed data lives in SQLite. There is **no `.bistro/expertise.yml`** —
that idea is dropped permanently (not deferred): the Team plan will use an
AST + vector-store approach, so a repo-committed config file has no future.
Per-repo excludes and global knobs live in SQLite, edited via Settings.

## Scope

**In (FOSS v1):**

- Signals 1–3 of the roadmap algorithm:
  1. **Surviving lines** — `git blame HEAD` per file, aggregated by author
     (high weight, no decay).
  2. **Substantial commits in last 12 months** — >5 lines changed, recency-
     decayed with a 6-month half-life (medium weight).
  3. **Original authorship** — who created the file, via
     `git log --diff-filter=A` (medium weight, captured as a boolean, weighted
     into the score).
- 90-day dormancy cutoff (per this repo — see Decisions).
- "Touches Your Code" PR-list filter + sidebar count.
- Per-file "why" view with raw signal components.
- Palette commands `enable expertise` / `disable expertise`.
- Enable-nudge after 10 reviewed PRs from the same local, un-indexed repo.
- Settings: manual alias emails ("also count these as me"), global
  min-surviving-lines threshold (default 50), per-repo exclude globs.

**Out (deferred to Team plan, roadmap updated accordingly):**

- **Signal 4** (prior reviewer of this file) — synced review data is sparse and
  biased toward recently-synced PRs; Team computes it server-side with
  complete data.
- **Pinned owners** — team semantics with no team in a single-user app.
- **Any repo-committed config file** — dropped permanently; Team uses
  AST + vector store.
- Cross-machine expertise graph, departed-employee detection beyond the 90-day
  heuristic (already listed out-of-scope in the roadmap).

## Decisions (from the design conversation)

| # | Decision | Choice |
|---|---|---|
| 1 | History source | **Local repos only** (`repos.source='local'`). Cache clones are base/head-only by design; expertise on a cache-only repo is a labeled unavailable state. No clone deepening. |
| 2 | Signal 4 | **Deferred to Team.** v1 ships signals 1–3. |
| 3 | Git engine | **System git via bounded `os/exec`** (extends the BIS-31 range-diff exception). No go-git fallback: git absent → labeled `git_unavailable` degradation. |
| 4 | Compute strategy | **Full-repo index as base, then incremental**: on enable, index every (non-excluded) file; on PR sync, re-score only files the PR touches whose freshness key is stale. |
| 5 | Opt-in | **Per-repo enable flag**, default off. Nudge after 10 reviewed PRs from the same local repo. |
| 6 | Identity matching | **Auto + manual aliases**: noreply-email pattern, author-name == login, authenticated account's API email, plus user-entered alias emails in Settings. |
| 7 | Config storage | **SQLite only.** No YAML file, ever (Team goes AST + vector store). Built-in exclude defaults + `.gitattributes linguist-generated=true` cover the common case with zero config. |
| 8 | Dormancy scope | Roadmap says "any commit or review in the watched repos within 90 days". FOSS v1 computes dormancy **per repo** (commit activity in *this* repo's history) — cross-repo activity aggregation and review activity are not honestly computable from a single local checkout; noted as a deliberate narrowing. |

## Architecture

New package `go/internal/expertise/` — all scoring logic contained here,
mirroring the shape of `internal/classify` (Phase 3 precedent). Sync
integration via an orchestrator option (`WithExpertise`, like
`WithClassifier`); WS surface via a new `internal/server/handler/expertise.go`.

```
enable (palette/nudge) ──► handler ──► expertise.Indexer (background goroutine)
                                          │  full-repo pass, progress events
                                          ▼
                                    file_expertise (SQLite)
                                          ▲
sync fan-out (post-diff) ──► expertise.Updater — re-scores PR-touched stale files
                                          │
PR list / detail ◄── touches_you flag ◄───┘
                     GET_EXPERTISE (why view)
```

### Package layout

| File | Responsibility |
|---|---|
| `identity.go` | Resolve "me": noreply pattern (`\d+\+login@users.noreply.github.com`), author-name == login, account API email, manual alias emails. Produces the set of emails/names that count as the current user. |
| `gitcli.go` | Bounded system-git wrapper (pattern: `git/rangediff.go`). `Available()`, `LsFiles`, `BlameFile` (`--line-porcelain`), `LogNumstat` (single repo-wide pass, 12-month window; yields per-author-per-file substantial-commit events + last-commit-at), `LogFirstCommits` (single repo-wide `--diff-filter=A` pass; yields file → original author). All read-only; context-cancellable; timeouts per command. |
| `score.go` | Pure math. `score = 55·(surviving/total) + 30·commit_score + 15·is_original`, clamped 0–100. `commit_score` = normalized sum of per-commit weights `0.5^(age_months/6)`, "substantial" = >5 changed lines. Dormant = no commit in this repo within 90 days. All weights/thresholds are named constants. |
| `excludes.go` | Built-in defaults (`vendor/**`, `dist/**`, `node_modules/**`, lockfiles, `**/*.pb.go`, `**/testdata/**`), `.gitattributes linguist-generated=true` (via `git check-attr`), per-repo user globs from settings. |
| `indexer.go` | Full-repo pass on enable: `ls-files` → filter excludes → two repo-wide log passes → per-file blame via bounded worker pool → batched writes → set `indexed_sha`, status `ready`. Progress row updated every N files + `EXPERTISE_INDEX_PROGRESS` events. One goroutine per repo; cancelled by disable or shutdown. |
| `updater.go` | Incremental, called post-diff in the sync fan-out: skip unless repo status `ready`; if local HEAD ≠ `indexed_sha`, re-run the two cheap log passes once; re-blame only PR-touched files whose `computed_at_sha` ≠ local HEAD. PR files absent from the local checkout simply have no rows. |
| `nudge.go` | Zero new counters: `COUNT(DISTINCT pr_id)` from `last_reviewed_sha` joined to `prs` for the repo. Checked on review submission; fires `EXPERTISE_NUDGE` when count ≥ 10 ∧ repo local ∧ not enabled ∧ nudge not dismissed. |
| `store.go` | SQLite access for the three tables below (owned here, not in `internal/store`, to keep the feature contained; uses the shared `*sql.DB`). |

### Data model (migration 030)

```sql
CREATE TABLE file_expertise (
  repo_id         TEXT NOT NULL,            -- 'provider:owner/repo'
  file_path       TEXT NOT NULL,
  author_email    TEXT NOT NULL,
  author_login    TEXT,                     -- resolved when matchable, else NULL
  surviving_lines INTEGER NOT NULL,         -- signal 1 raw
  total_lines     INTEGER NOT NULL,
  commit_score    REAL NOT NULL,            -- signal 2, decayed + normalized
  is_original     INTEGER NOT NULL,         -- signal 3
  score           REAL NOT NULL,            -- weighted sum, 0–100
  last_commit_at  TIMESTAMP,
  dormant         INTEGER NOT NULL DEFAULT 0,
  computed_at     TIMESTAMP NOT NULL,
  computed_at_sha TEXT NOT NULL,            -- local HEAD at compute (freshness key)
  PRIMARY KEY (repo_id, file_path, author_email)
);
CREATE INDEX idx_file_expertise_repo_path ON file_expertise(repo_id, file_path);

CREATE TABLE expertise_repo_state (
  repo_id            TEXT PRIMARY KEY,
  enabled            INTEGER NOT NULL DEFAULT 0,
  status             TEXT NOT NULL DEFAULT 'idle',  -- idle|indexing|ready|error
  indexed_sha        TEXT,
  indexed_at         TIMESTAMP,
  error_code         TEXT,                          -- git_unavailable|repo_not_local|...
  error              TEXT,
  files_total        INTEGER,
  files_done         INTEGER,
  nudge_dismissed_at TIMESTAMP
);

CREATE TABLE expertise_settings (
  key   TEXT PRIMARY KEY,   -- 'alias_emails' | 'min_surviving_lines' | 'excludes:<repo_id>'
  value TEXT NOT NULL       -- JSON-encoded
);
```

Raw components are stored so the "why" view is a read, never a recompute.

**Note on `repos` FK:** `file_expertise.repo_id` and
`expertise_repo_state.repo_id` reference `repos.id` values but carry no
`FOREIGN KEY` — `repos` rows can be created lazily by location scans, and the
roadmap's audit history (patchsets FK type mismatch) argues for keeping the
join key TEXT and index-backed rather than constraint-backed here.

### Scoring algorithm (normative)

For each `(file, author)` in a repo at local HEAD:

1. `surviving_ratio = surviving_lines / total_lines` (0 when file empty).
2. `commit_score = min(1, (Σ 0.5^(age_months/6)) / COMMITS_SATURATION)` over
   the author's substantial commits (>5 changed lines in that commit for that
   file) in the last 12 months, with `COMMITS_SATURATION = 4`. Worked example:
   4 substantial commits today ⇒ Σ = 4.0 ⇒ 4.0/4 = 1.0 (saturated); one
   substantial commit 6 months ago ⇒ Σ = 0.5 ⇒ 0.5/4 = 0.125.
3. `is_original ∈ {0,1}` from the first-commit pass.
4. `score = 55·surviving_ratio + 30·commit_score + 15·is_original`.
5. `dormant = 1` if the author has no commit (any file) in this repo in the
   last 90 days.

An author is an **expert** for a file when `surviving_lines ≥
min_surviving_lines` (global setting, default 50) **or** `score ≥ 40` (so a
prolific recent committer on a small file still counts; constant
`EXPERT_SCORE_FLOOR`).

**`touches_you`** (per PR): true iff the current user (per identity resolution)
is among the **top 3 non-dormant experts** by score for **any** file in the
PR's diff manifest.

### WS API

| Message | Direction | Payload |
|---|---|---|
| `EXPERTISE_ENABLE` | UI → Go | `{repo_id}`. Validates `source='local'` + git available; sets `enabled=1`, kicks off indexer. Errors respond `EXPERTISE_ERROR`. |
| `EXPERTISE_DISABLE` | UI → Go | `{repo_id}`. Cancels any running index, sets `enabled=0`, keeps rows (cheap re-enable). |
| `EXPERTISE_INDEX_PROGRESS` | Go → UI | `{repo_id, files_done, files_total}` |
| `EXPERTISE_READY` | Go → UI | `{repo_id, indexed_sha, files_total, skipped_files}` |
| `EXPERTISE_ERROR` | Go → UI | `{repo_id, code, message}` |
| `GET_EXPERTISE` | UI → Go | `{pr_id}` → `EXPERTISE_DATA`: per touched file, top experts with raw components (`surviving_lines/total_lines`, `commit_score`, `is_original`, `score`, `dormant`) + current user's rank. |
| `EXPERTISE_NUDGE` | Go → UI | `{repo_id, reviewed_count}` |
| `EXPERTISE_NUDGE_DISMISS` | UI → Go | `{repo_id}` → sets `nudge_dismissed_at`. |
| `EXPERTISE_SETTINGS_GET` / `EXPERTISE_SETTINGS_SET` | UI ↔ Go | alias emails, min-surviving-lines, per-repo excludes (classification-settings precedent). |

PR list rows gain a `touches_you` boolean computed Go-side at list-serve time,
so the list/lane needs no extra round trips.

### UI

- **"Touches Your Code (N)"**: new filter in `FilterMenu.tsx` + count in
  `StatusSidebar.tsx` beside the triage lanes. Rows show which files are
  "yours" (from `GET_EXPERTISE`, fetched on selection).
- **PR detail**: per-file expertise line with expandable "why" —
  `alice: 312/480 surviving lines, original author → 87`.
- **Indexing states**: progress/error surfaced in the repo picker row and
  status sidebar (`indexing 1,204/3,872`, `error: git unavailable`).
- **Nudge**: dismissable banner — "You've reviewed 10 PRs from this repo.
  Enable expertise indexing?" with Enable / Dismiss actions.
- **Palette**: `enable expertise` / `disable expertise` — act on the focused
  PR's repo; open the repo picker when no repo is in focus.
- **Settings**: "Expertise" section — alias emails (list input),
  min-surviving-lines (number), per-repo excludes (glob list per watched repo).
- Default behavior is **FYI in the local inbox only** — no external
  notifications, no provider write-back (roadmap 4.2 promise).

## Error handling & degradation

Every failure is a labeled state (range-diff precedent), never a silent gap:

| Condition | Behavior |
|---|---|
| System git absent | Enable refused → `EXPERTISE_ERROR{code:git_unavailable}`; settings/nudge explain. |
| Repo not local (`source≠'local'`) | Enable refused → `repo_not_local`; UI copy: "link a local checkout to enable expertise". |
| Local checkout deleted later | Existing `location_status` machinery flags the repo; updater skips; status set `error/repo_missing` on next touch. |
| Per-file blame failure (binary, >N MB, git error) | Skip file, log, count in `skipped_files`; indexing continues. |
| App quit mid-index | Status stays `indexing`; on next boot the engine restarts indexing for any repo `enabled=1 ∧ status='indexing'`. |
| Disable | Cancel goroutine, keep rows. No purge in v1. |
| User's repo checked out on a different branch / mid-rebase | Reads are against local HEAD, read-only; wrong-branch data self-heals on next incremental update when HEAD moves. No locks on the working tree. |

## Testing

- **Unit (pure):** scoring math, decay, saturation, expert threshold,
  identity resolution (noreply/login/alias), exclude matching. Table-driven.
- **Integration (`go test`, real git):** fixture repo built with `git`
  commands in `t.TempDir()` — two authors, a rename, a generated file, an
  old-vs-recent commit split. Enable → assert `file_expertise` rows, dormancy,
  excludes, `touches_you`. Commit again → incremental update re-scores only
  touched files (assert untouched `computed_at`s unchanged). Skipped when
  system git is unavailable (mirrors range-diff tests).
- **Handler tests:** follow `handler/*_test.go` patterns for every message.
- **Race:** `go test -race ./...` covers indexer/updater concurrency.
- **Renderer (Vitest):** filter/lane logic with `touches_you`, nudge
  component, settings form.

## Roadmap updates shipped with this spec

- 4.1: signal 4 marked **deferred to Team plan**; compute strategy corrected
  to "full index on explicit enable + incremental on sync" (was "on watch").
- 4.2: unchanged in substance; notes `touches_you` top-3 non-dormant rule.
- 4.3: rewritten — `.bistro/expertise.yml` **removed permanently** (Team uses
  AST + vector store); replaced by SQLite-backed settings + built-in excludes +
  `linguist-generated` respect.
