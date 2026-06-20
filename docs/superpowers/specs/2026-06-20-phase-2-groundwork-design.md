# Phase 2 Groundwork — Design

> **Note:** "Groundwork" is a synthesized prerequisite slice (shared schema + ingestion + capability layer that 2.1/2.2/2.3/2.4 all need), not a roadmap phase. The roadmap's Phase 2 is 2.1–2.4.

> First slice of the re-scoped Phase 2 (see
> `docs/superpowers/plans/2026-06-20-phase-2-decomposition.md`). Builds the data +
> ingestion + capability layer every Phase 2 feature needs. No feature UI here.
> Grounded in the 2026-06-19 audit (`docs/ROADMAP-AUDIT-2026-06-19.md`) and a
> code-anchor exploration verified against the tree on 2026-06-20.

## Goal

Stand up the prerequisites for 2.1+2.2 / 2.3 / 2.4, with nothing that needs a
server and a provider-agnostic model so GitLab/Gitea slot in later:

1. Fetch inline PR review comments (REST base).
2. Provider-agnostic `ReviewThread` model + GitHub GraphQL enrichment.
3. Unified `comments` table with a `kind` discriminator + anchoring columns.
4. Observed-patchset capture on head-SHA change.
5. Current-user identity persisted in SQLite at auth-success.

**Build-order note:** Phase 2 is being built before Phase 3, so this slice's
migration is **012**; Phase 3's classification migration moves to **013**. (Phase 2 Groundwork = 012 ships first; Phase 3 = 013 builds on it.)

## Settled decisions (this brainstorm)

- **ID namespacing (Q1=A):** issue comments keep `github-comment-<id>`; review
  comments get `github-review-comment-<id>`. `kind` column stored explicitly.
  Collision structurally impossible; `comment_classification_state.comment_id`
  (Phase 3) stays a single TEXT id.
- **Threads modeled in foundation (Q2=B), provider-agnostic:** a normalized
  `ReviewThread`; REST is the base (always works), GraphQL *enriches* grouping +
  resolution. **Consequence:** 2.4 shrinks to UI + resolve/unresolve write-back.
- **Two fetch methods (Q3=C):** `GetComments` stays for issue comments; new
  `GetReviewThreads` (capability interface) does REST+GraphQL stitching internally.
- **current_user write at auth-success (Q4=A):** narrow `CurrentUserStore`
  interface added to `AuthDeps`; no empty window.
- **Patchset capture in the Writer's tx (Q5=A1+B1):** atomic with the PR write;
  no real-time event yet.
- **GraphQL client:** `shurcooL/githubv4` (reuses existing `x/oauth2` client).
- **Anchor column:** reuse existing `comments.file_path` for both kinds; do NOT
  add a separate `path` column.

## Architecture / module layout

**Provider layer (`go/internal/provider/`):**
- `models.go` — add `ReviewThread` type; extend `Comment` with anchoring + reply
  + `Kind`/`ThreadID` fields.
- `interface.go` — add optional `ThreadProvider` capability interface (NOT on core
  `Provider`).
- `github/threads.go` (new) — `GetReviewThreads`: REST `PullRequests.ListComments`
  + GraphQL (`shurcooL/githubv4`) stitch, `normalizeReviewComment`.
- `github/graphql.go` (new) — thin githubv4 client built from the existing token
  (`oauth2.StaticTokenSource`, same pattern as `client.go`).
- `mock/` — implement `ThreadProvider` (and a second mock without it, for the
  graceful-skip test).

**Store (`go/internal/store/`):**
- `migrations/012_add_review_comments_and_patchsets.sql` (new).
- `current_user.go` (new) — `SetCurrentUser`/`GetCurrentUser`.
- patchset helper (`recordPatchsetIfChanged`) — in store or writer.

**Sync (`go/internal/sync/`):**
- `writer.go` — extend `writeComment` (new columns + `kind='issue'`); add
  `writeReviewThreads`; add observed-patchset capture in `writePR` (same tx).
- `orchestrator.go` — per-PR: capability-check + `GetReviewThreads`, pass results
  to the writer.

**Server (`go/internal/server/`):**
- `handler/auth.go` + `AuthDeps` — add `CurrentUserStore`; write current_user at
  auth-success alongside the existing cache write.
- `handler/comments.go` — **all new `comments` columns pass through as-is** in the
  existing flat comment payload (`kind`, `commit_sha`, `original_sha`, `side`,
  `position`, `original_position`, `in_reply_to`, `thread_id`, `patchset_number`).
  No new endpoint, no grouping — 2.3/2.4 consume this flat set and do their own
  shaping. (`review_threads` rows are served by a later slice's handler; foundation
  only guarantees the comment columns are exposed.)

**Renderer:** none this slice (payload shaping deferred to 2.3/2.4).

## Provider model

