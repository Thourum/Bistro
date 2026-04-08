/**
 * PR domain model — matches Go `prs` table schema exactly.
 *
 * Deviations from api-contracts.md (to be reconciled):
 * - `repo` is split into `owner` + `repo` (Go stores separately)
 * - `description` is called `body` (Go naming)
 * - `review_status` is missing (not yet in Go schema)
 * - `draft` is a separate integer column (not a state value)
 * - Extra columns: base_sha, head_sha, comment_count
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
  draft: number; // 0 or 1
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
