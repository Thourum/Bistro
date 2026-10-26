# Phase 2 — Patchset Semantics

[Roadmap](../ROADMAP.md)

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
> have shipped ([design](../go-project-overview.md#internalprsync----sync-engine)).
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
> [since-last-review across rebases and force-pushes](../go-project-overview.md#internalprsync----sync-engine).

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
>    [current design](../go-project-overview.md#internalprsync----sync-engine).
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
> is replaced by [patch-ID triage and go-git interdiffs](../go-project-overview.md#internalprsync----sync-engine).

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

