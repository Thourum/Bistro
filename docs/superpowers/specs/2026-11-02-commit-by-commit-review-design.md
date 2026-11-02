# Commit-by-commit review — design

Status: approved in brainstorming, 2026-11-02.

## Goal

Let a reviewer read a PR one commit at a time instead of only the combined
"all files changed" diff, and track which commits they have reviewed. Many
AI-authored PRs are a sequence of small, purposeful commits; reading them in
order is often faster than reading the squashed result.

## Scope

In scope:

- A commit list for a PR and a read-only diff for each commit.
- Local, per-user "viewed" state per commit. For non-merge commits it
  survives rebases when the commit's content is unchanged; merge commits are
  tracked by SHA and reset when recreated.
- Keyboard navigation for both views.

Out of scope (v1):

- Line comments, drafts, threads, or review actions from a commit diff. These
  stay on the combined diff page.
- Syncing viewed-commit state to GitHub (GitHub has no such concept).
- Caching full commit patches (the per-commit diff stays computed on demand;
  only list statistics are precomputed — see Amendment 1).
- Fetching commits from the GitHub API.

## Approach

Commits and their diffs are computed on demand from the local clone that PR
sync already maintains, the same way `Interdiff.Get` works today. Sync,
`pr_diffs`, drafts, and threads are unchanged. The only persisted addition is
the viewed-commit table.

