# Bistro FOSS — Roadmap to v1.0

> A keyboard-first, local-first desktop PR review client.
> This document covers the FOSS scope only. The Team plan has its own document.

> **Status audit (2026-06-10).** All status columns were verified against the codebase.
> Three corrections vs. the original draft:
> 1. *WebSocket port collision* upgraded to `partial` — `ws/server.go` already supports `:0` binding; only the Electron handshake is missing.
> 2. *`:open in browser`* upgraded to `partial` — the `o` keybinding (`list.openBrowser`) and `openExternal` IPC exist; only the palette command is missing.
> 3. *Schema multi-tenancy (6.3)* is largely already done — `prs` carries a `provider` column with `UNIQUE(provider, owner, repo, number)`, `repos.id` is `"provider:owner/repo"`, comment IDs are opaque TEXT.

> **Progress update (2026-06-12).** See `docs/UPDATE-2026-06-12.md` for the full
> session log. Newly done: WS port handshake, migration-race fix, `:sync`.
> Honestly partial: 1.6 triage (data layer complete, UI placement gaps remain)
> and the `pr_diffs` pipeline (existed on paper, only now actually enabled).

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

## Phase 0 — Foundation (status: largely done)

This is the existing architecture. Most of it ships in pre-v1 builds. Listed here for completeness and to flag what still needs hardening before v1.0.

### Architecture in place

| Feature | Status | Notes |
|---|---|---|
| Two-process design (Go sync + Electron UI) | done | `go/cmd/bistro-sync` + `electron/src/main/go-spawn.ts`; stable enough to build on |
| SQLite-as-cache, Go-owns-writes | done | `go/internal/store`, migrations 000–009 |
| WebSocket transport over localhost | done | `go/internal/ws/server.go` enforces local origin; pin the port-binding strategy before v1 (see below) |
| Provider interface in `internal/provider` | done | Pre-built for multi-provider; GitHub is the only impl today (plus mock) |
| Migration runner with embedded SQL | done | Versioned, idempotent, tested |
| go-git for clone + unified diffs | done | go-git v5; foundation for every "since I last reviewed" feature later |
| OS keychain token storage | done | macOS / Windows / Linux all working via `go-keyring` (`go/internal/keychain`) |
| Structured slog logging with context propagation | done | `go/internal/logging/context.go`; `sync_id`, `repo`, `pr_id` etc carry through |
| Offline action queue | done | `store/action_queue.go` + migration 004; survives rate-limit and network failures |
| Crash monitoring (Go process → Electron) | done | `go-spawn.ts` onCrash + bounded restarts; UI shows error state instead of silent hang |
| Keyboard engine on TanStack Hotkeys | done (2026-06-12) | Custom matcher replaced; bindings are pure config in `utils/keybindings.ts`, dispatch/format via `@tanstack/react-hotkeys` |
| Sync-engine self-protection | done (2026-06-12) | Single-instance flock, busy_timeout, HTTP + per-PR timeouts, stdin parent-death watchdog, SYNC_ERROR broadcast |

### Foundation hardening before v1.0

| Item | Status | Action |
|---|---|---|
| WebSocket port collision | done | Go binds `:0` and prints `{"event":"ready","wsPort":N}`; Electron parses it and delivers the port to the renderer via preload additionalArguments |
| OAuth scope honesty | partial | Current spec says "minimal scopes, no write." That's incompatible with approve / comment / request-changes. Update to "scope per feature" model — see Phase 1 |
| `@pierre/diffs` license review | not started | Verify the license terms before v1.0 ships publicly (currently `@pierre/diffs@^1.1.15`) |
| Initial-launch SQLite migration race | done | The ready signal is only emitted after migrations complete and the WS listener is up; Electron blocks on it |
| `pr_diffs` table size budget | not started | Add a config setting for max cache size and a vacuum strategy |
| `repos` watched-flag UI exposure | partial | Already in schema; surface a per-repo "last sync time / error / rate-limit remaining" indicator |

---

## Phase 1 — Core Review Surface (table stakes, mostly done)

This is what makes Bistro a *credible* PR client. Most is built; the gaps are filed honestly below.

### 1.1 PR list, filters, search

Three-pane layout (list / detail / status), filters for Open / Draft / Merged, search across title / author / repo / filename, per-repo watch toggle.

