import { Fault } from '@thourum/fault';
import { z } from 'zod';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import {
  type DismissBatch,
  dismissReport,
  popBatch,
  pushBatch,
} from '../utils/dismissals';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';
import { wsClient } from './ws/connection';

const log = createLogger('dismissals');

const dismissBatchSchema = z.object({
  batch_id: z.string(),
  dismissed_ids: z
    .array(z.string())
    .nullish()
    .transform((ids) => ids ?? []),
  resolved_thread_ids: z
    .array(z.string())
    .nullish()
    .transform((ids) => ids ?? []),
}) satisfies z.ZodType<DismissBatch>;

function bumpCommentsRevision(): void {
  appStore.setState((s) => ({
    ...s,
    commentsRevision: s.commentsRevision + 1,
  }));
}

/** Dismisses a batch and retains it for session undo. */
export function dismissComments(
  prId: string,
  commentIds: readonly string[],
): void {
  if (commentIds.length === 0) return;
  void wsClient
    .request('DISMISS_COMMENTS', {
      pr_id: prId,
      comment_ids: commentIds,
    })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .andThen((data) => parseResponse(dismissBatchSchema, data))
    .match(
      (batch) => {
        appStore.setState((s) => ({
          ...s,
          dismissBatches: pushBatch(s.dismissBatches, batch),
        }));
        addToast({
          type: 'info',
          message: dismissReport(batch),
          dismissible: true,
        });
        bumpCommentsRevision();
      },
      (err) => {
        log.warn({ err: err.message }, 'dismiss failed');
        addToast({
          type: 'error',
          message: 'Dismiss failed',
          dismissible: true,
        });
      },
    );
}

/** Restores the last dismissed batch, leaving it available to retry on failure. */
export function undoDismiss(): void {
  const { batch, rest } = popBatch(appStore.state.dismissBatches);
  if (batch === undefined) return;
  appStore.setState((s) => ({ ...s, dismissBatches: rest }));
  void wsClient
    .request('UNDISMISS_COMMENTS', {
      comment_ids: batch.dismissed_ids,
      unresolve_thread_ids: batch.resolved_thread_ids,
    })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .match(
      () => {
        addToast({
          type: 'info',
          message: `Restored ${batch.dismissed_ids.length}`,
          dismissible: true,
        });
        bumpCommentsRevision();
      },
      (err) => {
        log.warn({ err: err.message }, 'undo failed');
        // Put the batch back so `u` can retry.
        appStore.setState((s) => ({
          ...s,
          dismissBatches: pushBatch(s.dismissBatches, batch),
        }));
        addToast({ type: 'error', message: 'Undo failed', dismissible: true });
      },
    );
}
