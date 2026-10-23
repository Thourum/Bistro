import { z } from 'zod';

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
  touches_you: boolean;
  // Inbox signals (Phase 5, Task 4) — payload keys match Go JSON verbatim.
  mentioned: boolean;
  viewer_commented: boolean;
  unread: boolean;
  reviewed_head_stale: boolean;
  hydrated: number; // 0 or 1 from SQLite; 0 = list-only row, details fetched on demand
}

export interface ExpertiseExpert {
  author_email: string;
  author_login: string | null;
  surviving_lines: number;
  total_lines: number;
  commit_score: number;
  is_original: boolean;
  score: number;
  dormant: boolean;
}

export interface ExpertiseFileData {
  file_path: string;
  experts: ExpertiseExpert[];
  your_rank: number | null;
}

export type ExpertiseRepoStatusKind = 'idle' | 'indexing' | 'ready' | 'error';

export interface ExpertiseRepoStatus {
  repo_id: string;
  status: ExpertiseRepoStatusKind;
  files_done?: number;
  files_total?: number;
  indexed_sha?: string;
  skipped_files?: number;
  code?: string;
  message?: string;
}

export interface ExpertiseDataPayload {
  pr_id?: string | undefined;
  status?: 'ready' | 'unavailable' | undefined;
  reason?: string | undefined;
  files: ExpertiseFileData[];
}

export const expertiseDataSchema = z.object({
  pr_id: z.string().optional(),
  status: z.enum(['ready', 'unavailable']).optional(),
  reason: z.string().optional(),
  files: z.array(
    z.object({
      file_path: z.string(),
      experts: z.array(
        z.object({
          author_email: z.string(),
          author_login: z.string().nullable(),
          surviving_lines: z.number(),
          total_lines: z.number(),
          commit_score: z.number(),
          is_original: z.boolean(),
          score: z.number(),
          dormant: z.boolean(),
        }),
      ),
      your_rank: z.number().nullable(),
    }),
  ),
}) satisfies z.ZodType<ExpertiseDataPayload>;

export interface ExpertiseNudgePayload {
  repo_id: string;
  reviewed_count: number;
}

export interface ExpertiseSettingsPayload {
  alias_emails: string[];
  min_surviving_lines: number;
  excludes: Record<string, string[]>;
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
  line?: number | undefined;
  file?: string | undefined;
  is_reply_to?: string | undefined;
  // Classification (Phase 3, migration 026) — absent on unclassified/legacy rows.
  author_type?: CommentAuthorType | undefined;
  bot_vendor?: string | undefined;
  severity?: CommentSeverity | undefined;
  classification_source?: string | undefined;
  dismissed?: boolean | undefined;
  dedup_group_id?: string | undefined;
  /**
   * Review-thread this comment belongs to, when it is a thread reply. Already
   * present in the `COMMENTS_READY` payload (comments.thread_id FK); the
   * threads view joins on it to show each thread's discussion inline.
   */
  thread_id?: string | undefined;
  /**
   * Raw diff hunk from GitHub for inline review comments. Present only for
   * kind='review'; undefined for issue comments.
   */
  diff_hunk?: string | undefined;
}

// The engine also sends anchor metadata not represented by the shared Comment type.
export const commentSchema = z.looseObject({
  id: z.string(),
  pr_id: z.string(),
  author: z.string(),
  body: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
  line: z.number().optional(),
  file: z.string().optional(),
  is_reply_to: z.string().optional(),
  author_type: z.enum(['human', 'bot', 'unknown']).optional(),
  bot_vendor: z.string().optional(),
  severity: z
    .enum(['critical', 'warning', 'suggestion', 'nit', 'info'])
    .optional(),
  classification_source: z.string().optional(),
  dismissed: z.boolean().optional(),
  dedup_group_id: z.string().optional(),
  thread_id: z.string().optional(),
  diff_hunk: z.string().optional(),
}) satisfies z.ZodType<Comment>;

