/**
 * PR domain model — matches Go `prs` table schema exactly.
 */
export interface PR {
  id: string;
  provider: string;
  owner: string;
  repo: string;
  number: number;
  title: string;
  body: string | null;
  author: string;
  state: 'open' | 'closed' | 'merged';
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  base_branch: string;
  head_branch: string;
  base_sha: string;
  head_sha: string;
  comment_count: number;
  ci_status: string | null;
  draft: number; // 0 or 1 from SQLite
  review_status: string | null; // 'pending' | 'approved' | 'changes_requested'
  approved_by: string | null;
  changes_requested_by: string | null;
  requested_reviewers: string[]; // logins whose review is currently requested
}

/**
 * Comment model — matches Go `comments` table.
 */
type CommentAuthorType = 'human' | 'bot' | 'unknown';

export type CommentSeverity =
  | 'critical'
  | 'warning'
  | 'suggestion'
  | 'nit'
  | 'info';

export interface Comment {
  id: string;
  pr_id: string;
  author: string;
  body: string;
  created_at: string;
  updated_at: string;
  line?: number;
  file?: string;
  is_reply_to?: string;
  // Classification (Phase 3, migration 026) — absent on unclassified/legacy rows.
  author_type?: CommentAuthorType;
  bot_vendor?: string;
  severity?: CommentSeverity;
  classification_source?: string;
  dismissed?: boolean;
  dedup_group_id?: string;
  /**
   * Review-thread this comment belongs to, when it is a thread reply. Already
   * present in the `COMMENTS_READY` payload (comments.thread_id FK); the
   * threads view joins on it to show each thread's discussion inline.
   */
  thread_id?: string;
  /**
   * Raw diff hunk from GitHub for inline review comments. Present only for
   * kind='review'; undefined for issue comments.
   */
  diff_hunk?: string;
}

/**
 * File model for PR changed files.
 */
/**
 * A single changed file as returned in a `DIFF_READY` payload.
 *
 * `content_hash` is computed by the Go engine (SHA-256 of the file's unified
 * diff) and `viewed` reflects the engine's stored viewed state for the current
 * user. The UI never derives these itself — it echoes `content_hash` back
 * unchanged when marking a file viewed.
 */
export interface DiffFile {
  path: string;
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'binary';
  old_path?: string;
  is_binary: boolean;
  is_truncated: boolean;
  diff_status: 'ok' | 'truncated' | 'too_large';
  patch_bytes: number;
  hunk_count: number;
  additions: number;
  deletions: number;
  content_hash: string;
  viewed: boolean;
}

/**
 * An observed patchset snapshot — one row of the `patchsets` table, surfaced
 * via `GET_PATCHSETS` / `PATCHSETS_READY`. `number` is 1-based (P1, P2, …).
 *
 * Field names are snake_case to match the WebSocket payload verbatim; no
 * client-side renaming layer exists for these.
 */
export interface Patchset {
  pr_id: string;
  number: number;
  head_sha: string;
  base_sha: string;
  pushed_at: string; // ISO 8601
}

/**
 * Result of a `REQUEST_RANGE_DIFF` → `RANGE_DIFF_READY` round-trip.
 *
 * `raw` is the diff text to render. When `degraded` is true the engine fell
 * back from `git range-diff` (absent or too old); `fallback_reason` explains
 * why and may be shown to the reviewer. `raw` can be empty even when
 * `degraded` is true — render a no-content state in that case, never a fake
 * diff.
 */
export interface RangeDiffResult {
  pr_id: string;
  from_patchset: number;
  to_patchset: number;
  from_sha: string;
  to_sha: string;
  raw: string;
  degraded: boolean;
  fallback_reason: string;
}

/**
 * Acknowledgement of a `MARK_FILE_VIEWED` request.
 * `ok` is false when the engine declined the write; `reason` explains it.
 */
export interface FileViewedAck {
  pr_id: string;
  file_path: string;
  ok: boolean;
  reason?: string;
}

/**
 * A review thread on a PR — one row of the `review_threads` table, surfaced
 * via `GET_REVIEW_THREADS` / `REVIEW_THREADS_READY`. Field names are snake_case
 * to match the WebSocket payload verbatim; no client-side renaming layer exists.
 *
 * `state_known` is false when the engine could not determine the thread's true
 * resolved/outdated state (e.g. GraphQL was unavailable at sync time). In that
 * case the lifecycle is shown as "unknown" and resolve/unresolve is disabled —
 * the UI never guesses a state the backend hasn't confirmed.
 */
export interface ReviewThread {
  id: string;
  pr_id: string;
  file_path: string;
  line: number;
  resolved: boolean;
  outdated: boolean;
  state_known: boolean;
}

/** Lifecycle bucket derived from a {@link ReviewThread}'s flags. */
export type ThreadLifecycle = 'open' | 'resolved' | 'outdated' | 'unknown';

/**
 * Collapses a thread's three flags into a single lifecycle bucket. Order
 * matters: an unknown-state thread is always "unknown" regardless of its
 * (untrusted) resolved/outdated flags.
 */
export function threadLifecycle(thread: ReviewThread): ThreadLifecycle {
  if (!thread.state_known) return 'unknown';
  if (thread.resolved) return 'resolved';
  if (thread.outdated) return 'outdated';
  return 'open';
}

/**
 * CI check status.
 */
export interface CICheck {
  name: string;
  state: 'pending' | 'success' | 'failure';
  description?: string;
  url?: string;
}

/**
 * Extended PR with detail data (fetched via WebSocket).
 */
/**
 * A PR action in progress.
 */
export interface PendingAction {
  type: 'approve' | 'request_changes' | 'post_comment' | 'reply_comment';
  prId: string;
  startedAt: number;
}

/**
 * Result of a completed PR action.
 */
export interface ActionResult {
  type: 'approve' | 'request_changes' | 'post_comment' | 'reply_comment';
  prId: string;
  success: boolean;
  message: string;
}

/**
 * A toast notification for the UI.
 */
export interface Toast {
  id: string;
  type: 'info' | 'success' | 'error' | 'warning';
  message: string;
  dismissible: boolean;
}

/**
 * Repository discovered from provider, optionally watched for PR sync.
 */
export interface RepoInfo {
  id: string;
  provider: string;
  owner: string;
  repo: string;
  watched: boolean;
  source?: 'local' | 'cache' | null;
  location_status?: string | null;
  location_error_code?: string | null;
  location_error?: string | null;
}