| Sub-feature | Status | Notes |
|---|---|---|
| Three-pane layout | done | |
| Open / Draft / Merged filters | done | `FilterMenu.tsx` |
| Search across title/author/repo/filename | done | `SearchBar.tsx`, `/` binding |
| Repo picker + watch toggle | done | `RepoPicker.tsx`, `WATCH_REPO_RESULT` event |
| PR row metadata (state, age, +/-, file count, review status) | done | |
| **Mark as "needs my action" vs "FYI"** | partial | See 1.6. Done: classification logic, lane counters, `n` filter, and the data layer (requested reviewers + reviews API now populate review_status — it was empty before 2026-06-12). NOT done from the acceptance criteria: counters live in the inbox header, not the sidebar, and the default view is still "Open", not "Needs Action" |

### 1.2 Diff viewer

File-by-file unified diff via `@pierre/diffs`, syntax highlighting via highlight.js, inline comment threads, keyboard navigation (`n/p` files, `[/]` hunks, `hjkl` scroll).

| Sub-feature | Status | Notes |
|---|---|---|
| File-by-file unified diff | done | `PierreDiffView.tsx` |
| Syntax highlighting | done | highlight.js 11.x |
| Inline comment threads | done | `CommentThread.tsx` |
| Diff cached in `pr_diffs` | done | migration 007. Honesty note: the pipeline existed but was dormant until 2026-06-12 — sync never passed IncludeDiff, the git client was unwired, and clone auth was env-only. Now end-to-end with keychain auth + per-file stats |
| Keyboard navigation | done | |
| **Side-by-side diff toggle** | not started | Some users strongly prefer; cheap to add |
| **Generated-file collapse** | not started | Respect `.gitattributes linguist-generated`; auto-collapse `*.pb.go`, `dist/`, etc |

### 1.3 PR actions

Approve (with `a a` confirmation), request changes, post line/thread comments, exponential-backoff retry, offline queue.

| Sub-feature | Status | Notes |
|---|---|---|
| Approve / request changes / comment | done | |
| `a a` double-tap confirmation | done | `keyboard.ts` |
| Exponential backoff retry | done | `actions/executor.go` |
| Offline action queue with replay | done | |
| **Resolve / unresolve conversation** | not started | Currently can post comments but can't mark threads resolved (`PRDetailPage.tsx` has a TODO for real resolution tracking) |
| **Reviewer-request write-back** | not started | Required for Phase 4 expertise routing to be useful |
| **Idempotency keys on retried actions** | not started | Hash of `(action_type, target_id, content_hash)` to dedupe on retry |

### 1.4 Sync engine

5-min periodic sync, event-triggered sync via `syncTrigger`, concurrent per-PR fan-out, 429 handling that flips into queue mode, in-place query cache merging.

| Sub-feature | Status | Notes |
|---|---|---|
| Periodic + event-triggered sync | done | `server/syncloop.go` |
| Worker-pool fan-out | done | `sync/workerpool.go` |
| 429 detection and queue-mode | done | covered by orchestrator integration tests |
| In-place TanStack Query merging | done | |
| **Per-currently-open-PR fast sync** | not started | When a PR is open in the UI, sync it every 30–60s; everything else stays at 5m |
| **Manual force-sync command** | done | `:sync` palette command → FORCE_SYNC → syncTrigger; SYNCING footer status |

### 1.5 Real-time event coverage

WebSocket events for sync / PR / comment / auth / rate-limit / diff. Currently good for the things that have events.

In addition to the events below, the following are already implemented (not originally listed): `ACTION_COMPLETE`, `ACTION_FAILED`, `AUTH_REDIRECT`, `AUTH_SUCCESS`, `AUTH_FAILED`, `AUTH_CHECK_RESULT`, `AUTH_DISCONNECT_RESULT`, `PR_LIST_READY`, `REPO_LIST_READY`, `WATCH_REPO_RESULT`.

| Event | Status | Notes |
|---|---|---|
| `SYNC_COMPLETE`, `SYNC_ERROR` | done | |
| `PR_APPROVED`, `COMMENT_POSTED` | done | |
| `AUTH_TOKEN_EXPIRED`, `RATE_LIMITED` | done | |
| `DIFF_READY`, `COMMENTS_READY` | done | |
| **PR description / labels / assignees / reviewers changed** | not started | Currently invisible to the UI; feels off in a real-time tool |
| **CI status changed** | partial | Status is fetched, but no event; UI must wait for sync cycle |
| **Conversation resolved/reopened** | not started | Goes with 1.3 resolve write-back |

### 1.6 "Needs my action" vs "FYI" classification

