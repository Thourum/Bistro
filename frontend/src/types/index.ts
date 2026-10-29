import type { TriageResponse as TriageResult } from '@bindings/services';
import type { ReviewThreadRow as ReviewThread } from '@bindings/store/models';
import { z } from 'zod';

export type {
  CommentRow as Comment,
  DiffFile,
  FileViewed as FileViewedAck,
  InterdiffResponse as InterdiffResult,
  MarkReadResult as PRsReadAck,
  PRRow as PR,
  RepoRow as RepoInfo,
  TriageResponse as TriageResult,
} from '@bindings/services';
export type {
  Patchset,
  ReviewThreadRow as ReviewThread,
} from '@bindings/store/models';
export type { Draft as ReviewDraft } from '@bindings/store/reviewdraft/models';
export type { ReviewQueue } from '../../bindings/github.com/itterno/bistro/internal/provider/models';

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

export type CommentSeverity =
  | 'critical'
  | 'warning'
  | 'suggestion'
  | 'nit'
  | 'info';
export type TriageFileStatus = NonNullable<
  TriageResult['files']
>[number]['status'];
export type TriageBaselineSource = NonNullable<
  TriageResult['baseline']
>['source'];

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

/**
 * Extended PR with detail data (fetched via WebSocket).
 */
/**
 * A PR action in progress.
 */
export interface PendingAction {
  type:
    | 'approve'
    | 'request_changes'
    | 'post_comment'
    | 'reply_comment'
    | 'submit_review';
  prId: string;
  startedAt: number;
}

/**
 * Result of a completed PR action.
 */
export interface ActionResult {
  type:
    | 'approve'
    | 'request_changes'
    | 'post_comment'
    | 'reply_comment'
    | 'submit_review';
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
