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
- Local, per-user "viewed" state per commit that survives rebases when the
  commit's content is unchanged.
- Keyboard navigation for both views.

Out of scope (v1):

- Line comments, drafts, threads, or review actions from a commit diff. These
  stay on the combined diff page.
- Syncing viewed-commit state to GitHub (GitHub has no such concept).
- Precomputing commits during sync, or caching commit diffs.
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
- Cap: at most 250 commits (matching GitHub's PR commits limit). When more
  exist, return the oldest 250 and set `Truncated`.
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
  pr_id, patch_key, account_provider, account_login, login, viewed_at,
  PRIMARY KEY (pr_id, patch_key, account_provider, account_login, login)
)
```

- `patch_key` is the commit's `PatchID` for non-merge commits, so a rebased
  commit with identical content stays viewed and any content change resets it.
  For merge commits it is `merge:<sha>`.
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
- Header: "N of M viewed". When `truncated`, a notice: "showing first 250
  commits".
- When `available` is false, show the reason, styled like `InterdiffPane`'s
  messages.
- j / k / Enter as on the PR list.

### Commit page — `pages/CommitPage.tsx` + `useCommitData.ts`

- Header: SHA, subject, author, date, the full message body, position
  ("commit 3/7"), and viewed state.
- Body: a read-only `DiffView` with `syncFileSelection={false}`: no threads,
  drafts, line comments, interdiff, or since-last-review.
- Merge commit: "Merge commit. Its changes come from the merged branch;
  review them in their own commits." It can still be marked viewed.
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