**Why it matters.** The single highest-frequency complaint about GitHub notifications in the research was the lack of priority signal — every email is the same urgency. Computed locally from review-requested + mentioned + change-requested + author-vs-reviewer + draft state.

**User story.**

> As a developer who's on the reviewer list of 30 active PRs, I want my PR list to clearly separate the ones that are *waiting on me right now* from the ones that are *FYI*, so that I can act on the urgent ones without scrolling past the noise.

**Acceptance.**

- A persistent counter in the sidebar shows `Needs Action (N) / Waiting on Author (M) / FYI (K)`.
- Single-key filter to each.
- Default view is "Needs Action."
- Logic: review requested + not yet reviewed → Needs Action. Author + change requested → Needs Action. Approved + waiting for merge → Waiting on Author. Mentioned but not requested → FYI. Draft → FYI.

---

## Phase 2 — Patchset Semantics (the architectural unlock)

This is where Bistro starts having features that GitHub structurally cannot match. Everything here falls out of the fact that we clone the repo locally with go-git.

The architectural principle: **same patchset model for every provider**. Even where a provider exposes patchsets natively (GitLab), we use our own, computed from local git history. Mixing provider APIs and local computation produces inconsistent behaviour and divergent code paths.

### 2.1 Patchset timeline

**Why it matters.** The most-cited reason developers say they "miss" Gerrit is the patchset model — every push is a numbered patchset, comments are anchored to patchsets, and you can diff any pair. GitHub's "Changes since last review" button breaks under rebase ([daisy.wtf](https://daisy.wtf/writing/github-changes-since-last-review/), [community/3478](https://github.com/orgs/community/discussions/3478)).

**User story.**

> As a reviewer of a PR that's been force-pushed three times, I want to see the history of pushes as a numbered timeline and diff any two patchsets against each other, so that I can audit what actually changed across iterations without losing my place.

**Acceptance.**

- Each push to a PR is recorded as a patchset with a sequential number, its head SHA, and its timestamp.
- Patchsets persist even after rebase (we store them; the provider can lose them).
- A timeline UI in the PR detail shows P1 / P2 / P3 with diff-pair selector.
- Diffing arbitrary patchset pairs uses `git range-diff` semantics, not raw commit diff (so no-op rebases produce empty diffs).

**Schema.**

```sql
CREATE TABLE patchsets (
  pr_id INTEGER NOT NULL,
  number INTEGER NOT NULL,
  head_sha TEXT NOT NULL,
  base_sha TEXT NOT NULL,
  pushed_at TIMESTAMP NOT NULL,
  PRIMARY KEY (pr_id, number)
);
```

### 2.2 "Since I last reviewed" — survives rebase

**Why it matters.** GitHub's button works only if commits aren't rebased. As AI agents increasingly force-push every iteration, this gets worse, not better.

**User story.**

> As a reviewer returning to a PR after the author force-pushed, I want a "since my last review" view that shows me only what genuinely changed since I approved, so that I don't have to re-read 800 lines I already signed off on.

**Acceptance.**

- A `last_reviewed_sha` per `(pr_id, user_id)` is recorded on every review submission.
- "Since last review" toggle renders `git range-diff last_reviewed_sha..head_sha` against the prior base/head.
- The toggle is available even when commits have been rebased away.
- "Files viewed" state survives rebase if the file content hasn't actually changed.

### 2.3 Comment anchoring across rebase

**Why it matters.** The "where did my comment go?" thread on github/community has been growing for 7 years.

**Acceptance.**

- Comments are anchored to `(file_path, line_content_hash, surrounding_context)` in addition to line number.
- After rebase, comments resolve to their new line numbers if the content survived.
- If the content was deleted, the comment is shown as "outdated" but remains visible inline at its nearest surviving line, not banished to a sidebar.

### 2.4 Canonical "all unresolved threads" view

**Why it matters.** Free win from our SQLite cache — GitHub structurally cannot show every unresolved thread in one pane because of how their UI virtualises the timeline.

**Acceptance.**

- A dedicated view (keystroke `gu` for "go unresolved") lists every unresolved thread in the current PR, including threads on outdated lines.
- Each thread shows lifecycle state (Open / Resolved / Outdated) explicitly.
- Browser-style find-in-page works across all threads, including collapsed ones.
- A merge-readiness check surfaces every open thread.

---

## Phase 3 — AI Comment Classification (the wedge)

