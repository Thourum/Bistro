# Phase 2 Re-scope — Patchset Semantics (Decomposition)

> **⚠️ This is a DECOMPOSITION / TRACKING doc — NOT a buildable spec.** It defines
> how the re-scoped Phase 2 breaks into independent slices and the assumptions behind
> each. Every slice below gets its **own** brainstorm → spec → plan cycle (one spec +
> one plan per slice). Do not implement from this document; implement from the
> per-slice spec/plan it spawns. Track those in the checklist directly below.
>
> **Supersedes** the original Phase 2 in `docs/ROADMAP.md` (2.1–2.4). Written
> 2026-06-20 after the multi-agent audit (`docs/ROADMAP-AUDIT-2026-06-19.md`)
> found the original phase rested on a foundation that didn't exist and partly
> couldn't exist as specified. This document is the authoritative *shape*; the
> roadmap's Phase 2 prose is left for history with `⚠️ Audit` annotations.
>
> **Stage 2 (Team) deferrals referenced here:** `docs/STAGE-2-TEAM.md`.

## Slice tracking

Each slice runs: ponytail-audit (assumptions) → brainstorm → spec → plan → issue/PR → implement.

| Slice | Spec | Plan | Issue/PR | Status |
|---|---|---|---|---|
| Groundwork | ✅ `specs/2026-06-20-phase-2-groundwork-design.md` | ⏳ next | — | spec written + reviewed 2026-06-20; migration **012** |
| 2.1 + 2.2 Patchset timeline + since-last-review | ✅ `specs/2026-06-20-phase-2-1-2-2-patchset-timeline-design.md` | ✅ `plans/2026-06-20-phase-2-1-2-2-patchset-timeline.md` | — | spec+plan written 2026-06-20; migration **014**; blocked on Groundwork |
| 2.3 Comment anchoring across rebase | ✅ `specs/2026-06-20-phase-2-3-comment-anchoring-design.md` | ✅ `plans/2026-06-20-phase-2-3-comment-anchoring.md` | — | spec+plan written 2026-06-20; migration **015**; blocked on Groundwork |
| 2.4 Unresolved-threads view + resolve/unresolve write-back + resolved/reopened event | ✅ `specs/2026-06-20-phase-2-4-unresolved-threads-design.md` | ✅ `plans/2026-06-20-phase-2-4-unresolved-threads.md` | — | spec+plan written 2026-06-20; **no migration** (uses Groundwork's `review_threads`); blocked on Groundwork. Shrunk: thread *data layer* moved into Groundwork; 2.4 is now UI + write-back only |

> **Migration numbering:** 012 = Phase 2 Groundwork · 013 = Phase 3 classification ·
> 014 = Phase 2.1+2.2 · 015 = Phase 2.3 · Phase 2.4 needs none.
>
> **Open questions surfaced during spec/plan authoring (resolve before/while implementing each slice):**
> - **2.1+2.2:** range-diff wrapper signature — ship the "two-SHA simplified" form (adequate for P1→P2 pair diffs) or the full five-arg commit-range form from day one? Spec defaults to simplified.
> - **2.3:** Task 7 (stamping `patchset_number` in `writer.go`) assumes Groundwork's `WriteReviewComments` + patchset upsert exist; re-read against the Groundwork plan before starting 2.3.
> - **2.4:** the `useReviewThreads` fetch depends on Groundwork's exact server interface (REST endpoint vs WS request) — Groundwork spec references a handler but doesn't lock the shape; fill in 2.4's Task 13 once Groundwork ships.
>
> **Each slice's spec should still pass its own spec-review + your review before implementation** (only Groundwork's spec has been through the reviewer so far).

> Update this table as each slice's spec/plan/issue is created. Build order if serial:
> **Groundwork → 2.3 → 2.1+2.2 → 2.4** (see "Slice structure" below for rationale).

## Why this re-scope exists

The audit confirmed, against the codebase:

- **Patchset capture is lossy under polling.** A local-first poller (GitHub API or
  local `go-git` fetch) only observes repo state at sync time; force-pushes
  superseded between syncs are unrecoverable without a server receiving webhooks.
- **`git range-diff` is not in go-git v5.17.2.** Only `Tree.Patch` (plain
  tree-to-tree). No library provides range-diff for binding — libgit2/git2go,
  gitoxide, git2-rs, GitButler's `grit` all lack it; GitButler's own "interdiff"
  is a closed-source server computation.
- **Inline review comments are never fetched.** The provider calls
  `Issues.ListComments` (issue-level only), never `PullRequests.ListComments`
  (inline diff comments). The comments 2.3/2.4 operate on don't exist in the cache.
