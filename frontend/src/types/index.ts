import type { TriageResponse as TriageResult } from '@bindings/services';
import type { ReviewThreadRow as ReviewThread } from '@bindings/store/models';

export type {
  CommentRow as Comment,
  DiffFile,
  ExpertiseData as ExpertiseDataPayload,
  ExpertiseExpert,
  ExpertiseFileData,
  ExpertiseNudge as ExpertiseNudgePayload,
  ExpertiseSettings as ExpertiseSettingsPayload,
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