This is what makes Bistro the answer to the AI-PR era. Implemented as a rules engine + heuristics, with optional BYOK to push specific tasks to a user-supplied LLM.

**Design principle.** Every classification is deterministic, inspectable, and explainable. The user can always click a comment and see *why* it was tagged as "nit" — the matching rule, the regex, the heuristic.

### 3.1 Schema additions

```sql
ALTER TABLE comments ADD COLUMN author_type TEXT;       -- 'human' | 'bot' | 'unknown'
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;        -- 'coderabbit' | 'greptile' | 'copilot' | …
ALTER TABLE comments ADD COLUMN severity TEXT;          -- 'critical' | 'warning' | 'suggestion' | 'nit' | 'info'
ALTER TABLE comments ADD COLUMN category TEXT;          -- 'security' | 'correctness' | 'performance' | 'style' | 'readability' | 'docs'
ALTER TABLE comments ADD COLUMN classification_source TEXT; -- 'rule:<name>' | 'byok:<provider>' | 'manual'
ALTER TABLE comments ADD COLUMN dedup_group_id TEXT;    -- comments in the same group are duplicates of each other
```

### 3.2 Bot identification

**Acceptance.**

- Curated vendor list shipped as data (YAML), updated independently of the binary.
- Top vendors covered at v1.0: CodeRabbit, Greptile, Copilot Code Review, Qodo, Cursor BugBot, Claude Code, Codex, Sourcery.
- Unknown bots (author login ends in `[bot]` or matches heuristics) flagged as `bot:unknown`.
- Per-PR header counter: `Human: 4 · CodeRabbit: 23 · Greptile: 8 · Other bots: 2`.

### 3.3 Severity classification (rules engine)

A YAML-driven rules engine that's hot-reloadable, shippable as data, and extensible by users.

```yaml
# bistro/rules/builtin/coderabbit.yml
name: CodeRabbit
match:
  author: ["coderabbitai[bot]", "coderabbit-ai[bot]"]

classifiers:
  - name: emoji_severity
    pattern: '^(?P<emoji>🔒|⚠️|🛠️|💡)'
    severity_map:
      "🔒": critical
      "⚠️": warning
      "🛠️": suggestion
      "💡": nit

  - name: nitpick_section
    pattern: '<details>.*?Nitpick'
    severity: nit

  - name: explicit_severity_field
    pattern: '\*\*Severity:\*\*\s+(?P<level>\w+)'
    severity_field: level
```

**Acceptance.**

- Rules can match on author, body regex, file path, or combinations.
- Each rule produces severity + category + a confidence score.
- Multiple rules can match; highest-confidence wins, with deterministic tie-breaking.
- A "show classification reason" tooltip shows which rule fired.
- Users can author their own rules under `~/.config/bistro/rules/` and they hot-reload.

### 3.4 Severity heuristics for unlabeled comments

Used when a comment doesn't match a vendor-specific rule (humans, unknown bots, bots without severity hints).

**Heuristic library.**

- **Nit signals.** "consider," "you could," "minor," "nit:", "nitpick:", "stylistic," "preference," "personal taste," "would be cleaner" → severity `nit`.
- **Critical signals.** "SQL injection," "XSS," "credential," "secret," "password," "auth bypass," "memory leak," "race condition" + path matches `*/auth/*`, `*/security/*`, `*/payments/*` → severity `critical`.
- **Style signals.** Path matches `*.md`, `*.rst`, comment is purely about formatting → category `style`.
- Heuristics are weighted; final severity is the max of all triggered weights.

**Acceptance.**