```go
// provider/models.go
type ReviewThread struct {
    ID         string       // provider-opaque thread id
    PRID       string
    FilePath   string
    Line       int          // 0 if outdated/unknown
    Resolved   bool
    Outdated   bool
    StateKnown bool         // false when resolution couldn't be determined
    Comments   []*Comment   // ordered; first is the root
}

// Comment gains:
//   Kind, CommitSHA, OriginalSHA, Side, Position, OriginalPosition,
//   InReplyTo, ThreadID
```

```go
// provider/interface.go — optional capability, discovered by type assertion
type ThreadProvider interface {
    GetReviewThreads(ctx context.Context, prID string) ([]*ReviewThread, error)
}
```

GitHub `GetReviewThreads`: REST fetches comments + `in_reply_to_id` + anchoring
(always works); GraphQL enriches thread grouping + `isResolved`/`isOutdated`. On
GraphQL failure → fall back to one-thread-per-root reconstructed from
`in_reply_to_id`, `StateKnown=false`, **no error returned**. A real REST failure is
a genuine error. The core `Provider` interface is unchanged.

## Schema — migration 012

```sql
-- Migration: 012_add_review_comments_and_patchsets
-- Description: Inline review comments (unified comments table), review threads, observed patchsets, current_user

-- review_threads MUST be created BEFORE the comments.thread_id FK column is added,
-- or the ALTER TABLE ... REFERENCES fails at apply time (foreign key mismatch).
CREATE TABLE review_threads (
    id          TEXT PRIMARY KEY,            -- provider-opaque thread id
    pr_id       TEXT NOT NULL,
    file_path   TEXT,
    line        INTEGER,
    resolved    INTEGER NOT NULL DEFAULT 0,
    outdated    INTEGER NOT NULL DEFAULT 0,
    state_known INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

ALTER TABLE comments ADD COLUMN kind TEXT NOT NULL DEFAULT 'issue';  -- 'issue' | 'review'
ALTER TABLE comments ADD COLUMN commit_sha TEXT;
ALTER TABLE comments ADD COLUMN original_sha TEXT;
ALTER TABLE comments ADD COLUMN side TEXT;             -- 'LEFT' | 'RIGHT'
ALTER TABLE comments ADD COLUMN position INTEGER;
ALTER TABLE comments ADD COLUMN original_position INTEGER;
ALTER TABLE comments ADD COLUMN in_reply_to TEXT;
ALTER TABLE comments ADD COLUMN thread_id TEXT REFERENCES review_threads(id) ON DELETE SET NULL;
ALTER TABLE comments ADD COLUMN patchset_number INTEGER;  -- filled by 2.3; NULL now

CREATE TABLE patchsets (
    pr_id      TEXT NOT NULL,
    number     INTEGER NOT NULL,
    head_sha   TEXT NOT NULL,
    base_sha   TEXT NOT NULL,
    pushed_at  TIMESTAMP NOT NULL,  -- best-available: observation time (FOSS), true push time (Stage 2)
    PRIMARY KEY (pr_id, number),
    UNIQUE (pr_id, head_sha),        -- DB-level idempotency: one row per observed head; enables INSERT OR IGNORE
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE TABLE current_user (
    provider TEXT PRIMARY KEY,
    login    TEXT NOT NULL
);

CREATE INDEX idx_comments_pr_kind ON comments(pr_id, kind);
CREATE INDEX idx_comments_thread ON comments(thread_id);
CREATE INDEX idx_review_threads_pr ON review_threads(pr_id);
CREATE INDEX idx_patchsets_pr ON patchsets(pr_id);
```

Notes:
- **`comments.thread_id` is an FK** to `review_threads(id)` `ON DELETE SET NULL`
  (orphaned comments survive as ungrouped review comments). Reuses existing
  `file_path` for the anchor; no `path` column. Existing rows get `kind='issue'`
  via the default — clean backfill.
- **Migration ordering vs. Phase 3:** Phase 2 Groundwork = 012, Phase 3 classification = 013
  (renumbered because Phase 2 builds first). Both additive; no conflict. Phase 3's
  classification columns + `comment_classification_state` apply uniformly to the
  unified `comments` table regardless of `kind`.
- **FK enforcement** is on (`db.go:22` DSN: `_foreign_keys=on`). Write order is
  thread row → its comments, so the FK is always satisfiable.

## Ingestion control flow

```
per PR (existing worker-pool fan-out):
  ├─ GetComments(prID)                          → issue comments        [REST, existing]
  ├─ if provider implements ThreadProvider:
  │     GetReviewThreads(prID)                   → []*ReviewThread       [REST + GraphQL, new]
  │     (not capable → skip; real error → log, continue with nil)
  └─ writer, ONE tx per PR:
        ├─ writePR            → upsert PR + observed-patchset capture
        ├─ writeComment       → issue comments (kind='issue')
        └─ writeReviewThreads → review_threads rows + their comments (kind='review')
```

All writes are upserts on stable IDs; re-sync refreshes resolution state + bodies
in place. Per-item write errors log and skip, never abort the PR tx.

