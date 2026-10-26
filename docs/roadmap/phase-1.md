# Phase 1 — Core Review Surface

[Roadmap](../ROADMAP.md)

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

