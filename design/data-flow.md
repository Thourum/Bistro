# Bistro Data Flow — Request & Update Lifecycle

**Purpose:** Explain how data moves through Bistro from startup to user actions.

**Audience:** Electron and Go teams implementing features. Anyone debugging data inconsistencies.

---

## Overview

Bistro's data lifecycle has three main modes:
1. **Initial Load** — App starts, SQLite reads local DB, renders PR list instantly
2. **Background Sync** — Go process fetches fresh PRs from GitHub, writes to SQLite, notifies Electron
3. **User Actions** — User approves/comments, Electron sends to Go, Go updates GitHub + SQLite

---

## App Startup Flow

```
User clicks app icon (0ms)
    ↓
Electron main process starts, queries SQLite
    ↓
SQLite returns last 50 PRs + metadata (~50ms, local)
    ↓
Renderer renders PR list immediately
    ↓
Go process wakes up, connects WebSocket
    ↓
Go fetches fresh PRs from GitHub (500-2000ms, network)
    ↓
Go writes new PRs to SQLite
    ↓
Go sends WebSocket event: "PRs refreshed"
    ↓
Electron renderer updates list (preserves scroll position, selection)
```

**Key constraint:** PR list renders BEFORE Go fetches fresh data. Users see stale data first, then updates.

---

## Detailed Flows

### Flow 1: User Opens App → Instant PR List

**Goal:** < 500ms from click to visible PR list (using local cache).

**Steps:**

1. **Electron main process starts**
   - Loads SQLite DB from `~/.bistro/data.db`
   - Queries: `SELECT * FROM prs WHERE state = 'open' ORDER BY updated_at DESC LIMIT 50`

2. **SQLite returns results**
   - Column schema:
     ```sql
     id, provider, owner, repo, number, title, body, author, state,
     created_at, updated_at, merged_at, base_branch, head_branch,
     base_sha, head_sha, comment_count, ci_status, draft,
     review_status, approved_by, changes_requested_by
     ```
   - Result: ~50 rows (~100KB)

3. **Electron IPC sends to renderer**
   - Message: `{ type: "PR_LIST_LOADED", prs: [...], timestamp: "2025-04-08T14:32:00Z" }`

4. **Renderer displays PR list**
   - Applies filter (open/draft/merged)
   - Renders visible rows (~20 PRs, rest virtualized)
   - Status bar: "Loaded from cache. Syncing..."

5. **Go process wakes (parallel to step 3-4)**
   - Starts WebSocket connection
   - Loads GitHub token from OS keychain
   - Issues `GET /repos/{owner}/{repo}/pulls` per subscribed repo

6. **GitHub API returns new data**
   - ~2-5 seconds depending on:
     - Number of watched repos
     - Network latency
     - GitHub API rate limits
   - Data: PR number, title, state, review status, CI status

7. **Go updates SQLite**
   - Upserts new/changed PRs: `INSERT OR REPLACE INTO prs (...) VALUES (...)`
   - Deletes merged PRs: `DELETE FROM prs WHERE state = 'merged' AND closed_at < NOW() - 7 days`
   - Updates metadata: `last_sync_at`, `sync_error` (if any)

8. **Go sends WebSocket event to Electron**
   - Message: `{ type: "SYNC_COMPLETE", count: 42, timestamp: "2025-04-08T14:32:05Z" }`

9. **Electron renderer updates PR list**
   - Merges new PRs into state
   - Maintains scroll position and selection
   - Status bar: "Synced 42 PRs"

---

### Flow 2: User Opens a PR → Diff Renders

**Goal:** < 100ms if diff cached locally, < 2s if needs clone.

**Steps:**

1. **User presses Enter on PR in list**

2. **Electron queries SQLite for PR details**
   - Queries:
     ```sql
     SELECT * FROM prs WHERE id = ?
     SELECT * FROM diffs WHERE pr_id = ?
     SELECT * FROM comments WHERE pr_id = ? ORDER BY created_at
     ```
   - Diff table schema:
     ```sql
     id, pr_id, repo, base_commit, head_commit, raw_diff,
     cached_at, size_bytes
     ```

3. **Diff rendering (parallel paths)**

   **Path A: Diff cached (95% of cases)**
   - SQLite returns cached diff from previous sync
   - Electron renders diff immediately (~50-100ms)
   - Status bar: "Loaded from cache"

   **Path B: Diff not cached (new PR or cache expired)**
   - Go receives IPC: `{ type: "FETCH_DIFF", pr_id: 123 }`
   - Go clones/pulls repo if needed (seconds, first time only)
   - Go computes diff: `git diff base_commit...head_commit`
   - Go stores in SQLite: `INSERT INTO diffs (...) VALUES (...)`
   - Go sends IPC: `{ type: "DIFF_READY", diff: "..." }`
   - Electron renders (~2-5s total)

