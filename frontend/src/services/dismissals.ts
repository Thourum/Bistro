import { Dismiss } from '@bindings/services';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import {
  type DismissBatch,
  dismissReport,
  popBatch,
  pushBatch,
} from '../utils/dismissals';
import { createLogger } from '../utils/logger';
import { callWrite } from './engine/call';

const log = createLogger('dismissals');

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
  void callWrite(() =>
    Dismiss.Dismiss({
      pr_id: prId,
      comment_ids: [...commentIds],
    }),
  ).match(
    (response) => {
      const batch: DismissBatch = {
        batch_id: response.batch_id,
        dismissed_ids: response.dismissed_ids ?? [],
        resolved_thread_ids: response.resolved_thread_ids ?? [],
      };
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
  void callWrite(() =>
    Dismiss.Undismiss({
      comment_ids: batch.dismissed_ids,
      unresolve_thread_ids: batch.resolved_thread_ids,
    }),
  ).match(
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