Rejected alternatives: precomputing in sync (slower sync and storage growth
for an occasional view) and the GitHub API (online-only, rate-limited, and
contrary to Bistro's local-diff design).

## Engine

### Commit range — `internal/git` (`commits.go`)

Input: local repo path, PR base SHA, PR head SHA, merge-base SHA. The
merge-base is the current patchset's `merge_base_sha` when recorded,
otherwise `git.MergeBase(localPath, baseSHA, headSHA)`.

Behavior:

- The range is `git rev-list <merge-base>..<head>`: commits reachable from
  head and not reachable from the merge-base. Commits merged in from the base
  branch (e.g. "Merge main into feature") are excluded because they are
  ancestors of the merge-base.
- Order: topological, oldest first (parents before children).
- Cap: at most 250 commits. When more exist, return the oldest 250 and set
  `Truncated`. Rationale: `List` diffs every commit on each request (stats
  and viewed keys need the patch), so the cap bounds that latency without a
  cache. A PR with more than 250 commits is outside what step-by-step review
  serves well; for those, the combined diff stays the full review path, and
  the truncation notice links there. Raising the cap or paging is a follow-up
  if real PRs hit it.
- Per commit: SHA, parent SHAs, subject (first message line), body (rest of
  the message, trimmed), author name, authored-at time, `IsMerge` (more than
  one parent).
- Non-merge commits are diffed against their single parent (or the empty tree
  for a root commit) with `diff.Generate` and `diff.DefaultOptions()`,
  producing additions, deletions, file count, the unified patch, and a
  `PatchID` of the commit's full unified patch.
- Merge commits get no diff: zero stats, empty patch.
- Reads take the shared per-repo lock from `repolock`, as interdiff does.

### `Commits` service — `internal/services/commits.go`

Dependencies: `RepoForPR`, the PR reader (base/head SHAs), the patchset
reader (current merge-base), the commit-viewed store, `CurrentUser`,
`Panics`, `Log`. Every method is wrapped in `guard`.

```
Commits.List(PRRef) -> CommitsResponse
  { pr_id, available, reason, truncated, commits: []Commit }
  Commit { sha, parent_shas, subject, body, author_name, authored_at,
           is_merge, additions, deletions, file_count, viewed }

Commits.Get(CommitRequest{pr_id, sha}) -> CommitDiffResponse
  { pr_id, sha, available, reason, diff, additions, deletions }

Commits.SetViewed(SetCommitViewedRequest{pr_id, sha, viewed}) -> CommitViewed
  { pr_id, sha, viewed }
```

- `Get` and `SetViewed` accept only a SHA inside the PR's current range;
  anything else returns `invalid`. This prevents diffing arbitrary objects.
- `SetViewed` computes the viewed key on the server from the SHA; it never
  accepts a key from the client.
- `viewed` in `List` is true when a row exists for the commit's key and the
  current user.

### Viewed state — `internal/store/commitviewed`

Migration `00038_add_commits_viewed.sql`:

```
commits_viewed(
  pr_id, patch_key, occurrences, account_provider, account_login, login,
  viewed_at,
  PRIMARY KEY (pr_id, patch_key, account_provider, account_login, login)
)
```

- For non-merge commits, `patch_key` is `<PatchID>:<n>`, where `n` is the
  zero-based count of earlier commits in the range (oldest first) with the
  same `PatchID`. A rebased commit with identical content keeps its key and
  stays viewed; any content change resets it. The ordinal keeps repeated
  identical commits (apply, revert, re-apply) as separate review steps.
- Each row also stores `occurrences`: how many commits in the range shared
  that `PatchID` when it was marked. A commit counts as viewed only when a
  row exists and its stored `occurrences` equals the current count. So when a
  rewrite adds, removes, or changes one of several identical commits, all
  commits with that fingerprint reset to unviewed instead of passing viewed
  state between them. Commits with a unique fingerprint (count 1 before and
  after) are unaffected.
- For merge commits, `patch_key` is `merge:<sha>`. Merges have no shown diff to
  fingerprint, so a recreated merge resets to unviewed.
- Queries in `internal/store/queries/commits_viewed.sql`: upsert, delete, and
  list keys for (pr, account, login). Generated with sqlc.
- The store is an independent subpackage like `store/prdiff`; it may import
  `store` and `store/db`, never the reverse.
- Local only; never sent to GitHub.

## Frontend

### Routes — `router.tsx`

- `/prs/$prId/commits` — commit list.
- `/prs/$prId/commits/$sha` — one commit's diff.
- New `RouteBase` values `'commits'` and `'commit'`; `routeBaseFromPath`
  maps both.
- Back / Esc: commit page → commit list → PR detail.

### Commit list — `pages/CommitsPage.tsx` + `useCommitsData.ts`

- One row per commit, oldest first: short SHA, subject, author, relative
  date, +/- and file count, a viewed square, and a `merge` tag on merges.
- Header: "N of M viewed". When `truncated`, a notice: "showing the first
  250 commits; review the rest in the combined diff", linking to
  `/prs/$prId/diff`.
- When `available` is false, show the reason, styled like `InterdiffPane`'s
  messages.
- j / k / Enter as on the PR list.

### Commit page — `pages/CommitPage.tsx` + `useCommitData.ts`

- Header: SHA, subject, author, date, the full message body, position
  ("commit 3/7"), and viewed state.
- Body: a read-only `DiffView` with no threads, drafts, line comments,
  interdiff, or since-last-review. The route takes the diff page's `?file=`
  search and keeps file selection synced, so the reused next/prev-file keys
  work; it is the only `DiffView` on the page.
- Merge commit: "Merge commit — its diff is not shown. Edits made while
  resolving conflicts live only in the merge, so they appear in the combined
  diff but in none of the listed commits. Review them there." with a link to
  `/prs/$prId/diff`. It can still be marked viewed.
- If the SHA is no longer in the PR's range (the head moved), the page shows
  "this commit is no longer part of the PR" with a link to the list.

### Services

`services/commits.ts` calls `Commits.List` / `Commits.Get` through `call()`
and `Commits.SetViewed` through `callWrite()`. Viewed toggles update the UI
immediately and roll back on a fault. Errors use `@thourum/fault` `Result`
chains and branch on `fault.tag` / `faultReason`, never message text.

### Keyboard

Bindings are config in `ACTION_CATALOG` / `KEYMAPS`, handlers in
`keybindingActions.ts`, registered only through `useAppHotkeys`. The vim keys
are below; the plan picks emacs and standard keys and validates every style
with `utils/keybindingConflicts.ts`.

| Action | Routes | vim |
|---|---|---|
| `detail.commits` / `diff.commits` — open commit list (also in the palette) | detail, diff | `g c` |
| `commits.down` / `commits.up` / `commits.open` | commits | `j` / `k` / `Enter` |
| `commit.next` / `commit.prev` — step commits, clamped at the ends | commit | `]` / `[` |
| `commit.toggleViewed` | commits, commit | `v` |
| `commit.viewedAndNext` — mark viewed, then go to the next commit | commit | `Shift+V` |

The existing diff scroll and file-navigation actions (`detail.scroll*`,
`detail.pageDown` / `pageUp`, `diff.nextFile` / `prevFile`,
`diff.toggleStyle`, `diff.toggleCollapse`, `diff.toggleWhitespace`,
`diff.openFileBrowser`) also become active on `'commit'`. Comment, draft,
review, interdiff, and since-last-review actions do not.

## Error handling

- When data is missing, `List` and `Get` return `available: false` with a
  reason instead of an error: no local clone yet, head or merge-base missing
  from the clone, PR not yet synced. This matches `Interdiff`.
- Classified `ServiceError`s:
  - unknown PR → `not_found`
  - malformed SHA or SHA outside the PR's range → `invalid`
  - context cancelled / deadline → `cancelled` / `timeout`
  - `SetViewed` with no signed-in login → `unauthenticated`
- The frontend shows the reason for `available: false`, and the stale-SHA
  message for `invalid` on the commit page.

## Testing

Each test must fail if the behavior breaks.

- `internal/git`, using `gittest` fixtures:
  - The range excludes base ancestors, including commits that arrive through
    a base→branch merge.
  - Order is oldest first; merge commits are flagged and have no diff.
  - More than 250 commits sets `Truncated` and returns the oldest 250.
- `internal/services`:
  - A SHA outside the range returns `invalid`.
  - No local copy returns `available: false`.
  - Viewed survives a rebase with identical content and resets when the
    content changes.
  - Two identical commits in one range (apply, revert, re-apply) have
    separate viewed state.
  - When a rewrite drops one of two identical commits, the remaining one
    shows as unviewed even if the dropped one was viewed.
- `internal/store/commitviewed`: upsert, delete, and list round-trip against
  `storetest.Open`.
- Frontend: utility tests only (no React component tests). Cover the pure
  next/prev/position helper and keybinding activation on the new routes.
  Pages are checked manually in `wails3 task dev`.

## Generated code and docs

- Run `wails3 task generate` (sqlc + bindings); commit `internal/store/db/`
  and `frontend/bindings/`.
- `AGENTS.md`: add the `Commits` service and the `commitviewed` store to the
  architecture notes and the independent-stores list.
- `website/src/pages/docs/`: document the commit views and new shortcuts.

## Amendment 1 — list performance (2026-11-02)

Status: approved by the user after measurement, 2026-11-02.

### Problem

Measured on a cached clone (PR with 89 commits), then confirmed in the app
with debug timing logs: `Commits.Get` takes 13 ms,
the range walk 10 ms, and `Commits.List` 2.4 s, because `List` diffs every
commit for stats and viewed keys. A CPU profile splits that cost between
go-git's regexp line splitting in the unified encoder (~46%) and re-opening
the packfile for every object read (~30%). The renderer refetched `List` on
every commit change and showed a loading state until it returned, so each
step between commits took seconds. In dev, React StrictMode ran each load
effect twice; with no request sharing, opening the list ran two serialized
`List` calls (about 4.9 s) and each commit's `Get` ran twice. The repo lock
showed no sync contention; all waiting was behind other `List` calls.

### Changes

1. **Renderer.** `useCommitsData` loads the list once per PR and keeps it
   across commit navigation; it reloads only when the PR changes or the user
   re-enters the list (a head that moves meanwhile surfaces through `Get`'s
   existing "no longer part of the PR" message, which links to the list). The
   commit page renders the commit diff as soon as `Get` returns and no longer
   waits for the list; header fields that come from the list (position,
   viewed) fill in when it arrives. `services/commits.ts` shares in-flight
   `List` and `Get` requests per key and keeps the last list per PR, so
   remounts and StrictMode's double effects issue one engine call.
   `CommitDiffResponse` gains `is_merge` so the commit page can show the
   merge message before the list arrives.
2. **Commit stats cache.** Migration `00039_add_commit_stats.sql`:

   ```
   commit_stats(
     pr_id, sha, additions, deletions, file_count, patch_id, algo_version,
     PRIMARY KEY (pr_id, sha),
     FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
   )
   ```

   A commit SHA fixes its tree and its first parent, so its diff never
   changes; rows are never invalidated by sync. `algo_version` is a Go
   constant; a row with a different version counts as missing, so a future
   change to diff options or fingerprinting recomputes instead of reusing
   stale fingerprints. Merges get no row. Store: `internal/store/commitstats`
   (`List(prID, version) map[sha]Stats`, `Upsert(prID, version, []Stats)`,
   and `ReplaceTx(tx, prID, version, []Stats)` for sync), independent like
   `store/commitviewed`.
3. **Sync precomputes.** In `fetchDiff`, inside the existing `WithPRClone`
   callback (clone ready, repo lock held), after the PR diff is generated,
   sync lists the capped range for the PR's merge-base..head, computes stats
   only for SHAs without a current-version row, and carries all current-range
   rows on `FetchedPR`. The writer replaces the PR's rows with them (pruning
   SHAs a force-push dropped) inside the per-PR savepoint, under a nested
   savepoint so a failed stats write rolls back only the stats; the PR row
   exists by then, so the FK holds for new PRs. A failure here is logged and leaves the
   cache untouched; it never fails the PR sync.