- 70%+ accuracy on a held-out set of real review comments (we'll build the test corpus from public PRs in popular OSS repos before v1.0).
- Every heuristic is documented with examples in `docs/classification.md`.

### 3.5 Bulk comment operations

**User story.**

> As a senior reviewer triaging a PR with 47 comments, I want a single keystroke to dismiss all nits, all comments from one bot, or all comments below a severity threshold, so that I can get to the 3 comments that actually matter.

**Acceptance.**

- `dN` — dismiss all nits in this PR (mark resolved).
- `dB <vendor>` — dismiss all comments from one bot.
- `f c` / `f w` / `f s` / `f n` — filter to critical / warning / suggestion / nit.
- `f h` — show only human comments.
- All bulk operations are reversible via `u` (undo) within the session.
- Dismissals from FOSS are local-only; they don't write back to the provider unless the user explicitly enables that in settings (per-account opt-in).

### 3.6 Multi-bot deduplication

**Why it matters.** When a team has CodeRabbit + Greptile + Copilot all running, they routinely flag the same line with overlapping comments. Today the user reads three near-identical paragraphs.

**Acceptance.**

- Comments on the same `(file_path, line_range)` with text similarity > 0.7 (Jaccard or trigram) get a shared `dedup_group_id`.
- The UI renders one merged finding with provenance: "Flagged by CodeRabbit + Greptile + Copilot (3 sources)."
- Each source is expandable individually.
- The most severe rating in the group becomes the group's severity.

### 3.7 PR spam / slop detection

**Why it matters.** OSS maintainers in 2025 reported being overwhelmed by AI-generated PRs that look plausible but are slop. curl shut down its bug bounty for this reason.

**Acceptance.**

- A PR-level "slop score" (0–100) computed from:
  - Generic phrases in description ("This PR improves the code quality," "I have made the following changes")
  - Description-to-diff length mismatch
  - Generated-file overlap
  - First-time contributor + account-age < 30 days
  - Commit message pattern (single-line generic, "Update foo.py", etc.)
- Slop score is shown as a sidebar indicator on each PR; doesn't auto-hide anything.
- Filter `f !` — hide PRs with slop score > 70.

### 3.8 BYOK (Bring Your Own Key) AI

**Acceptance.**

- Settings page: "AI Classification (Optional)" section.
- Provider dropdown: Anthropic, OpenAI, OpenRouter, Ollama (any OpenAI-compatible endpoint).
- API key stored in OS keychain.
- Per-task toggle: which classification jobs to send to LLM (severity, category, dedup, slop, all, none).
- All LLM calls go from the local Go process directly to the provider's API. No traffic through Bistro infrastructure (FOSS doesn't have any).
- Token-usage display in the UI: "This session: 14,230 tokens, est. $0.18."
- Aggressive cache: same comment text → same classification, never re-classify.
- Off by default. Opt-in only. Privacy-conscious users get full functionality with regex-only.

---

## Phase 4 — Local Expertise Scoring (the killer feature, FOSS layer)

The Layer 1 of expertise routing. Computes per-file expertise from local git history. Layer 2 (team-wide expertise graph, departed-employee handling, write-back to provider) is Team-only.

### 4.1 Expertise score per file per author

**Algorithm.** A weighted sum of:

1. **Surviving lines** (high weight, no decay). `git blame HEAD` aggregated by author. Who wrote the code that's still in the file today?
2. **Substantial commits in last 12 months** (medium weight, recency-decayed with ~6mo half-life). "Substantial" = changed > 5 lines, not a typo fix.
3. **Original authorship** (medium weight, slow decay). Who first created the file? `git log --follow --diff-filter=A`.
4. **Prior reviewer of this file** (low-medium weight, recency-decayed). Who has reviewed PRs that touched this file?

**Recency cutoff.** Only include authors who have *any* commit or review in the watched repos within the last 90 days. This catches departed contributors at FOSS level (without org context). Authors who fail this cutoff are flagged `dormant` — still recorded, but excluded from default suggestions.

**Acceptance.**

- Initial computation runs once per repo on watch (can take a few minutes on a large repo; show progress).
- Incremental updates on each new commit synced.
- Stored in `file_expertise(repo_id, path, author, score, last_computed_at, dormant)`.
- A "why" view per file: "Alice: 312/480 surviving lines, original author, 4 reviews in last 6 months → score 87."

### 4.2 "PRs touching files you've worked on" view

**User story.**

> As a senior dev, I want Bistro to show me PRs touching files I have meaningful history with, even if I wasn't requested as a reviewer, so that changes to code I care about don't slip past me.

**Acceptance.**

- A new view in the PR list: "Touches Your Code (N)."
- A PR appears here if any file it touches has the current user in the top 3 expertise scores for that file.
- Each row shows which files are "yours" with the expertise score.
- Default behaviour is *FYI in the local inbox only*. No external notifications. No write-back to provider. The user can promote one to a real reviewer-request manually.

### 4.3 Configuration

**Acceptance.**

- `.bistro/expertise.yml` per repo, optional, committable to the repo:

  ```yaml
  exclude:
    - "vendor/**"
    - "**/*.pb.go"
    - "dist/**"
  pin:
    - path: "auth/*"
      owners: ["@alice", "@bob"]
  ```

- Respects `.gitattributes linguist-generated=true`.
- A global setting for "minimum surviving lines to count as expert" (default 50).

---

## Phase 5 — Inbox & Triage Polish

