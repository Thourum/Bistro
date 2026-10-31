import type { Comment } from '../types';

/**
 * DISMISS_RESULT payload — one bulk-dismiss batch. `resolved_thread_ids`
 * means "resolve ENQUEUED", not "resolved on GitHub": undo simply enqueues
 * the matching unresolves and FIFO queue ordering settles the final state
 * (spec: Dismissals 3.5).
 */
export interface DismissBatch {
  batch_id: string;
  dismissed_ids: string[];
  resolved_thread_ids: string[];
}

/** Comments `d n` targets: nit + info severity, not already dismissed. */
export function selectNitIds(comments: readonly Comment[]): string[] {
  return comments
    .filter(
      (comment) =>
        comment.dismissed !== true &&
        (comment.severity === 'nit' || comment.severity === 'info'),
    )
    .map((comment) => comment.id);
}

/**
 * Comments `d b` targets: bot comments, not already dismissed. With a vendor
 * given, only that vendor's comments.
 */
export function selectBotIds(
  comments: readonly Comment[],
  vendor?: string,
): string[] {
  return comments
    .filter(
      (comment) =>
        comment.dismissed !== true &&
        comment.author_type === 'bot' &&
        // Missing bot_vendor folds into 'unknown' — the same normalization
        // visibleBotVendors uses, so a picker offering 'unknown' matches.
        (vendor === undefined || (comment.bot_vendor ?? 'unknown') === vendor),
    )
    .map((comment) => comment.id);
}

/** Distinct bot vendors among visible (undismissed) comments. */
export function visibleBotVendors(comments: readonly Comment[]): string[] {
  return [
    ...new Set(
      comments
        .filter(
          (comment) =>
            comment.dismissed !== true && comment.author_type === 'bot',
        )
        .map((comment) => comment.bot_vendor ?? 'unknown'),
    ),
  ];
}

/** Push a batch onto the session undo stack (immutable). */
export function pushBatch(
  stack: readonly DismissBatch[],
  batch: DismissBatch,
): DismissBatch[] {
  return [...stack, batch];
}

/**
 * Pop the most recent batch (immutable). Returns the batch to undo and the
 * remaining stack; batch is undefined on an empty stack.
 */
export function popBatch(stack: readonly DismissBatch[]): {
  batch: DismissBatch | undefined;
  rest: DismissBatch[];
} {
  if (stack.length === 0) return { batch: undefined, rest: [] };
  return {
    batch: stack[stack.length - 1],
    rest: stack.slice(0, -1),
  };
}

/** Status-line report for a completed dismiss batch. */
export function dismissReport(batch: DismissBatch): string {
  const dismissed = `Dismissed ${batch.dismissed_ids.length}`;
  if (batch.resolved_thread_ids.length === 0) return dismissed;
  return `${dismissed} · ${batch.resolved_thread_ids.length} resolved on GitHub`;
}