4. **List reads the cache.** `List` walks the range (no diffs), reads
   `commit_stats`, computes only the missing non-merge commits under the repo
   lock, upserts them, and builds summaries and viewed keys from the stored
   patch IDs. `SetViewed` uses the same path. A fully warmed PR's `List`
   costs the walk plus one query.
5. **Packfile descriptors.** Range reads open the repository with go-git's
   `KeepDescriptors` storage option and close it when done, so object reads
   stop re-opening the packfile.

6. **Timing logs.** `Commits.List` / `Get` / `SetViewed` and the range reads
   log durations at debug level (lock wait, walk, diff), so a regression
   shows up in `BISTRO_LOG_LEVEL=debug` logs.

Unchanged: `Get` still computes the single commit's patch on demand; the
viewed-key scheme; the list layout; routes and keys.

### Testing

- `commitstats`: upsert, list, prune, and version mismatch round-trip.
- `services`: `List` with a warm cache does not call the per-commit diff
  function; with a partial cache it computes only the missing SHAs and
  stores them; viewed keys built from cached patch IDs match those from
  computed diffs.
- `prsync`: a synced PR leaves rows for every non-merge commit in its range,
  a force-push prunes rows for dropped SHAs, and a stats failure does not
  fail the PR write.
- Renderer: utility tests only; navigation latency is checked manually in
  `wails3 task dev` (stepping between commits must not show the list
  loading state).