By this point Bistro is feature-rich. Phase 5 makes it *fast to live in*.

### 5.1 Reviewer Inbox

A unified, prioritised landing screen.

**Acceptance.**

- Inbox lanes: `Urgent` (changes requested on your PR, or you're requested and PR is > 24h old), `Needs Your Action`, `Touches Your Code`, `Authored by You`, `Mentioned`, `FYI`.
- Single keystroke moves between lanes (`g 1` … `g 6`).
- Per-lane keyboard navigation (`j/k`).
- Lane counts in the sidebar at all times.
- "Mark all read in lane" with `Shift-R`.

### 5.2 Diff polish

| Feature | Status | Notes |
|---|---|---|
| Side-by-side diff toggle | not started | |
| Word-level diff within changed lines | not started | |
| Whitespace-only-change collapse | not started | One keystroke to hide |
| Generated-file collapse | not started | Auto + manual |
| Image/binary diff handling | not started | Side-by-side image render for `.png/.jpg/.svg` |

### 5.3 Command palette polish

| Command | Status | Notes |
|---|---|---|
| `:sync` (force sync now) | done (2026-06-12) | Sends FORCE_SYNC; footer shows SYNCING until SYNC_COMPLETE |
| `:settings`, `:about`, `:help` | done | Also implemented: `:clear` (clear filters/search), `:reset-store` |
| `:focus list/detail` | done | |
| `:open in browser` | partial | `o` keybinding (`list.openBrowser`) + `openExternal` IPC already exist; palette command missing |
| `:open in editor` | not started | Opens current file in $EDITOR at the right line |

### 5.4 Help / discoverability

| Item | Status | Notes |
|---|---|---|
| `?` overlay with all bindings | done | `HelpOverlay.tsx` |
| First-run tour | not started | 5-screen guided intro |
| "Did you know?" tooltips | not started | Surface lesser-known bindings contextually |

---

## Phase 6 — GitLab Provider

The first non-GitHub provider. Validates the abstraction.

### 6.1 Provider boundary audit (do this first)

Before writing GitLab code, sweep the existing codebase for GitHub-specific assumptions that have leaked past the provider interface. This is the highest-leverage week of engineering in the entire FOSS roadmap — refactor cost scales with the number of providers, and we want to do it once, while the count is one.

Known leaks as of the 2026-06-10 audit: `go/internal/auth/callback.go`, `go/internal/server/handler/{auth,diff,repo}.go` reference GitHub directly; `rate_limit_state.id` defaults to `'github'`.

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
- Migrate `rate_limit_state` off its `'github'` default id.

---

## Phase 7 — v1.0 Release Prep

A focused phase on quality, docs, distribution.

### 7.1 Quality gates

- Bistro reviews its own PRs (eat dog food).
- Performance budget: 3000-file PR opens < 2s to first paint.
- Memory budget: < 1GB on a 10000-line diff.
- Crash-free sessions: > 99.5% across last 7 days of internal use.

### 7.2 Distribution

- macOS DMG (signed + notarised).
- Windows MSI (signed).
- Linux AppImage and `.deb`.
- Auto-update via the standard Electron updater.
- Homebrew cask formula.

### 7.3 Documentation

- `README.md` with the value-prop framing (see Communication Plan doc).
- `docs/install.md`, `docs/keybindings.md`, `docs/classification.md`, `docs/expertise.md`, `docs/byok.md`.
- A 90-second screencast on the landing page.
- `docs/architecture.md` for contributors (the existing architecture writeup, condensed).
- `CONTRIBUTING.md` and a clear scope statement: *what we will and won't merge.*

### 7.4 Telemetry

- Off by default. Opt-in only.
- If on: anonymised crash reports + feature-usage counts (no PR content, no repo names, no usernames).
- Settings page surfaces exactly what's sent.

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

1. **`@pierre/diffs` license.** Verify before public release.
2. **Bot list update mechanism.** Is the rules YAML bundled-and-updated-with-the-binary, or fetched at runtime from a curated repo? (Latter is more flexible but adds a network dependency. Suggest: bundled with optional fetch.)
3. **Telemetry stance.** Final call on opt-in / opt-out / none-at-all.
4. **Crash reporter.** Sentry? Self-hosted? None?
5. **Auto-update server.** Self-hosted via static files, or a service?
6. **License for the FOSS code.** AGPL? MIT? Apache 2? (Affects whether Team plan can build on it without a CLA — see Communication Plan.)