4. **Comments load (second, non-blocking)**
   - Electron queries SQLite: `SELECT * FROM comments WHERE pr_id = ?`
   - Comments rendered below diff
   - Meanwhile, Go fetches fresh comments from GitHub API in background
   - GitHub returns comments, Go stores in SQLite
   - Go sends WebSocket: `{ type: "COMMENTS_UPDATED", pr_id: 123 }`
   - Electron updates comment display (preserves scroll, reveals new comments)

5. **Status sidebar updates**
   - Queries: review_status, approved_by, ci_status
   - Renders: approval state, CI checks, PR metadata

---

### Flow 3: User Approves PR

**Goal:** < 3s round-trip approval + database update.

**Steps:**

1. **User presses `a` twice (confirmation)**

2. **Electron sends to Go via IPC**
   - Message: `{ type: "APPROVE_PR", pr_id: 123, repo: "org/repo", pr_number: 456 }`

3. **Go approves on GitHub**
   - Calls GitHub API: `POST /repos/{owner}/{repo}/pulls/{pr_number}/reviews`
   - Payload: `{ event: "APPROVE" }`
   - Response: `{ id: 999, state: "APPROVED", ... }`

4. **Go updates SQLite**
   - Updates PR row: `review_status = 'approved', approved_by = 'current_user', approved_at = NOW()`
   - Records action: `INSERT INTO actions (type, pr_id, result) VALUES ('approve', 123, 'success')`

5. **Go sends WebSocket to Electron**
   - Message: `{ type: "PR_APPROVED", pr_id: 123, approved_by: "you" }`

6. **Electron updates UI**
   - Status pane changes to ✓ APPROVED (green)
   - PR list updates review_status indicator
   - Status bar: "PR approved"
   - Brief haptic feedback (macOS)

7. **Electron syncs with WebSocket confirmation**
   - Waits for Go confirmation before considering action complete
   - Timeout: 3s → show error "Approval timeout, try again"

---

### Flow 4: User Comments on PR

**Goal:** < 500ms local comment appears, 2-3s server confirmation.

**Steps:**

1. **User presses `c` on a diff line**

2. **Comment box appears (local)**
   - Focus moves to comment input
   - Placeholder: "Add comment (Cmd+Enter to send, Esc to cancel)"

3. **User types comment**

4. **User presses Cmd+Enter**

5. **Electron sends to Go via IPC**
   - Message: `{ type: "POST_COMMENT", pr_id: 123, line: 42, body: "..." }`

6. **Optimistic update (local)**
   - Comment appears immediately with "Sending..." state
   - Not stored in SQLite yet (in-memory only)

7. **Go posts to GitHub**
   - Calls GitHub API: `POST /repos/{owner}/{repo}/pulls/{pr_number}/comments`
   - Payload includes: `body, line, commit_id`
   - Response: `{ id: 888, created_at: "..." }`

8. **Go stores in SQLite**
   - Inserts: `INSERT INTO comments (pr_id, github_id, author, body, line, created_at) VALUES (...)`

9. **Go sends WebSocket to Electron**
   - Message: `{ type: "COMMENT_POSTED", pr_id: 123, comment_id: 888 }`

10. **Electron confirms**
    - Comment state changes from "Sending..." to "Posted"
    - Stores in SQLite for next sync
    - Syncs to WebSocket event

---

### Flow 5: Background Sync (No User Input)

**Goal:** Every 30-60s, refresh PR list without blocking UI.

**Steps:**

1. **Go timer fires (60s interval)**

2. **Go fetches PRs from all subscribed repos**
   - GitHub API: `GET /repos/{owner}/{repo}/pulls?state=all`
   - Parallel requests (one per repo)
   - Timeout: 10s per request

3. **Go processes results**
   - For each PR:
     - Check if in SQLite already
     - If new or updated: upsert
     - If merged/closed > 7 days: delete
   - Update sync metadata: `last_sync_at = NOW()`, `sync_error = NULL`

4. **Go stores in SQLite**
   - Transaction (atomic): all or nothing
   - Duration: ~100ms (50 PRs)

5. **Go sends WebSocket event**
   - Message: `{ type: "SYNC_COMPLETE", count: 42, timestamp: "..." }`

6. **Electron processes event**
   - Merges new PRs into list
   - If user is in detail view: fetch fresh comments (do NOT interrupt detail pane)
   - Update status bar timestamp
   - If new PR in current filter: add to visible list

---

### Flow 6: Multi-Provider (GitHub + GitLab) — Future: Not Yet Implemented

> **Note:** This flow is planned for a future release. Only GitHub is supported today. The schema and design below reflect the intended architecture when multi-provider support is added.

**Goal:** Single DB stores PRs from multiple providers; filters by provider.

**Schema addition:**

```sql
-- provider column (github, gitlab, gitea)
prs.provider = 'github' OR 'gitlab' OR 'gitea'

-- Example: PR from GitHub
{ id: 1, provider: 'github', owner: 'org', repo: 'repo', number: 123, ... }

-- Example: PR from GitLab
{ id: 2, provider: 'gitlab', owner: 'group', repo: 'project', number: 456, ... }
```

