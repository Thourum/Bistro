# Stage 2 (Team) — Deferred Scope

> **Purpose.** Bistro ships in two stages. **Stage 1 (FOSS)** is the local-first,
> no-server desktop client covered by `docs/ROADMAP.md`. **Stage 2 (Team)** adds a
> server Bistro operates, enabling real-time monitoring and capabilities that are
> structurally impossible without one.
>
> This document parks Stage-2 work surfaced during Stage-1 design so it isn't lost.
> Items here are **explicitly out of FOSS v1 scope**. Created 2026-06-20 during the
> Phase 2 re-scope (see `docs/ROADMAP-AUDIT-2026-06-19.md` — removed 2026-07-11, in git history — for why these were deferred).

---

## S2.1 — Server-based patchset capture (complete push history)

**Why it's here, not in FOSS.** A local-first poller (GitHub API or local `go-git`
fetch) only observes repo state at sync time. Two force-pushes inside one sync
window collapse to a single observed head SHA; the intermediate push is gone with
no way to recover it. Only a server *receiving push webhooks* sees every push.

**Stage-1 (FOSS) behaviour we keep.** FOSS records **observed patchsets** — a
patchset is a distinct head SHA Bistro observed at sync time, numbered in
observation order. The `pushed_at` column holds the best-available timestamp
(observation time under polling). Per-open-PR fast-sync (roadmap 1.4) makes capture
good in practice for PRs under active review. The schema and UI are deliberately
identical to what Stage 2 will use.

**Stage-2 addition.**
- A Bistro-operated server subscribes to provider push/force-push events (GitHub
  webhooks; GitLab equivalent).
- Every push — including force-pushes superseded seconds later — is recorded with
  its **true** push timestamp and before/after SHAs.
- The server backfills the *same* `patchsets` table the FOSS client already uses.
  `pushed_at` now carries real push time instead of observation time. **No schema
  change, no rename** — this is exactly why FOSS keeps the column named `pushed_at`
  rather than `observed_at`.
- Result: Gerrit-grade "every push is a numbered patchset" completeness, which FOSS
  cannot promise.

---

## S2.2 — Pure-Go `git range-diff` engine (standalone foundational module)

> **Superseded for FOSS (2026-10-23):** FOSS no longer shells out to `git range-diff`; since-last-review uses per-file patch-ID triage and go-git interdiffs (see `docs/go-project-overview.md`, `internal/sync`). A pure-Go range-diff remains a possible Team feature.

**Why it's here, not in FOSS.** FOSS v1 gets range-diff semantics by shelling out
to the **system `git range-diff`** binary (`os/exec`), when present. If the user has
no system git, the rebase-aware "since last review" / patchset-pair interdiff
degrades gracefully (plain head-to-head diff). This is a deliberate, scoped
exception to Bistro's "pure go-git, no `os/exec`" principle, limited to range-diff.

No library provides range-diff for binding (verified 2026-06-19): libgit2 / git2go,
gitoxide (gix), git2-rs, and GitButler's `grit` all lack it; GitButler's own
"interdiff" is a closed-source **server** computation. So a library shortcut does
not exist — it's either the system binary (FOSS) or a hand-roll (Stage 2).

**Stage-2 addition.** Implement range-diff as a **standalone, well-tested pure-Go
module** that 2.1 (patchset-pair diff) and 2.2 (since-last-review) consume — built
before the features that need it, not inline. This removes the system-git dependency
entirely and lets the server compute interdiffs centrally.

Algorithm (confirmed from `git-range-diff` docs, 2026-06-19):
1. Per-commit patch generation for both ranges — **go-git already does this**
   (`commit.Patch(parent).String()`).
2. Cost matrix: diff each patch pair (3 context lines); cost = line count of that
   meta-diff. Use `sergi/go-diff` (already in `go.sum`).
3. Pad with fudge/creation-factor slots (default 60%) so unrelated patches don't
   falsely match.
4. Minimum-cost bipartite assignment — git uses Jonker-Volgenant; a Munkres/Hungarian
   Go lib (`clyphub/munkres` or similar, ~30 LOC glue) is equivalent at our scale
   (commits per PR ≤ ~30).
5. Meta-diff rendering — dual-color diff-of-diffs (outer = which range, inner = what
   changed). **The hard ~30% of the work and most of the correctness risk.**

Estimate: ~650 LOC, ~4–6 engineer-days, +1–2 days to match git's output closely for
tests. The expensive half (patch generation) is already covered by go-git; the
assignment solver is a small library; the renderer is the real cost.

---

## Other Stage-2 items (from the FOSS roadmap "Out of scope" list)

Recorded here for completeness; see `docs/ROADMAP.md` "Out of scope for FOSS v1.0":
shared state between humans (seen-by-teammate), Layer-2 cross-machine expertise
graph, departed-employee detection beyond the 90-day heuristic, codebase-aware AI
(whole-repo indexing), team integrations (Slack/Linear/Jira), compliance (SSO/SAML/
audit), analytics dashboards.