- **The schema stores no anchoring or resolution data** — no `commit_sha`,
  `position`, `side`, `thread_id`, `patchset_number`, or resolution state.
- **No current-user identity in SQLite** — the login lives only in an in-memory
  JSON cache, so user-scoped queries (2.2, Phase 4) are brittle.
- **Thread resolution is GitHub-GraphQL-only** — the entire provider stack is REST.

## Three honesty corrections

1. **"Observed patchsets," not "every push."** A patchset = a distinct head SHA
   Bistro observed at sync time, numbered in observation order. The `pushed_at`
   column **keeps its name** (Stage 2's server backfills it with true push times —
   identical schema). FOSS is honest that rapid force-push bursts can collapse;
   per-open-PR fast-sync (roadmap 1.4) makes this rare for actively-reviewed PRs.
2. **Range-diff via the system `git` binary, with graceful degradation.** FOSS
   shells out to `git range-diff` when present; if git is absent or `<2.19`,
   rebase-aware diffs degrade to plain head-to-head `Tree.Patch` with a small footer
   status indicator. A deliberate, scoped exception to the "pure go-git, no
   `os/exec`" principle, isolated in one wrapper. The pure-Go range-diff engine is
   deferred to Stage 2 (S2.2).
3. **GraphQL capability added (no server needed).** A thin GitHub GraphQL client
   (`shurcooL/githubv4`) behind an optional capability interface unlocks thread
   resolution state + resolve/unresolve mutations. This is **not** a Stage-2 item —
   it needs no infrastructure, just GitHub's other API, callable from the local
   process with the token already in hand.

## Slice structure

