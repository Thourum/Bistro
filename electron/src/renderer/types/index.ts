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
}

/**
 * File model for PR changed files.
 */
export interface File {
  path: string;
  status: 'added' | 'modified' | 'deleted';
  additions: number;
  deletions: number;
}

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
  additions: number;
  deletions: number;
  content_hash: string;
  viewed: boolean;
}

/**
 * An observed patchset snapshot — one row of the `patchsets` table, surfaced
 * via `GET_PATCHSETS` / `PATCHSETS_READY`. `number` is 1-based (P1, P2, …).
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
 * back from `git range-diff`; `fallback_reason` explains why.
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
 */
export interface FileViewedAck {
  pr_id: string;
  file_path: string;
  ok: boolean;
  reason?: string;
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
export interface PRDetail extends PR {
  files: File[];
  comments: Comment[];
  ci_checks: CICheck[];
}

/**
 * A PR action in progress.
 */
export interface PendingAction {
  type: 'approve' | 'request_changes' | 'post_comment';
  prId: string;
  startedAt: number;
}

/**
 * Result of a completed PR action.
 */
export interface ActionResult {
  type: 'approve' | 'request_changes' | 'post_comment';
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
}