export const commentsResponseSchema = z.object({
  comments: z
    .array(commentSchema)
    .nullish()
    .transform((comments) => comments ?? []),
});

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
  old_path?: string | undefined;
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

export const diffFileSchema = z.object({
  path: z.string(),
  status: z.enum(['added', 'modified', 'deleted', 'renamed', 'binary']),
  old_path: z.string().optional(),
  is_binary: z.boolean(),
  is_truncated: z.boolean(),
  diff_status: z.enum(['ok', 'truncated', 'too_large']),
  patch_bytes: z.number(),
  hunk_count: z.number(),
  additions: z.number(),
  deletions: z.number(),
  content_hash: z.string(),
  viewed: z.boolean(),
}) satisfies z.ZodType<DiffFile>;

export const diffResponseSchema = z.object({
  diff: z.string().optional(),
  files: z
    .array(diffFileSchema)
    .nullish()
    .transform((files) => files ?? []),
  additions: z.number().nullish(),
  deletions: z.number().nullish(),
});

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

export const patchsetSchema = z.object({
  pr_id: z.string(),
  number: z.number(),
  head_sha: z.string(),
  base_sha: z.string(),
  pushed_at: z.string(),
}) satisfies z.ZodType<Patchset>;

const triageFileStatusSchema = z.enum([
  'changed',
  'new',
  'removed',
  'unchanged',
]);

/**
 * Result of a `REQUEST_TRIAGE` → `TRIAGE_READY` round-trip: each file's status
 * in `to_patchset` relative to `from_patchset`. `baseline` is the review event
 * the engine chose for since-last-review, `null` for an explicit pair. When
 * `state` is not `ok`, `files` is empty and `reason` says why.
 */
export const triageResultSchema = z.object({
  pr_id: z.string(),
  state: z.enum(['ok', 'no_patchsets', 'no_baseline', 'no_fingerprints']),
  reason: z.string(),
  from_patchset: z.number(),
  to_patchset: z.number(),
  baseline: z
    .object({
      source: z.enum(['github_review', 'bistro_action', 'all_viewed']),
      reviewed_at: z.string(),
    })
    .nullable(),
  files: z.array(
    z.object({ path: z.string(), status: triageFileStatusSchema }),
  ),
  changed_count: z.number(),
  total_count: z.number(),
});
export type TriageResult = z.infer<typeof triageResultSchema>;
export type TriageFileStatus = z.infer<typeof triageFileStatusSchema>;
export type TriageBaselineSource = NonNullable<
  TriageResult['baseline']
>['source'];

/**
 * Acknowledgement of a `MARK_FILE_VIEWED` request.
 * `ok` is false when the engine declined the write; `reason` explains it.
 */
export const fileViewedAckSchema = z.object({
  pr_id: z.string(),
  file_path: z.string(),
  ok: z.boolean(),
  reason: z.string().optional(),
});
export type FileViewedAck = z.infer<typeof fileViewedAckSchema>;

/** Acknowledgement of a `MARK_PRS_READ` request. */
export const prsReadAckSchema = z.object({
  ok: z.boolean(),
  marked: z.number(),
  reason: z.string().optional(),
});
export type PRsReadAck = z.infer<typeof prsReadAckSchema>;

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

export const reviewThreadSchema = z.object({
  id: z.string(),
  pr_id: z.string(),
  file_path: z.string(),
  line: z.number(),
  resolved: z.boolean(),
  outdated: z.boolean(),
  state_known: z.boolean(),
}) satisfies z.ZodType<ReviewThread>;

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
  description?: string | undefined;
  url?: string | undefined;
}

export const ciCheckSchema = z.object({
  name: z.string(),
  state: z.enum(['pending', 'success', 'failure']),
  description: z.string().optional(),
  url: z.string().optional(),
}) satisfies z.ZodType<CICheck>;

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
