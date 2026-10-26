# Phase 4 — Local Expertise Scoring

[Roadmap](../ROADMAP.md)

## Phase 4 — Local Expertise Scoring (the killer feature, FOSS layer) — **status: implemented 2026-07-15**

> **Implemented 2026-07-15** — sub-features 4.1–4.3 landed: local-only
> expertise indexing, the "Touches Your Code" inbox filter, per-file why view,
> nudge, palette commands, and SQLite-backed expertise settings.
> Authoritative docs:
> - Spec: `docs/superpowers/specs/2026-07-15-local-expertise-scoring-design.md`
> - Plan: `docs/superpowers/plans/2026-07-15-local-expertise-scoring.md`
> - User guide: `website/src/pages/docs/expertise.mdx`

> ✅ **Spec (2026-07-15):** `docs/superpowers/specs/2026-07-15-local-expertise-scoring-design.md`
> is authoritative; the draft below is left for history. Key deviations:
> - **Signal 4 (prior reviewer) deferred to Team plan** — synced review data is
>   sparse/biased locally; Team computes it server-side with complete data.
> - **`.bistro/expertise.yml` removed permanently** (not deferred) — Team uses an
>   AST + vector-store approach, so a repo-committed config file has no future.
>   Excludes/thresholds live in SQLite settings; built-in defaults +
>   `.gitattributes linguist-generated=true` cover the common case.
> - **Opt-in per repo, local checkouts only** — expertise runs only on repos with
>   `source='local'` that the user explicitly enables (`enable expertise` palette
>   command, or a nudge after 10 reviewed PRs from the same local repo). Full-repo
>   index on enable, then incremental updates for PR-touched files on sync.
> - **System git via the bounded os/exec exception** (BIS-31 precedent);
>   git absent → labeled unavailable state, no pure-Go fallback.

The Layer 1 of expertise routing. Computes per-file expertise from local git history. Layer 2 (team-wide expertise graph, departed-employee handling, write-back to provider) is Team-only.

### 4.1 Expertise score per file per author — **done (2026-07-15)**

**Algorithm.** A weighted sum of:

1. **Surviving lines** (high weight, no decay). `git blame HEAD` aggregated by author. Who wrote the code that's still in the file today?
2. **Substantial commits in last 12 months** (medium weight, recency-decayed with ~6mo half-life). "Substantial" = changed > 5 lines, not a typo fix.
3. **Original authorship** (medium weight, slow decay). Who first created the file? `git log --follow --diff-filter=A`.
4. ~~**Prior reviewer of this file** (low-medium weight, recency-decayed). Who has reviewed PRs that touched this file?~~ **Deferred to Team plan (2026-07-15)** — see spec note above.

**Recency cutoff.** Only include authors who have *any* commit or review in the watched repos within the last 90 days. This catches departed contributors at FOSS level (without org context). Authors who fail this cutoff are flagged `dormant` — still recorded, but excluded from default suggestions.

**Acceptance.**

- Initial computation runs once per repo on watch (can take a few minutes on a large repo; show progress).
- Incremental updates on each new commit synced.
- Stored in `file_expertise(repo_id, path, author, score, last_computed_at, dormant)`.
- A "why" view per file: "Alice: 312/480 surviving lines, original author, 4 reviews in last 6 months → score 87."

### 4.2 "PRs touching files you've worked on" view — **done (2026-07-15)**

**User story.**

> As a senior dev, I want Bistro to show me PRs touching files I have meaningful history with, even if I wasn't requested as a reviewer, so that changes to code I care about don't slip past me.

**Acceptance.**

- A new view in the PR list: "Touches Your Code (N)."
- A PR appears here if any file it touches has the current user in the top 3 expertise scores for that file.
- Each row shows which files are "yours" with the expertise score.
- Default behaviour is *FYI in the local inbox only*. No external notifications. No write-back to provider. The user can promote one to a real reviewer-request manually.

### 4.3 Configuration — **done (2026-07-15; no repo file)**

> `.bistro/expertise.yml` and pinned owners are **removed permanently** — the
> Team plan replaces them with AST + vector-store expertise. FOSS config is
> SQLite-backed, edited in Settings.

**Acceptance.**

- Built-in exclude defaults (`vendor/**`, `dist/**`, lockfiles, `**/*.pb.go`, `**/testdata/**`) plus per-repo user exclude globs in Settings.
- Respects `.gitattributes linguist-generated=true`.
- A global setting for "minimum surviving lines to count as expert" (default 50).
- Manual alias emails ("also count these commits as me") in Settings.

---

