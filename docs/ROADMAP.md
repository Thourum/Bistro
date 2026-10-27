# Bistro FOSS — Roadmap to v1.0

> A keyboard-first, local-first desktop PR review client.
> This document covers the FOSS scope only. The Team plan has its own document.

> **Status audit (2026-06-10).** All status columns were verified against the codebase.
> Three corrections vs. the original draft:
> 1. *WebSocket port collision* upgraded to `partial` — `ws/server.go` already supports `:0` binding; only the Electron handshake is missing.
> 2. *`:open in browser`* upgraded to `partial` — the `o` keybinding (`list.openBrowser`) and `openExternal` IPC exist; only the palette command is missing.
> 3. *Schema multi-tenancy (6.3)* is largely already done — `prs` carries a `provider` column with `UNIQUE(provider, owner, repo, number)`, `repos.id` is `"provider:owner/repo"`, comment IDs are opaque TEXT.

> **Progress update (2026-06-12).** See `docs/UPDATE-2026-06-12.md` (removed
> 2026-07-11; in git history) for the full session log. Newly done: WS port handshake, migration-race fix, `:sync`.
> Honestly partial: 1.6 triage (data layer complete, UI placement gaps remain)
> and the `pr_diffs` pipeline (existed on paper, only now actually enabled).

> **🔍 Audit (2026-06-19).** A multi-agent audit of the built code + Phase 1/2 plans
> ran; 49 findings confirmed against the codebase (10 false positives dropped). Full
> record: `docs/ROADMAP-AUDIT-2026-06-19.md` (removed 2026-07-11; in git history). Inline `> ⚠️ Audit:` notes below mark
> affected items. **Headlines:** Phase 2's original wording overpromised FOSS
> completeness. BIS-31 resolved the changed-since-review strategy: FOSS records
> **observed patchsets**. **Updated 2026-10-23:** changed-since-review now uses
> per-file patch-ID triage and go-git interdiffs, with explicit unavailable reasons
> when local data is insufficient (see the [current design](go-project-overview.md#internalprsync----sync-engine)). Complete push capture remains Team/server scope. (The audit record
> and BIS-31 spec were removed from `docs/` on 2026-07-11 — recover from git history.) The comment layer
> can't feed Phase 2: only issue-level comments are fetched (not inline review
> comments), and the schema stores no commit SHA / position / resolution state.
> Resolve/unresolve is GitHub-GraphQL-only (the stack is REST). Two active bugs in
> shipped code were found and fixed this session (action-queue dedup, rate-limit
> provider key). Several Phase 2/4 features need an unscheduled prerequisite: a
> SQLite-resident current-user identity.