## Observed-patchset capture (in `writePR`, same tx)

```
-- fetch the latest patchset for this PR (pin the query — no bare-column MAX):
latest := SELECT number, head_sha FROM patchsets WHERE pr_id = ? ORDER BY number DESC LIMIT 1
if no row:                       nextNum := 1
else if latest.head_sha != pr.HeadSHA: nextNum := latest.number + 1
else:                            no-op (head unchanged) → skip insert, then upsert PR
-- insert is race-safe via the UNIQUE(pr_id, head_sha) constraint:
INSERT OR IGNORE INTO patchsets {pr_id, number:nextNum, head_sha:pr.HeadSHA, base_sha:pr.BaseSHA, pushed_at:now}
then upsert the PR row (existing)
```

- **Idempotent at the DB level** via `UNIQUE(pr_id, head_sha)` + `INSERT OR IGNORE`
  — re-syncs (and concurrent workers racing on the same PR) record nothing for an
  already-seen head. The application-level `ORDER BY ... LIMIT 1` check computes the
  next number; the UNIQUE constraint is the actual race-safety guard.
- `pushed_at = time.Now()` at write (observation time). Name kept (`pushed_at`, not
  `observed_at`) so Stage 2's server backfills true push times into the same column
  — code + migration comments state this so no one "fixes" the name.
- First observation = P1; **no claim** about pushes before watching began.
- Atomic with the PR write (never diverge). No event emitted (deferred).
- Helper `recordPatchsetIfChanged(tx, prID, headSHA, baseSHA)` keeps `writePR`
  readable and the logic unit-testable.

## Current-user identity

At auth-success (`handler/auth.go`, where `Cache.Set` already runs), call
`CurrentUserStore.SetCurrentUser(provider, login)` — a narrow 1-method interface
added to `AuthDeps` (not the whole `*store.DB`). Upsert by `provider`. Write lands
exactly when the login is known → no empty window. User-scoped queries (2.2,
Phase 4) key on `current_user.login`.

## Error handling

- **GraphQL unavailable/fails:** caught inside `GetReviewThreads`; returns
  REST-only threads, `StateKnown=false`, no error. Resolution shows *unknown*, not
  *resolved*. A real REST failure → logged error, PR continues with nil threads.
- **Provider lacks `ThreadProvider`:** capability check skips; no error.
- **Per-item write failure:** logged and skipped inside the per-PR tx; never aborts.
- **Patchset capture failure:** logged, does not abort PR write; recoverable next
  sync (idempotent).
- **current_user write failure:** logged; auth still succeeds (cache write already
  done); user-scoped SQL falls back to "unknown user"; next auth re-writes.
- **FK integrity:** relies on `_foreign_keys=on` (confirmed); thread-before-comments
  write order satisfies the constraint.
- Go-side: idiomatic wrapped errors (`fmt.Errorf("...: %w")`). No TS this slice.

## Testing

- **Migration 012:** applies clean; existing comments get `kind='issue'`; new
  tables/indexes/FK exist and enforce (bogus thread_id rejected; deleting a thread
  NULLs its comments' thread_id).
- **`normalizeReviewComment`:** `Kind='review'`, `github-review-comment-<id>`,
  anchoring fields, `in_reply_to` populated.
- **REST+GraphQL stitch (table-driven, recorded fixtures):** both succeed → grouped
  + resolution set, `StateKnown=true`; GraphQL fails → REST-only, `StateKnown=false`,
  no error; reply chains reconstruct via `in_reply_to`.
- **Mock provider** implements `ThreadProvider`; a second mock without it asserts
  graceful skip.
- **Observed-patchset capture (most focus):** first obs → P1; unchanged head →
  no-op (row count stable); changed head → P2 incremented; atomic with PR write;
  `pushed_at` populated.
- **`writeReviewThreads`:** thread + comments land; re-sync flips `resolved` in
  place; bad item skipped without aborting tx.
- **current_user:** `SetCurrentUser` upserts by provider; queryable offline after.
- **ID collision guard:** issue + review comment with same numeric GitHub id coexist
  (`github-comment-N` vs `github-review-comment-N`).
- **Race detector** on the sync path.

## Acceptance

- Inline review comments fetched + stored as `kind='review'` rows with anchoring
  fields, alongside issue comments.
- Review threads stored with resolution state when GraphQL available; `StateKnown=0`
  and app still works when not.
- Patchset row recorded on head-SHA change; no-op when unchanged.
- `current_user` populated at login, queryable offline.
- Mock provider updated; full Go suite + race detector green.
- No renderer changes; existing comment query returns the new columns.

## Out of scope (later slices)

- Range-diff / system-git (2.1+2.2). Anchoring algorithm + `patchset_number`
  population (2.3). Unresolved-threads UI + resolve/unresolve write-back (2.4).
  Real-time `PATCHSET_RECORDED` event (deferred). Pure-Go range-diff + complete
  push capture (Stage 2).