**Sync loop (parallel for each provider):**

1. Go maintains separate auth tokens per provider (OS keychain)
2. For each provider:
   - Fetch PRs from provider API (GitHub /pulls, GitLab /merge_requests)
   - Upsert to SQLite with `provider = 'github'` OR `'gitlab'`
3. Electron filters by provider in UI:
   - PR list query: `SELECT * FROM prs WHERE provider IN ('github', 'gitlab')`
   - OR filter by single provider: `SELECT * FROM prs WHERE provider = 'github'`

**No conflicts:** PK is `(provider, owner, repo, number)`, not just `number`.

---

### Flow 7: Offline Mode (No Network)

**Goal:** App remains functional; actions queue for sync.

**Steps:**

1. **Electron detects no network**
   - Failed WebSocket connect or Go process unreachable
   - Status bar: "⚠ Offline (using cache)"

2. **Read operations work normally**
   - SQLite queries work: PR list, diffs, comments all from cache
   - Diff rendering: uses cached diffs (no new diffs until sync)

3. **User tries to approve/comment**
   - Electron sends IPC to Go as normal
   - Go can't reach GitHub API (no network)
   - Go returns error: `{ type: "ERROR", code: "NETWORK_ERROR", message: "No internet" }`
   - Electron shows modal: "Can't reach GitHub. Try again when online."

4. **OR: Electron implements action queue** (optional Phase 2)
   - User approves offline
   - Electron stores in local queue table: `offline_actions`
   - Status: "Queued for sync" (yellow indicator)
   - When network returns, Go processes queue in order

---

## Cache Invalidation Strategy

| Data | Cache Lifetime | Invalidated By |
|------|----------------|----------------|
| PR list | 30-60s | Background sync timer |
| Diff | 7 days | User manually refresh OR diff outdated |
| Comments | 5 min | Background refresh OR new comment posted |
| User profile | 1 hour | Logout/login |
| GitHub token | Until revoked | User revokes in GitHub settings |

---

## Error Handling in Data Flow

**See `error-handling.md` for comprehensive error strategy.**

Quick reference:
- GitHub API timeout → retry with exponential backoff, max 3 retries
- Network error → show user-friendly message, don't crash
- SQLite write conflict → queue action, retry on next sync
- Malformed response → log error, skip PR, continue

---

## Database Schema Summary

```sql
-- PRs table
CREATE TABLE prs (
    id              TEXT     PRIMARY KEY,
    provider        TEXT     NOT NULL,
    owner           TEXT     NOT NULL,
    repo            TEXT     NOT NULL,
    number          INTEGER  NOT NULL,
    title           TEXT     NOT NULL,
    body            TEXT,
    author          TEXT     NOT NULL,
    state           TEXT     NOT NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    merged_at       DATETIME,
    base_branch     TEXT     NOT NULL,
    head_branch     TEXT     NOT NULL,
    base_sha        TEXT     NOT NULL,
    head_sha        TEXT     NOT NULL,
    comment_count   INTEGER  DEFAULT 0,
    ci_status       TEXT,
    draft           INTEGER  DEFAULT 0,
    review_status   TEXT,
    approved_by     TEXT,
    changes_requested_by TEXT,
    UNIQUE(provider, owner, repo, number)
);

-- Diffs table
CREATE TABLE diffs (
  id TEXT PRIMARY KEY,
  pr_id TEXT NOT NULL,
  repo TEXT NOT NULL,
  base_commit TEXT,
  head_commit TEXT,
  raw_diff TEXT NOT NULL,
  cached_at TEXT,
  size_bytes INTEGER,
  FOREIGN KEY(pr_id) REFERENCES prs(id)
);

-- Comments table
CREATE TABLE comments (
  id TEXT PRIMARY KEY,
  pr_id TEXT NOT NULL,
  github_id TEXT,
  author TEXT NOT NULL,
  body TEXT NOT NULL,
  line INTEGER,
  created_at TEXT,
  updated_at TEXT,
  FOREIGN KEY(pr_id) REFERENCES prs(id)
);

-- Sync metadata
CREATE TABLE sync_metadata (
  provider TEXT PRIMARY KEY,
  last_sync_at TEXT,
  sync_error TEXT,
  pr_count INTEGER
);
```

---

## Questions?

**Q: Why not sync comments automatically?**
A: Comments are lower priority than PRs. We fetch fresh comments when user opens a PR, not on every sync. Saves bandwidth and GitHub API quota.

**Q: What if a PR is deleted on GitHub?**
A: Go checks for 404 responses, removes from SQLite. Electron removes from list if user is viewing it.

**Q: Why SQLite for caching, not in-memory?**
A: Persistence across app restarts. Survives crashes. Enables offline mode.

**Q: How do we handle stale diffs?**
A: Compare `head_commit` in DB with GitHub PR. If different, invalidate cache and re-fetch.

---

**Last updated:** 2026-04-08
