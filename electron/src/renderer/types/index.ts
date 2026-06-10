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