> **🔍 Status audit (2026-07-11).** A 5-agent verification of every phase against the
> codebase (migrations now at 025, commits through 2026-07-11). **Headlines:** Phase 2
> is substantially **built and shipped** — patchset timeline (2.1), since-last-review
> (2.2), files-viewed surviving rebase via content hash, the comment-anchoring backend
> (2.3), the unresolved-threads view with GraphQL resolve/unresolve and merge-readiness
> (2.4), comment replies, and a SQLite current-user identity (migration 022) all landed.
> Diff size budgets landed (BIS-25, migrations 024/025). Markdown + Mermaid rendering
> of PR bodies/comments and a Bistro design-system primitives pass shipped
> (2026-06-24/25) — neither was on the roadmap. Opt-in usage telemetry + Sentry crash
> capture are wired behind a consent setting. Phases 3 (classification) and 4
> (expertise) remain **not started** — zero code hits. **Updated 2026-10-23:**
> patch-ID triage and go-git per-file interdiffs have shipped; the old comparison
> degradation gap is cleared (see the [current design](go-project-overview.md#internalprsync----sync-engine)). Posting a
> *new* line-level comment is still rejected even though thread replies work. Dev
> plans/specs/audit docs were removed from `docs/` in this cleanup (git history keeps
> them). A consolidated v1.0 checklist is at the end of this document.

## How to read this document

The roadmap is broken into **phases** rather than fixed-length sprints. Each phase is a coherent chunk of work that produces something shippable or testable on its own. Phases are roughly ordered by dependency, but some can run in parallel if there's capacity.

Every feature has:

- **What it is** — concrete behaviour
- **Why it matters** — link back to the user pain it addresses, with sources
- **Status** — `done`, `partial`, `not started`
- **Acceptance** — what "complete" looks like
- **User story** — included where the feature is non-obvious or user-facing in a way that needs framing

The North Star for FOSS v1.0: *the best individual code review client on the market for the AI-PR era. Runs entirely on your machine. Works on GitHub today, GitLab next.*

---

## Finished phase details

- [Phase 0 — Foundation](roadmap/phase-0.md): largely done.
- [Phase 1 — Core Review Surface](roadmap/phase-1.md): mostly done (table stakes).
- [Phase 2 — Patchset Semantics](roadmap/phase-2.md): largely done (updated 2026-10-23).
- [Phase 3 — AI Comment Classification](roadmap/phase-3.md): implemented 2026-07-12; 3.7 deferred to Team plan.
- [Phase 4 — Local Expertise Scoring](roadmap/phase-4.md): implemented 2026-07-15.
- [Phase 5 — Inbox & Triage Polish](roadmap/phase-5.md): done 2026-08-09.

## Phase 6 — GitLab Provider

The first non-GitHub provider. Validates the abstraction.

### 6.1 Provider boundary audit (do this first)

Before writing GitLab code, sweep the existing codebase for GitHub-specific assumptions that have leaked past the provider interface. This is the highest-leverage week of engineering in the entire FOSS roadmap — refactor cost scales with the number of providers, and we want to do it once, while the count is one.

Known leaks as of the 2026-06-10 audit: `go/internal/auth/callback.go`, `go/internal/server/handler/{auth,diff,repo}.go` reference GitHub directly; ~~`rate_limit_state.id` defaults to `'github'`~~ (FIXED 2026-06-19, migration 011 — see 6.3). ⚠️ Audit (2026-06-19) added one more: the diff fetcher hardcodes `https://github.com/%s/%s.git` (`go/internal/sync/fetcher.go:149`) regardless of `pr.Provider` — move clone-URL construction into the provider layer. **Re-verified 2026-07-11: still leaking** — clone URL still hardcoded (`fetcher.go:184`), plus new leaks accrued since: account-provider constants and inbox discovery in `sync/orchestrator.go`, provider-prefixed keys in `store/last_reviewed.go`, OAuth scope assumptions in `auth/callback.go`.

**Audit checklist.**

- Hardcoded GitHub URLs, API patterns, or domain names anywhere outside `internal/provider/github/`.
- WebSocket event types named after GitHub concepts that don't translate (e.g., a "PR" vs "MR" leak).
- Renderer code with `github.com` strings, GitHub-specific icons, or hardcoded review state names.
- Schema columns whose values implicitly assume GitHub semantics.
- Action types in `internal/actions` whose payloads assume GitHub-shaped data.

### 6.2 GitLab implementation

**Acceptance.**

- `internal/provider/gitlab/` with full `Provider` interface implementation.
- OAuth via gitlab.com only at v1.0 (self-hosted GitLab deferred — see scope notes).
- Same patchset model as GitHub: clone the repo locally, compute patchsets from git, ignore GitLab's `merge_request_diffs` API. Architectural consistency over provider convenience.
- Same expertise scoring, same classification rules, same UI.
- Multi-account support: a user can be signed in to GitHub and GitLab simultaneously and see PRs from both in one inbox.

### 6.3 Schema multi-tenancy

Largely done already (verified 2026-06-10): `prs` carries `provider` with `UNIQUE(provider, owner, repo, number)`, `repos.id` is `"provider:owner/repo"`, comment IDs are opaque TEXT. Remaining:

- Verify no URLs are stored as columns; reconstruct from the provider instead.
- ~~Migrate `rate_limit_state` off its `'github'` default id.~~ ✅ Done (2026-06-19): migration `011_fix_rate_limit_state_provider_key.sql` recreates the table without the `DEFAULT 'github'`, so distinct providers coexist without overwriting each other.

---

## Phase 7 — v1.0 Release Prep

A focused phase on quality, docs, distribution.

### 7.1 Quality gates

- Bistro reviews its own PRs (eat dog food).
- Performance budget: 3000-file PR opens < 2s to first paint.
- Memory budget: < 1GB on a 10000-line diff.
- Crash-free sessions: > 99.5% across last 7 days of internal use.

### 7.2 Distribution

- macOS DMG (signed + notarised). *(2026-10-19: ZIP + DMG makers and the release workflow `.github/workflows/release.yml` exist; the workflow signs and notarizes only when the `MACOS_*` / `APPLE_*` secrets are set, otherwise the alpha is unsigned. Alpha install steps: `docs/ALPHA.md`.)*
- Windows MSI (signed). *(not started)*
- Linux AppImage and `.deb`. *(not started)*
- Auto-update via the standard Electron updater. *(code exists in `main/updater.ts` via `update-electron-app`, env-gated / disabled by default)*
- Homebrew cask formula. *(not started)*

### 7.3 Documentation

- `README.md` with the value-prop framing (see Communication Plan doc).
- `docs/install.md`, `docs/keybindings.md`, `docs/classification.md`, `docs/expertise.md`, `docs/byok.md`.
- A 90-second screencast on the landing page.
- `docs/architecture.md` for contributors (the existing architecture writeup, condensed).
- `CONTRIBUTING.md` with DCO sign-off instructions and a clear scope statement:
  *what we will and won't merge.*

### 7.4 Telemetry — largely done (2026-07-11)

- Off by default. Opt-in only. ✅ (consent-gated in `main/telemetry.ts` + Settings)
- If on: anonymised crash reports + feature-usage counts (no PR content, no repo names, no usernames). ✅ (usage telemetry + Sentry crash capture in `main/sentry.ts`, both behind the same consent path)
- Settings page surfaces exactly what's sent. partial — a "Usage Telemetry" toggle exists; no separate crash-reporting setting or detailed what-is-sent surface.

---

## Out of scope for FOSS v1.0 (reserved for Team plan)

To keep the line clean, these are *not* in the FOSS scope:

- Anything requiring a server we operate.
- Shared state between humans (seen-by-teammate, reviewed-by-teammate, etc).
- Departed-employee detection beyond the 90-day activity heuristic.
- Cross-machine expertise graph (Layer 2).
- Codebase-aware AI (whole-repo indexing).
- Slack / Linear / Jira integrations as a team surface.
- Compliance features (SSO, SAML, audit trail).
- Analytics dashboards.
- Self-hosted GitLab (deferred; community PR welcome).
- Bitbucket, Azure DevOps, Gitea, Forgejo (community PRs welcome; not core team focus pre-v1).
- Mercurial, jj (out of scope, period).

---

## Open questions to resolve before v1.0

1. ~~**`@pierre/diffs` license.**~~ **Resolved (2026-10-19):** `@pierre/diffs` and `@pierre/theming` are Apache-2.0, which is GPLv3-compatible. Electron and its bundled deps are permissive. Bundled `@fontsource/*` fonts are OFL-1.1, which needs human review before a non-alpha release (see `THIRD_PARTY_NOTICES.md`).
2. ~~**Bot list update mechanism.**~~ **Resolved (2026-06-19):** bundled via `go:embed`,
   distributed through the app's normal auto-update — no runtime fetch, no network
   dependency. Built-ins seed to `~/.config/bistro/rules/` on first launch and are
   presence-gated-updated on upgrade; user files are never touched. See the Phase 3
   foundation spec/plan.
3. ~~**Telemetry stance.**~~ **Resolved in code (2026-07-11):** opt-in only, consent-gated usage telemetry shipped (`electron/src/main/telemetry.ts`); formalize the stance in user docs before release.
4. ~~**Crash reporter.**~~ **Resolved in code (2026-07-11):** Sentry, behind the same opt-in consent path (`electron/src/main/sentry.ts`).
5. **Auto-update server.** Self-hosted via static files, or a service?
6. ~~**License for the FOSS code.**~~ **Resolved (2026-06-30):** Bistro is
   licensed under GPLv3-only with DCO sign-off for contributions. See
   `LICENSE`, `DCO.txt`, and `CONTRIBUTING.md`.

---

## v1.0 checklist (as of 2026-07-11)

Everything left between here and FOSS v1.0, in rough dependency order. Done work is omitted — see the phase sections above.

**Blockers (release-gating)**
- [x] `@pierre/diffs` license review (open question #1). OFL-1.1 font review still open in `THIRD_PARTY_NOTICES.md`
- [ ] Security gates pass: `govulncheck` and `pnpm audit --prod` currently fail (reachable Go vulns in `go/`; 1 critical in `electron/` via `seroval`). Fix and raise `PNPM_AUDIT_LEVEL` to `high` in the release workflow
- [ ] macOS signing + notarization (secrets not yet configured; alpha ships unsigned); decide auto-update hosting (open question #5)
- [ ] Windows MSI, Linux AppImage/`.deb`, Homebrew cask (7.2)
- [ ] Post *new* line-level review comments (`PostReviewComment` with path/commitSHA/side) — draft-based line comments shipped 2026-10-23 via `SubmitReview` / `CreateReview` (1.3); direct `PostReviewComment` remains unimplemented
- [x] Former range-diff handler wiring blocker superseded (2026-10-23): patch-ID triage and go-git interdiffs shipped; `s` no longer falls back after an observed force-push when comparable fingerprints exist (2.1/2.2)

**Phase 3 — classification (implemented 2026-07-12, the wedge)**
- [x] 3.1 schema + 3.2 bot identification + 3.3 rules engine + 3.4 heuristics
- [x] 3.5 bulk ops, 3.6 dedup, 3.8 BYOK (3.7 slop score deferred to Team plan, 2026-07-12)
- Spec for all of the above: `docs/superpowers/specs/2026-07-12-comment-classification-design.md`
- User guide: `website/src/pages/docs/comment-classification.mdx`
- Follow-ups (non-gating): remaining vendor rule files as samples land (Copilot,
  Qodo, Cursor BugBot, Claude Code, Codex, Sourcery); `d b` per-vendor picker;
  manual acceptance still pending — `local+provider` dismissal resolving a real
  GitHub thread, and the rule-edit-without-restart walkthrough on a live PR

**Phase 4 — expertise (implemented 2026-07-15)**
- [x] 4.1 scoring engine + `file_expertise` table (signals 1–3; signal 4 → Team)
- [x] 4.2 "Touches Your Code" view, 4.3 SQLite settings (expertise.yml dropped)
- Spec: `docs/superpowers/specs/2026-07-15-local-expertise-scoring-design.md`
- User guide: `website/src/pages/docs/expertise.mdx`

**Phase 5 — inbox & triage polish (done 2026-08-09)**
- [x] Mentions rule implemented (1.6) — `mentioned` column (migration 033), computed at sync time from body + comments
- [x] Six-lane inbox with `g1`–`g6`, read state, `Shift-R` (5.1)
- [x] CodeView migration, side-by-side toggle, word-level diff, whitespace/generated-file collapse, viewed-file sidebar feedback (5.2)
- [x] `:open in browser`, `:open in editor` (with checkout verification), `:tour`, diff-route command palette (5.3)
- [x] First-run tour (5.4); "did-you-know" tooltips marked won't-do
- Design reference: `docs/DESIGN.md` (plan/spec removed after shipping)
- Follow-ups (non-gating, deferred): image/binary diff rendering (placeholder ships instead)

**Polish (partial, not part of Phase 5)**
- [ ] Comment-level "outdated near line N" display (2.3 remainder)
- [ ] PR_METADATA_CHANGED for description/labels/assignees/reviewers; event for *remotely* resolved threads (1.5)
- [ ] Per-open-PR fast sync (1.4); per-repo last-sync/error/rate-limit indicator (Phase 0)

**Phase 6 — GitLab**
- [ ] 6.1 provider-boundary sweep (clone URL, orchestrator constants, `last_reviewed` keys, auth callback)
- [ ] 6.2 GitLab provider; 6.3 remaining multi-tenancy checks

**Phase 7 — release prep**
- [ ] Quality gates: perf/memory budgets, crash-free-session tracking (7.1)
- [ ] `docs/install.md`, `docs/classification.md`, `docs/expertise.md`, `docs/byok.md`, `docs/architecture.md`; README value-prop pass; screencast (7.3)
- [ ] Document telemetry stance in user docs; itemized "what is sent" in Settings (7.4)
