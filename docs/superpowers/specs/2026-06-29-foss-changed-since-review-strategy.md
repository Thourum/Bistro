# FOSS Changed-Since-Review Strategy

Issue: BIS-31
Owner: Diff Performance Engineer
Status: Decided

## Decision

FOSS Bistro will ship **honest, local-first changed-since-review**:

1. Record **observed patchsets only**. A patchset is a distinct PR head SHA Bistro saw during sync, ordered by observation. FOSS does not promise to capture force-pushes that happened and disappeared between syncs.
2. Record the reviewer's last reviewed **observed patchset identity** on review submission: patchset number, head SHA, and base SHA when available.
3. For “since last review”, prefer `git range-diff` between observed patchset ranges when the system `git` binary is available and Bistro has the required local repo objects.
4. Degrade to a clearly labeled simplified file diff for the current latest pair when range-diff is unavailable or the local clone/object data is not present.
5. If Bistro cannot compute a pair-specific changed-since-review diff without misleading the reviewer, show current diff state plus an unavailable/degraded reason instead of inventing server-grade history.

This is a product honesty decision: FOSS optimizes for local speed and privacy, not perfect push-history completeness.

## Trade-offs considered

### Best-effort matched commits via `git range-diff`

- Pros: closest to GitHub/Gerrit changed-since-review semantics after rebases; already bounded behind a capability probe and graceful fallback.
- Cons: requires system `git >= 2.19`; requires the relevant observed commits/ranges to be present locally; cannot reconstruct pushes Bistro never observed.
- Decision: use as the preferred FOSS path, but never as a guarantee.

### Simplified file diff since last review

- Pros: fast, locally generated, works with existing `pr_diffs` cache for the current snapshot, keeps reviewers moving.
- Cons: not rebase-aware; can over-report changes after commit rewriting; only safe when the requested pair matches cached diff identity.
- Decision: keep as labeled degraded fallback, not as the primary semantic claim.

### Explicit deferral

- Pros: avoids overpromising.
- Cons: punts a key AI-PR pain point even though FOSS can provide useful best-effort behavior.
- Decision: defer only **complete push capture** and pure-Go/server-grade range-diff; do not defer the FOSS best-effort feature.

## Local-first constraint

A desktop poller or local `go-git` fetch only sees repository state at sync time. If an author force-pushes P2 and then P3 before Bistro syncs, P2 may be unrecoverable. GitHub REST/timeline polling cannot provide every overwritten before/after SHA. Complete capture requires a continuously listening server receiving push webhooks.

## UI/product wording

Use wording like:

- “Observed patchsets”
- “Changes since your last observed review”
- “Range-diff unavailable; showing current file diff”
- “Pair-specific diff unavailable because Bistro did not observe or retain the required patchset objects”

Avoid wording like:

- “Every push” in FOSS docs/UI
- “Complete force-push history”
- “Always survives rebase”
- “Server-grade changed-since-review”

## Deferred to Team/server scope

`docs/STAGE-2-TEAM.md` remains the home for complete push capture. A Bistro-operated server can subscribe to provider webhooks, capture true push timestamps and before/after SHAs, and backfill the same patchset model. FOSS keeps schema compatibility but not the completeness promise.