Each slice is its own spec → plan → implement cycle (matching Phase 3's approach).

```
Groundwork  (hard prerequisite for all features)
      │
      ├── 2.1+2.2  Patchset timeline + since-last-review   (MERGED — shared machinery)
      ├── 2.3      Comment anchoring across rebase          (pure-local)
      └── 2.4      Unresolved-threads view                  (gu)
            ├── 2.4.1  resolve/unresolve write-back   (was Phase 1.3)
            └── 2.4.2  conversation resolved/reopened event (was Phase 1.5)
```

**Recommended build order (if serial):** Groundwork → 2.3 → 2.1+2.2 → 2.4. Rationale: 2.3
is pure-local and validates the inline-comment data model with the least new
machinery; 2.1+2.2 adds the range-diff dependency; 2.4 adds the GraphQL write path
(highest risk, benefits from the foundation being shaken out first). The three
feature slices may run in parallel if there's capacity.

---

## Phase 2 Groundwork

No feature UI. Data + ingestion + capability plumbing, built and tested first.

### 1. Inline review comments (REST)

- Add `PullRequests.ListComments` to the GitHub provider + a normalizer mapping
  `PullRequestComment` fields: `CommitID`, `OriginalCommitID`, `Path`, `Line`,
  `OriginalLine`, `Side`, `Position`, `OriginalPosition`.
- The fetcher/orchestrator gains the call; the writer gains an inline-comment write
  path. go-github v84 already exposes this — pure REST, no new dependency.

### 2. Unified `comments` table + schema migrations

**Decision: one unified `comments` table with a `kind` discriminator
(`'issue' | 'review'`)** — NOT a separate `review_comments` table. Rationale: Phase
3 classification (`classify-on-write` hook, 5 classification columns,
`comment_classification_state` keyed on `comment_id`) must apply to inline comments
too (bot findings are mostly inline). A unified table makes the classification hook,
columns, state table, and the renderer's severity-sorted query compose with zero
special-casing. A separate table would force duplicated columns/hooks and a `UNION`
in the renderer. Nullable inline-only columns on issue rows are cosmetic; SQLite
doesn't care.

New columns on `comments` (additive migration, version after Phase 3's 012):
- `kind TEXT NOT NULL DEFAULT 'issue'` — `'issue' | 'review'`
- `commit_sha TEXT`, `original_commit_sha TEXT`
- (no new `path` column) — **reuse the existing `file_path` column uniformly across
  both `kind='issue'` and `kind='review'` rows.** Do NOT add a separate `path`
  column: a second name for the same conceptual field would fork the 2.3 anchoring
  query and Phase 3's classification renderer across kinds. `file_path` already
  carries the right semantics.
- `position INTEGER`, `original_position INTEGER`
- `side TEXT` — `'LEFT' | 'RIGHT'`
- `thread_id TEXT`
- `patchset_number INTEGER` — which observed patchset the comment was made against
  (nullable; backfilled NULL for historical rows). Lets 2.3 detect "outdated."

`patchsets` table (corrected DDL — original roadmap used `pr_id INTEGER`, a
type mismatch against `prs.id TEXT`):

```sql
CREATE TABLE patchsets (
  pr_id      TEXT NOT NULL,
  number     INTEGER NOT NULL,
  head_sha   TEXT NOT NULL,
  base_sha   TEXT NOT NULL,
  pushed_at  TIMESTAMP NOT NULL,  -- best-available: observation time (FOSS), true push time (Stage 2)
  PRIMARY KEY (pr_id, number),
  FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);
```

`current_user` table:

```sql
CREATE TABLE current_user (
  provider TEXT PRIMARY KEY,
  login    TEXT NOT NULL
);
```

### 3. Observed-patchset capture (writer hook)

On each sync, compare the PR's incoming `head_sha` against the latest stored
patchset for that PR. If changed (or none exists), insert `number = max+1`,
`head_sha`, `base_sha`, `pushed_at = <observed time>`. Idempotent on
`(pr_id, head_sha)` — a known SHA is a no-op. A patchset-write error logs and
continues; never aborts the sync tx (matches existing writer per-item discipline).

### 4. Current-user identity (auth hook)

At auth-success, persist `(provider, login)` to `current_user`, sourced from
`GetUsername`. User-scoped queries (2.2, Phase 4) key on `login TEXT`. Eliminates
the cold-cache-silent-wrong-result hazard the audit flagged.

### 5. GraphQL capability (`shurcooL/githubv4`)

- Add `shurcooL/githubv4` (composes with `golang.org/x/oauth2`, already in
  `go.mod` — token auth reuses the existing OAuth http client).
- Expose via an **optional capability interface** (e.g. `ThreadResolver`)
  discovered by type assertion at call sites. The core `Provider` stays REST and
  provider-agnostic; providers that don't implement it degrade gracefully (no forced
  stub). GitLab's native thread resolution slots into the same interface later.
- Surfaces: read `isResolved` / `isOutdated` per thread; `resolveReviewThread` /
  `unresolveReviewThread` mutations.

### Foundation acceptance

- Inline comments fetched + stored as `kind='review'` rows alongside issue comments.
- Patchset row recorded on head-SHA change; no-op on unchanged.
- `current_user` populated at login and queryable offline.
- GraphQL capability reads thread state and resolves/unresolves a thread.
- Mock provider updated with inline-comment + GraphQL fakes; all behind tests.

---

## 2.1 + 2.2 — Patchset timeline + since-last-review (merged slice)

Merged because they share the entire patchset + range-diff machinery.

- **Timeline UI** in PR detail: P1/P2/P3 (observed patchsets) with a diff-pair
  selector. Labeled honestly: "snapshots observed since you started watching."
- **Pair diff** shells out to system `git range-diff` against the local go-git
  clone. No git installed → graceful fallback to plain `Tree.Patch` head-to-head +
  footer status indicator.
- **Since-last-review:** record `last_reviewed_sha` per `(pr_id, login)` (keyed on
  `current_user.login`) on every review submission — needs a small
  `last_reviewed_sha` table. Toggle renders `git range-diff last_reviewed_sha..head`
  (same wrapper + fallback).
- **Files-viewed survives rebase:** `files_viewed` table keyed on
  `(pr_id, file_path, login, content_hash)` so "viewed" persists when content is
  unchanged across a rebase. Internal ordering note for the slice spec:
  `content_hash` must be tied to a patchset row, so files-viewed depends on the
  `patchsets` table — build it after patchset capture, not in parallel.
- **Depends on:** patchsets, current_user, range-diff wrapper.

---

## 2.3 — Comment anchoring across rebase (pure-local)

- Anchor inline comments (now stored with `commit_sha` / `position` /
  `patchset_number`) to `(file_path, line_content_hash, surrounding_context)` in
  addition to line number.
- After rebase, re-resolve to new line numbers if content survived; if deleted, show
  "outdated" inline at the nearest surviving line (not banished to a sidebar).
- Pure-local computation over the go-git clone — no API round-trip for display. A
  missing/failed clone degrades to line-number-only display.
- **Depends on:** inline comments, `patchset_number`.

---

## 2.4 — Canonical unresolved-threads view (`gu`)

- Keystroke `gu` lists every thread in the PR with explicit lifecycle
  (Open / Resolved / Outdated), state read via the GraphQL capability
  (`isResolved` / `isOutdated`).
- Find-in-page across all threads (including collapsed); merge-readiness check
  surfaces every open thread.
- **Depends on:** inline comments, GraphQL capability.

### 2.4.1 — Resolve/unresolve write-back (was Phase 1.3)

Resolve/unresolve threads in-app via the GraphQL `resolveReviewThread` /
`unresolveReviewThread` mutations. Folded here because it shares the GraphQL
capability. Mutations route through the existing offline action queue (which now
dedups correctly after the 2026-06-19 fix).

### 2.4.2 — Conversation resolved/reopened event (was Phase 1.5)

Emit a WebSocket event when a thread's resolution state changes, so the UI updates
without waiting for a full sync cycle. Shares the before/after-on-write pattern that
the broader CI/metadata-changed events (Phase 1.5) will also need.

---

## What is explicitly NOT in this re-scope

**→ Stage 2 (`docs/STAGE-2-TEAM.md`):**
- **S2.1** Complete server-based push capture (every force-push, true `pushed_at`).
  FOSS gets observed patchsets only.
- **S2.2** Pure-Go range-diff engine (~650 LOC). FOSS shells out to system git or
  degrades.

**Deferred / unchanged elsewhere:**
- Phase 1.3 idempotency-key scheme stays a later enhancement (the acute dup-enqueue
  bug is already fixed, 2026-06-19).
- Reviewer-request write-back (Phase 1.3) stays in Phase 1 — independent of patchset
  semantics, not pulled into Phase 2.

---

## Cross-phase interaction: Phase 3 classification

Phase 3 classification operates on the **unified `comments` table**. Its wedge value
(triaging the 1 critical among 47 bot comments) depends on Groundwork's inline-comment
fetch landing — bot findings (CodeRabbit, Greptile, etc.) are mostly inline review
comments, not issue-level. **The Phase 3 foundation spec
(`2026-06-19-comment-classification-foundation-design.md`) must be amended** with a
one-paragraph note: classification applies to the unified table, and full coverage of
bot findings requires Groundwork's inline fetch. No code conflict — both are additive
migrations with distinct version numbers (Groundwork = 012; Phase 3 = 013).

**Sequencing consequence (state plainly so no planner assumes otherwise):** Groundwork
(012) ships **first**; Phase 3 classification (013) builds on top of it. Full bot
coverage (the inline CodeRabbit/Greptile findings that are the wedge's whole point)
is available from day one of Phase 3 because Groundwork's inline fetch already landed.
This is the planned, correct ordering — Phase 3 should not ship before Groundwork.

---

## Error handling

- **No system git:** one-time capability probe at startup (`git --version`); absent
  or `<2.19` → range-diff features fall back to `Tree.Patch` head-to-head + footer
  status icon. Probe result cached, never re-run per diff. Never crashes or hangs.
- **`os/exec` discipline:** the git shell-out is the *only* sanctioned `os/exec`,
  isolated in one wrapper (e.g. `git/rangediff.go`), args fully controlled — SHAs
  validated hex, paths from our own clone, no user-string interpolation. No shell
  injection surface.
- **GraphQL failures:** thread-state reads degrade to "resolution unknown" rather
  than blocking the threads view; mutations surface a typed error and route through
  the offline action queue. Go-side idiomatic errors; TS-side neverthrow.
- **Patchset capture:** idempotent on `(pr_id, head_sha)`; write errors log and
  continue, never abort the sync tx.
- **Anchoring:** content-no-longer-exists resolves to "outdated" (not an error);
  missing clone degrades to line-number-only display.

## Testing

- **Foundation:** mock provider gains inline-comment + GraphQL fakes; migration
  tests assert unified `comments` schema + `patchsets` + `current_user`; capture-hook
  test asserts a new patchset row on head-SHA change and no-op on unchanged.
- **Range-diff wrapper:** tested with system git present (real `git range-diff` on a
  fixture repo) AND a forced-absent path asserting graceful fallback. Skip-if-no-git
  guards keep CI green on minimal images; the fallback path is always tested.
- **2.3 anchoring:** table-driven over a fixture repo — content survived / moved /
  deleted. Pure-Go, deterministic, no network.
- **2.4:** GraphQL capability tested against recorded fixtures; resolve/unresolve
  tested through the offline queue.
- Race detector on the capture hook and any new concurrent paths.

## The honesty contract (UI + docs)

- Patchsets labeled "observed since you started watching" — never "every push."
- Range-diff degraded (no-git) mode shown as a small footer status icon/notification
  (reusing the existing `AppFooter` slot system) — not an intrusive per-diff banner.
- Thread state shows "unknown" rather than guessing when GraphQL is unavailable.
