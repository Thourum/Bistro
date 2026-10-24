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
> when local data is insufficient (see the [current design](go-project-overview.md#internalsync----sync-engine)). Complete push capture remains Team/server scope. (The audit record
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
> degradation gap is cleared (see the [current design](go-project-overview.md#internalsync----sync-engine)). Posting a
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

## Phase 0 — Foundation (status: largely done)

This is the existing architecture. Most of it ships in pre-v1 builds. Listed here for completeness and to flag what still needs hardening before v1.0.

### Architecture in place

| Feature | Status | Notes |
|---|---|---|
| Two-process design (Go sync + Electron UI) | done | `go/cmd/bistro-sync` + `electron/src/main/go-spawn.ts`; stable enough to build on |
| SQLite-as-cache, Go-owns-writes | done | `go/internal/store`, migrations 000–011 |
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
| `pr_diffs` table size budget | done (2026-07-11) | BIS-25: diff budgets enforced, partial manifest persisted with per-file metadata + diff_status (migrations 024/025, `git/diff.go`, `sync/fetcher.go`) |
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
| Approve / request changes / PR-level comment | done | |
| **Line-level / inline review comment** | partial (2026-07-11) | Inline threads render, and **replies to existing threads post end-to-end** (`CreateCommentInReplyTo`, reply UI in diff view). Posting a *new* line-level comment is still rejected when `line > 0` — provider lacks `path`/`commitSHA`/`side` posting |
| `a a` double-tap confirmation | done | `keyboard.ts` |
| Exponential backoff retry | done | `actions/executor.go` |
| Offline action queue with replay | done | ⚠️ Audit (FIXED 2026-06-19): re-enqueuing the same `(PR, action)` hit a `PRIMARY KEY` collision and silently dropped the retry. `Enqueue` now uses `ON CONFLICT(id) DO UPDATE … status='pending', retry_count=0` |
| **Resolve / unresolve conversation** | done (2026-07-11) | GraphQL client landed (`provider/github/graphql.go` — `resolveReviewThread`/`unresolveReviewThread`), wired through action handlers and `useThreadActions` + `UnresolvedThreadsView` |
| **Reviewer-request write-back** | not started | Required for Phase 4 expertise routing to be useful. ⚠️ Audit: wider blast radius than implied — needs a new `Provider.RequestReviewers` method (breaks the mock + all interface assertions), a new `ActionCommand`, handler, and WS message. go-github v84 *does* expose `PullRequests.RequestReviewers`, so the GitHub impl is feasible |
| **Idempotency keys on retried actions** | done (2026-07-11) | Stable action IDs in `store/action_queue.go`; replay path in the orchestrator dedupes on them |

### 1.4 Sync engine

5-min periodic sync, event-triggered sync via `syncTrigger`, concurrent per-PR fan-out, 429 handling that flips into queue mode, in-place query cache merging.

| Sub-feature | Status | Notes |
|---|---|---|
| Periodic + event-triggered sync | done | `server/syncloop.go` |
| Worker-pool fan-out | done | `sync/workerpool.go` |
| 429 detection and queue-mode | done | covered by orchestrator integration tests |
| In-place TanStack Query merging | done | |
| **Per-currently-open-PR fast sync** | not started | When a PR is open in the UI, sync it every 30–60s; everything else stays at 5m. ⚠️ Audit: no single-PR code path exists — `SyncRequest` has no PR field and the orchestrator always calls `ListPRs`. Needs: (1) a `PRNumber`/`PRID` on `SyncRequest` + a `GetPR`-based path in the orchestrator, (2) a WS message so the renderer signals which PR is open (the engine has no renderer-state awareness), (3) cost control — a per-PR details fetch is 4–5 API calls (`GetCIStatus` redundantly re-fetches the PR for the head SHA); floor the interval and fix that. Also: clone URL is hardcoded to `github.com` (`fetcher.go:149`) |
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
| **PR description / labels / assignees / reviewers changed** | partial (2026-07-11) | Writer now diffs before/after and emits `PR_METADATA_CHANGED` — but only for state/title/draft. Description, labels, assignees, reviewers still uncovered |
| **CI status changed** | done (2026-07-11) | `CI_STATUS_CHANGED` emitted from the writer path, consumed in `services/ws/events/review.ts` |
| **Conversation resolved/reopened** | partial (2026-07-11) | Events fire when *this client* resolves/unresolves (local actions); remote resolutions by others are only picked up on the next sync, with no event |

### 1.6 "Needs my action" vs "FYI" classification — **superseded by Phase 5's six-lane inbox (done 2026-08-09)**

**Why it matters.** The single highest-frequency complaint about GitHub notifications in the research was the lack of priority signal — every email is the same urgency. Computed locally from review-requested + mentioned + change-requested + author-vs-reviewer + draft state.

**User story.**

> As a developer who's on the reviewer list of 30 active PRs, I want my PR list to clearly separate the ones that are *waiting on me right now* from the ones that are *FYI*, so that I can act on the urgent ones without scrolling past the noise.

**As-built.** The three/four-lane sketch below was superseded by the revised
six-lane taxonomy (Needs Action, Waiting on Author, Your PRs, Your Code,
Mentioned, FYI) approved during Phase 5 design and shipped 2026-08-09. See
`docs/DESIGN.md` "Default lanes" and §5.1 above for the shipped rules.

- Lane counts render as section headers, always visible (not a separate sidebar counter).
- `g 1`…`g 6` jump directly to a lane.
- Default `prList.view` is `'flat'` (lane-sectioned, Needs Action first), not a `filterState` default — same practical effect.
- The mentions rule **is implemented**: migration 033 added `prs.mentioned`, computed at sync time from the PR body and comments (resolves the 2026-06-19 audit finding below).

<details>
<summary>Original draft (superseded, kept for history)</summary>

**Acceptance.**

- A persistent counter in the sidebar shows `Needs Action (N) / Waiting on Author (M) / FYI (K)`.
- Single-key filter to each.
- Default view is "Needs Action."
- Logic: review requested + not yet reviewed → Needs Action. Author + change requested → Needs Action. Approved + waiting for merge → Waiting on Author. Mentioned but not requested → FYI. Draft → FYI.

> ⚠️ **Audit (2026-06-19).** The "**Mentioned but not requested → FYI**" rule is **unimplementable from synced data**: there is no mentions field on the PR model or schema, and GitHub's REST API exposes `@mentions` only inside raw comment body text. `triage.ts` silently classifies every unmatched PR as FYI without checking mentions. Either drop the rule (it degrades gracefully) or add a post-sync body scan + a `mentions_me` column + a triage check. Separately, switching the default view to "Needs Action" requires changing the filter default and ensuring the renderer fetches enough PRs before client-side lane filtering.

</details>

---

## Phase 2 — Patchset Semantics (the architectural unlock) — **status: largely done (updated 2026-10-23)**

> ✅ **Built (verified 2026-07-11):** patchsets table + observed-snapshot capture +
> timeline UI with diff-pair selector (2.1); last-reviewed identity recorded on review
> submission + `s` since-last-review view (2.2); `files_viewed` keyed by patch
> content hash — survives rebase (2.2); inline review comments fetched, anchor columns
> (migrations 015–020, 023), re-anchoring backend (2.3); unresolved-threads `gu`
> overlay with Open/Resolved/Outdated lifecycle, GraphQL resolve/unresolve, and a
> merge-readiness badge (2.4); comment replies; current-user identity + per-account
> namespacing (migration 022). **Updated 2026-10-23:** merge-base PR diffs,
> observed-head pins, patch-ID file triage on `s`, and per-file interdiff on `i`
> have shipped ([design](go-project-overview.md#internalsync----sync-engine)).
> Viewed marks now use rebase-stable patch fingerprints (pre-upgrade marks reset
> once). **Remaining gap:** comment-level outdated-near-line display isn't wired
> as a distinct UI state (thread-level lifecycle is).

This is where Bistro starts having features that GitHub structurally cannot match, within an honest FOSS boundary: Bistro can reason over locally observed patchsets quickly, but it cannot claim server-grade capture of pushes it never observed.

The architectural principle: **same patchset model for every provider**. Even where a provider exposes patchsets natively (GitLab), FOSS uses its own observed local model so behavior and storage stay consistent. Provider/webhook-enriched complete history belongs in `docs/STAGE-2-TEAM.md`.

> ✅ **BIS-31 comparison strategy superseded (2026-10-23).** FOSS still records
> observed patchsets, not complete push history. Since-last-review now compares
> per-file patch IDs, with go-git interdiffs between retained heads on demand.
> Missing review baselines, pre-fingerprint patchsets, and unavailable heads have
> explicit reasons rather than claiming a complete comparison. The old
> `git range-diff` / bounded `os/exec` path has been removed. Current design:
> [since-last-review across rebases and force-pushes](go-project-overview.md#internalsync----sync-engine).

> ⚠️ **Audit (2026-06-19) — Phase 2 needs design decisions before any implementation.**
> This phase as originally written contained two *infeasible-as-specified* items and
> rests on a comment layer that can't yet feed it. BIS-31 resolves the changed-since-
> review/patchset scope by narrowing FOSS promises to observed patchsets and labeled
> degraded modes; remaining comment-layer prerequisites still need implementation:
> 1. **Patchset capture by polling is lossy** (2.1). FOSS records observed patchsets,
>    not every push. `pushed_at` is the best available observed timestamp in FOSS;
>    true push timestamps/complete before-after SHAs require the Team webhook server.
>    **Comparison decision superseded (2026-10-23):** patch-ID triage and go-git
>    per-file interdiffs replace the former bounded subprocess exception; see the
>    [current design](go-project-overview.md#internalsync----sync-engine).
> 3. **The comment layer can't feed 2.3/2.4.** Only *issue-level* comments are fetched
>    (`Issues.ListComments`), never inline review comments (`PullRequests.ListComments`),
>    and the `comments` schema stores no `commit_sha`/`position`/`side`/`resolved`/
>    `thread_id`. Inline posting also doesn't work (see 1.3). These are prerequisites,
>    not Phase 2 work.
> 4. **Prerequisite migrations nobody scheduled:** a SQLite-resident current-user
>    identity (2.2 `user_id`, all of Phase 4), `files_viewed` state (2.2), a
>    `patchset_number` column on `comments` (2.3). See the audit doc.
> Full detail with `file:line` evidence: `docs/ROADMAP-AUDIT-2026-06-19.md`
> (removed 2026-07-11; in git history).

### 2.1 Patchset timeline — done (updated 2026-10-23)

> Shipped: observed patchset capture, `GET_PATCHSETS`, and `PatchsetTimeline.tsx`
> with diff-pair selection. The pair picker now drives patch-ID file triage and
> per-file go-git interdiffs. Merge-base diffs and pinned heads retain meaningful
> comparisons across observed rebases and force-pushes; unavailable heads say so.

**Why it matters.** The most-cited reason developers say they "miss" Gerrit is the patchset model — pushes become reviewable snapshots, comments can be anchored to snapshots, and reviewers can compare iterations. GitHub's "Changes since last review" button breaks under rebase ([daisy.wtf](https://daisy.wtf/writing/github-changes-since-last-review/), [community/3478](https://github.com/orgs/community/discussions/3478)).

**User story.**

> As a reviewer of a PR that Bistro has observed across multiple force-pushes, I want to see those observed snapshots as a numbered timeline and diff safe pairs, so that I can audit what changed across iterations without losing my place.

**Acceptance.**

- A patchset is recorded when the observed PR head or a known merge base changes, not when only the base tip moves. It stores a sequential number, head SHA, base SHA, `merge_base_sha`, and best-available observed timestamp.
- Per-file `patch_id`s are stored in `patchset_files`; `has_fingerprints` distinguishes a recorded fingerprint snapshot from a pre-feature or pending snapshot.
- Observed patchsets persist after later rebases/force-pushes if Bistro has already stored them; FOSS does not promise to recover intermediate heads it never observed. Sync pins observed heads by SHA, and cache clones refresh in place.
- A timeline UI in the PR detail shows P1 / P2 / P3 as **observed patchsets** with a diff-pair selector.
- Comparing a pair triages files by patch ID and offers per-file go-git interdiffs on `i`. If a head is unavailable locally, the interdiff shows an explicit reason.

**Schema.**

```sql
-- ⚠️ Audit: pr_id must be TEXT, not INTEGER — prs.id is TEXT (e.g. 'github:owner/repo#1').
-- An INTEGER FK never matches; ON DELETE CASCADE never fires; JOINs return zero rows.
-- Mirror the pattern in 007_add_pr_diffs.sql. Corrected DDL:
CREATE TABLE patchsets (
  pr_id TEXT NOT NULL,
  number INTEGER NOT NULL,
  head_sha TEXT NOT NULL,
  base_sha TEXT NOT NULL,
  pushed_at TIMESTAMP NOT NULL, -- FOSS: observed timestamp; Team/webhook mode: true push timestamp
  PRIMARY KEY (pr_id, number),
  FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);
-- ⚠️ Audit: also unscheduled but required here — add patchset_number INTEGER to the
-- comments table (which patchset a comment was made against) so 2.3 can detect "outdated".
```

### 2.2 "Since I last reviewed" — done (updated 2026-10-23)

> Shipped: rebase-stable patch-ID file triage on `s`, a status-bar summary,
> per-file go-git interdiff on `i`, and `v` viewed marks keyed by the same patch
> fingerprint. Baselines include GitHub reviews from any client, Bistro review
> actions, and marking all files viewed. The `s` falls-back-after-force-push
> release blocker is cleared; missing comparison data is reported explicitly.

**Why it matters.** GitHub's button works only if commits aren't rebased. As AI agents increasingly force-push every iteration, this gets worse, not better.

**User story.**

> As a reviewer returning to a PR after the author force-pushed, I want a "since my last review" view that shows me only what genuinely changed since I approved, so that I don't have to re-read 800 lines I already signed off on.

**Acceptance.**

- The baseline per `(pr_id, reviewer_login)` is the newest of a submitted GitHub review (from any client), a Bistro approve/request-changes/top-level comment (not a reply), or a mark-viewed action leaving no file unviewed. It records the reviewed head, patchset number when known, timestamp, and source.
- Triage compares per-file patch IDs between the baseline and target patchsets, classifying files as changed, new, removed, or unchanged. `s` filters the current PR diff to changed/new files, not to a replacement whole-PR diff; removed paths are listed in a separate notice.
- The status bar summarizes the changed/total file count and baseline. No comparable prior review, no observed patchsets, or missing fingerprints produce an explicit message; they do not masquerade as a successful comparison.
- `i` toggles an interdiff for the current file between the two patchset heads. A `rebased: includes upstream changes` label warns when that file differs between the merge bases (or recorded merge bases cannot be read); missing heads show an unavailable reason.
- "Files viewed" state survives line-shift-only rebases when the normalized patch fingerprint is unchanged. Marks made before the upgrade reset once.

> ✅ **BIS-31 update superseded (2026-10-23).** `reviewer_login`/current-user identity
> remains the review key. `last_reviewed_sha` now stores review source and timestamp
> as well as the head and known observed patchset. `files_viewed` uses file path +
> login + normalized patch fingerprint. The former subprocess comparison policy
> is replaced by [patch-ID triage and go-git interdiffs](go-project-overview.md#internalsync----sync-engine).

### 2.3 Comment anchoring across rebase — mostly done (2026-07-11)

> Shipped: inline review comments are fetched and stored with position fields
> (migrations 015/018/019/020/023), re-anchoring runs in `git/anchor.go`, and the
> diff view consumes thread lifecycle/line annotations. Remaining: a distinct
> "outdated, shown at nearest surviving line" *comment-level* display.

**Why it matters.** The "where did my comment go?" thread on github/community has been growing for 7 years.

**Acceptance.**

- Comments are anchored to `(file_path, line_content_hash, surrounding_context)` in addition to line number.
- After rebase, comments resolve to their new line numbers if the content survived.
- If the content was deleted, the comment is shown as "outdated" but remains visible inline at its nearest surviving line, not banished to a sidebar.

> ⚠️ **Audit (2026-06-19).** Blocked on missing inputs: inline review comments are
> **never fetched** (provider uses `Issues.ListComments`, not `PullRequests.ListComments`),
> and the schema stores **no `commit_sha`/`position`/`side`/`patchset_number`** — the
> anchoring algorithm has nothing to work with. Posting an inline comment back also doesn't
> work yet (see 1.3). Prereq: fetch + store `PullRequestComment` fields and add the anchoring
> columns before this is buildable. (Note: the *display-only re-anchoring* concept is sound —
> a verifier confirmed 2.3 makes no false round-trip claim; the gap is purely the missing data.)

### 2.4 Canonical "all unresolved threads" view — done (2026-07-11)

> Shipped: `gu` overlay (`UnresolvedThreadsView.tsx`) with Open/Resolved/Outdated
> lifecycle, GraphQL resolve/unresolve write-back, and `MergeReadinessBadge`.

**Why it matters.** Free win from our SQLite cache — GitHub structurally cannot show every unresolved thread in one pane because of how their UI virtualises the timeline.

> ⚠️ **Audit (2026-06-19).** The "free win" framing is misleading: **the SQLite cache is
> missing the data this view needs.** "Unresolved" + "Open/Resolved/Outdated lifecycle" +
> "merge-readiness" all require thread-resolution state, which (a) has no `resolved`/`thread_id`
> column today and (b) is **GitHub-GraphQL-only** (see 1.3). 2.4 is therefore **blocked on
> unbuilt 1.3 (resolve/unresolve) and 1.5 (resolved/reopened events)** — sequence those first.
> Mark 2.4 "blocked on 1.3, 1.5."

**Acceptance.**

- A dedicated view (keystroke `gu` for "go unresolved") lists every unresolved thread in the current PR, including threads on outdated lines.
- Each thread shows lifecycle state (Open / Resolved / Outdated) explicitly.
- Browser-style find-in-page works across all threads, including collapsed ones.
- A merge-readiness check surfaces every open thread.

---

## Phase 3 — AI Comment Classification (the wedge) — **status: implemented 2026-07-12 (3.7 deferred to Team plan)**

> **Implemented 2026-07-12** (PR #22, branch `P3-Comment-Classification`) —
> sub-features 3.1–3.6 + 3.8; **3.7 deferred to the Team plan** (see below).
> Authoritative docs:
> - Spec: `docs/superpowers/specs/2026-07-12-comment-classification-design.md`
> - Plan: `docs/superpowers/plans/2026-07-12-comment-classification.md`
> - User guide: `website/src/pages/docs/comment-classification.mdx`
>
> (History: a 2026-07-11 audit found zero 3.x code; the 2026-06-19 foundation
> spec/plan were recovered from git history and superseded by the docs above.)

This is what makes Bistro the answer to the AI-PR era. Implemented as a rules engine + heuristics, with optional BYOK to push specific tasks to a user-supplied LLM.

**Design principle.** Every classification is deterministic, inspectable, and explainable. The user can always click a comment and see *why* it was tagged as "nit" — the matching rule, the regex, the heuristic.

> **As-built decisions** (where the implementation diverges from the original
> draft below — the draft is left intact for history; the spec is authoritative):
> - **Single engine.** Heuristics (3.4) are not a separate code path — they ship as a
>   built-in YAML (`zz-fallback.yml`) loaded last. The rule schema gained `weight` for
>   weighted heuristic scoring; vendor rules set severity directly.
> - **Resolution is last-match-wins**, not "highest confidence wins." Files load
>   alphabetically; `coderabbit.company.yml` overrides `coderabbit.yml` by loading later.
>   No confidence score in v1 (deferred until conflicts are observed in practice).
> - **No hot-reload / fsnotify.** A coarse `sha256` fingerprint over the rules dir
>   (+ classification mode) is stored per comment; comments re-classify lazily on
>   PR-open when the fingerprint is stale.
> - **Rule distribution = bundled + app auto-update.** Built-ins are `go:embed`-ed,
>   seeded to `~/.config/bistro/rules/` on first launch, then manifest-gated-updated
>   on app upgrade (`.seeded-builtins` tracks what shipped: present built-ins are
>   overwritten, new built-ins arrive, user deletions stay deleted, user files are
>   never touched). No runtime fetch.
> - **Per-PR counter lives in the sidebar** (3.2), not a per-PR header. The overview
>   reorders comments by severity and auto-collapses nits; the diff view shows inline
>   severity markers. Built-in vendors shipped: CodeRabbit, Greptile, fallback
>   heuristics (remaining vendors added as comment samples are collected).
> - **Corpus gate:** 50-comment fixture with a ≥70% heuristic-accuracy CI gate;
>   shipped built-ins score 90%.
> - **BYOK per-task toggles collapsed to one mode setting** (`rules` /
>   `rules+llm` / `llm-all`; `server` is a hidden Team-plan stub) — severity+category
>   are the only LLM jobs in v1, so per-job toggles had nothing to toggle. Official
>   vendor SDKs (`openai-go`, `anthropic-sdk-go`) replace hand-rolled HTTP.

### 3.1 Schema additions — **done (migrations 027–029)**

```sql
ALTER TABLE comments ADD COLUMN author_type TEXT;       -- 'human' | 'bot' | 'unknown'
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;        -- 'coderabbit' | 'greptile' | 'copilot' | …
ALTER TABLE comments ADD COLUMN severity TEXT;          -- 'critical' | 'warning' | 'suggestion' | 'nit' | 'info'
ALTER TABLE comments ADD COLUMN category TEXT;          -- 'security' | 'correctness' | 'performance' | 'style' | 'readability' | 'docs'
ALTER TABLE comments ADD COLUMN classification_source TEXT; -- 'rule:<name>' | 'byok:<provider>' | 'manual'
ALTER TABLE comments ADD COLUMN dedup_group_id TEXT;    -- comments in the same group are duplicates of each other
```

### 3.2 Bot identification — **done**

> As-built: CodeRabbit + Greptile ship as built-in rule files; the remaining
> vendors (Copilot, Qodo, Cursor BugBot, Claude Code, Codex, Sourcery) are
> added as one YAML each when real comment samples are collected. Unknown
> `[bot]` logins classify as `bot`/`unknown`. The counter lives in the sidebar.

**Acceptance.**

- Curated vendor list shipped as data (YAML), updated independently of the binary.
- Top vendors covered at v1.0: CodeRabbit, Greptile, Copilot Code Review, Qodo, Cursor BugBot, Claude Code, Codex, Sourcery.
- Unknown bots (author login ends in `[bot]` or matches heuristics) flagged as `bot:unknown`.
- Per-PR header counter: `Human: 4 · CodeRabbit: 23 · Greptile: 8 · Other bots: 2`.

### 3.3 Severity classification (rules engine) — **done**

> As-built: no confidence score (last-match-wins + weighted tally instead;
> deferred until real conflicts appear), and "hot-reload" is lazy — rule
> edits apply on next PR open via the fingerprint, not via a file watcher.

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

### 3.4 Severity heuristics for unlabeled comments — **done (90% on corpus, gate ≥70%)**

Used when a comment doesn't match a vendor-specific rule (humans, unknown bots, bots without severity hints).

**Heuristic library.**

- **Nit signals.** "consider," "you could," "minor," "nit:", "nitpick:", "stylistic," "preference," "personal taste," "would be cleaner" → severity `nit`.
- **Critical signals.** "SQL injection," "XSS," "credential," "secret," "password," "auth bypass," "memory leak," "race condition" + path matches `*/auth/*`, `*/security/*`, `*/payments/*` → severity `critical`.
- **Style signals.** Path matches `*.md`, `*.rst`, comment is purely about formatting → category `style`.
- Heuristics are weighted; final severity is the max of all triggered weights.

**Acceptance.**

- 70%+ accuracy on a held-out set of real review comments (we'll build the test corpus from public PRs in popular OSS repos before v1.0).
- Every heuristic is documented with examples in `docs/classification.md`.

### 3.5 Bulk comment operations — **done**

**User story.**

> As a senior reviewer triaging a PR with 47 comments, I want a single keystroke to dismiss all nits, all comments from one bot, or all comments below a severity threshold, so that I can get to the 3 comments that actually matter.

**Acceptance.**

- `d n` — dismiss all nits (+info) in this PR. *(shipped)*
- `d b` — dismiss all bot comments (per-vendor picker deferred; selection helper already supports it). *(shipped)*
- `f c` / `f w` / `f s` / `f n` — filter to critical / warning / suggestion / nit.
- `f h` — show only human comments.
- All bulk operations are reversible via `u` (undo) within the session.
- Dismissals from FOSS are local-only; they don't write back to the provider unless the user explicitly enables that in settings (per-account opt-in).

### 3.6 Multi-bot deduplication — **done**

**Why it matters.** When a team has CodeRabbit + Greptile + Copilot all running, they routinely flag the same line with overlapping comments. Today the user reads three near-identical paragraphs.

**Acceptance.**

- Comments on the same `(file_path, line_range)` with text similarity > 0.7 (Jaccard or trigram) get a shared `dedup_group_id`.
- The UI renders one merged finding with provenance: "Flagged by CodeRabbit + Greptile + Copilot (3 sources)."
- Each source is expandable individually.
- The most severe rating in the group becomes the group's severity.

### 3.7 PR spam / slop detection — **deferred to Team plan (2026-07-12)**

> **Deferred (2026-07-12).** Two of the five signals need data FOSS doesn't
> sync: commit messages (no commits table) and contributor account age (author
> account created-date isn't fetched). Rather than adding sync surface for a
> single feature, slop scoring moves to the Team plan where it can be computed
> server-side with complete data. The acceptance below is kept for that scope.

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

### 3.8 BYOK (Bring Your Own Key) AI — **done**

**Acceptance.**

- Settings page: "AI Classification (Optional)" section.
- Provider dropdown: Anthropic, OpenAI, OpenRouter, Ollama (any OpenAI-compatible endpoint).
- API key stored in OS keychain.
- ~~Per-task toggle~~ As-built: one mode setting (rules / rules+llm / llm-all) — severity+category are the only LLM jobs in v1.
- All LLM calls go from the local Go process directly to the provider's API. No traffic through Bistro infrastructure (FOSS doesn't have any).
- Token-usage display in the UI: "This session: 14,230 tokens, est. $0.18."
- Aggressive cache: same comment text → same classification, never re-classify.
- Off by default. Opt-in only. Privacy-conscious users get full functionality with regex-only.

---

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

## Phase 5 — Inbox & Triage Polish — **status: done (2026-08-09)**

By this point Bistro is feature-rich. Phase 5 makes it *fast to live in*.

> **Shipped early, off-roadmap (2026-06-24/25, verified 2026-07-11):**
> - **Markdown rendering** of PR descriptions and comments — sanitized
>   ReactMarkdown/GFM (`Markdown.tsx`, `markdownComponents.tsx`) incl. fenced-code
>   highlighting and **Mermaid diagrams** with strict security + size/concurrency
>   bounds (`MermaidDiagram.tsx`).
> - **Bistro design-system primitives** — restyled shadcn-style components under
>   `components/ui/` plus an integrated draggable title bar (`AppTitleBar.tsx`).

> **Implemented 2026-07-18 through 2026-08-09.** Authoritative docs (the
> original plan/spec were removed once the work shipped):
> - Lane rules and UI spec: `docs/DESIGN.md` (lane classifier lives in
>   `electron/src/renderer/utils/lanes.ts`)
> - CodeView migration spike notes: `docs/superpowers/plans/2026-07-18-codeview-spike-notes.md`
> - Shortcut reference: `docs/keyboard-shortcuts.md`
>
> **As-built deviations from the original draft below** (draft left intact for
> history; the spec is authoritative):
> - **Six-lane taxonomy**, not the four-lane sketch this section originally
>   described: Needs Action, Waiting on Author, Your PRs, Your Code, Mentioned,
>   FYI. See `docs/DESIGN.md` "Default lanes" for the shipped rules.
> - **Diff surface migrated to `@pierre/diffs` `CodeView`** (replacing the
>   hand-rolled TanStack-Virtual-around-`PatchDiff` component) before the
>   polish features landed — most of 5.2 became configuration afterward.
> - **Navigation remapping (post-implementation correction):** the diff route's
>   `j`/`k` now always scroll; `Shift+J`/`Shift+K` navigate files;
>   `n`/`p` navigate inline comments. PR-detail mode keeps `n`/`p` for file
>   navigation (it has no inline-comment view). This replaced an earlier
>   mapping where `j`/`k` tried comment navigation first and silently broke
>   scrolling on any file with threads.
> - **Image/binary diff rendering deferred** — a placeholder row
>   (`binary file — o opens in browser`) ships instead; full image diff is a
>   follow-up.
> - **React component tests excluded by user directive (2026-07-18).** Only
>   pure utilities, hooks, and non-trivial behavior are unit-tested; component
>   surfaces (lane headers, tour screens, dialogs) are verified manually.

### 5.1 Reviewer Inbox — done

**Acceptance.**

- Six-lane taxonomy: Needs Action, Waiting on Author, Your PRs, Your Code, Mentioned, FYI (see `docs/DESIGN.md`).
- Single keystroke moves between lanes (`g 1` … `g 6`).
- Per-lane keyboard navigation (`j/k`), continuous across lane boundaries in the flat visible list.
- Lane counts shown as section headers at all times.
- "Mark all read in lane" with `Shift-R`; PR read state persisted in SQLite (migration 034), Go engine owns the write.
- Auto-read on PR open (Enter, click, or palette — one code path).
- Repo-grouped view remains available via `:view groups` / Settings.

### 5.2 Diff polish — done (image diff deferred)

| Feature | Status | Notes |
|---|---|---|
| CodeView migration | done | Replaces hand-rolled TanStack Virtual; see spike notes doc |
| Side-by-side diff toggle | done | `Shift+S`; persisted in settings |
| Word-level diff within changed lines | done | `lineDiffType: 'word'`, on by default |
| Whitespace-only-change collapse | done | `w`; custom patch-rewriting (library has no ignore-whitespace option) |
| Generated-file collapse | done | Auto-collapse + `z` manual toggle per file |
| Viewed-file sidebar feedback | done | Sidebar icon switches `description` → `check_circle` when a file is marked viewed |
| Image/binary diff handling | deferred | Binary files render a placeholder row with a browser escape hatch (`o`); full image diff is a follow-up |

### 5.3 Command palette polish — done

| Command | Status | Notes |
|---|---|---|
| `:sync` (force sync now) | done (2026-06-12) | Sends FORCE_SYNC; footer shows SYNCING until SYNC_COMPLETE |
| `:settings`, `:about`, `:help` | done | Also implemented: `:clear` (clear filters/search), `:reset-store` |
| `:focus list/detail` | done | |
| `:view lanes` / `:view groups` | done | Switches `prList.view` |
| `:open in browser` | done | Palette command wired to the existing `list.openBrowser` action |
| `:diff split` / `:diff unified` | done | |
| `:open in editor` | done | Checks out the PR head on mismatch (with a confirmation dialog) before opening in `$EDITOR` |
| `:tour` | done | Re-opens the first-run tour |
| Command palette reachable from the diff route | done | `global.command` activation now includes `diff` (was list/detail only) |

### 5.4 Help / discoverability — done (tooltips won't-do)

| Item | Status | Notes |
|---|---|---|
| `?` overlay with all bindings | done | `HelpOverlay.tsx`; diff-review shortcuts shown in their own section |
| First-run tour | done | 5-screen guided intro (`TourOverlay.tsx`), auto-opens once after onboarding, re-runnable via `:tour` |
| "Did you know?" tooltips | won't-do | Deferred indefinitely — no plan to revisit |

---

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
- [ ] Post *new* line-level review comments (`PostReviewComment` with path/commitSHA/side) — replies already work (1.3)
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
