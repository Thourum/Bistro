import { fromPromise } from 'neverthrow';
import { useEffect } from 'react';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import type { Comment } from '../types';
import {
  type DismissBatch,
  dismissReport,
  popBatch,
  pushBatch,
  selectBotIds,
  selectNitIds,
} from '../utils/dismissals';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('useDismissals');

/** DOM events dispatched by the d/u keybindings (emitThreadsEvent pattern). */
export const DISMISS_NITS_EVENT = 'bistro:comments:dismiss-nits';
export const DISMISS_BOTS_EVENT = 'bistro:comments:dismiss-bots';
export const UNDO_DISMISS_EVENT = 'bistro:comments:undo-dismiss';

function bumpCommentsRevision(): void {
  appStore.setState((s) => ({
    ...s,
    commentsRevision: s.commentsRevision + 1,
  }));
}

function parseDismissBatch(data: unknown): DismissBatch | null {
  const payload = data as Partial<DismissBatch> | null;
  if (!payload || typeof payload.batch_id !== 'string') return null;
  return {
    batch_id: payload.batch_id,
    dismissed_ids: payload.dismissed_ids ?? [],
    resolved_thread_ids: payload.resolved_thread_ids ?? [],
  };
}

/**
 * Bulk-dismiss wiring for the PR detail view (Phase 3.5): listens for the
 * d n / d b / u keybinding events, sends DISMISS_COMMENTS / UNDISMISS_COMMENTS
 * over WS, maintains the session undo stack, and bumps commentsRevision so
 * the page re-requests comments. No confirmation dialog by design — the
 * status toast reports what happened and `u` reverses the whole batch.
 */
export function useDismissals(
  prId: string | undefined,
  comments: readonly Comment[],
): void {
  const { request } = useWebSocket();

  useEffect(() => {
    if (prId === undefined) return;

    const dismiss = (commentIds: string[]): void => {
      if (commentIds.length === 0) return;
      void fromPromise(
        request('DISMISS_COMMENTS', { pr_id: prId, comment_ids: commentIds }),
        (err) => err,
      ).match(
        (data) => {
          const batch = parseDismissBatch(data);
          if (batch === null) {
            log.warn({ data }, 'unparseable DISMISS_RESULT');
            return;
          }
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
          log.warn({ err }, 'dismiss failed');
          addToast({
            type: 'error',
            message: 'Dismiss failed',
            dismissible: true,
          });
        },
      );
    };

    const onDismissNits = (): void => {
      dismiss(selectNitIds(comments));
    };

    const onDismissBots = (event: Event): void => {
      const vendor = (event as CustomEvent<{ vendor?: string }>).detail?.vendor;
      dismiss(selectBotIds(comments, vendor));
    };

    const onUndo = (): void => {
      const { batch, rest } = popBatch(appStore.state.dismissBatches);
      if (batch === undefined) return;
      appStore.setState((s) => ({ ...s, dismissBatches: rest }));
      void fromPromise(
        request('UNDISMISS_COMMENTS', {
          comment_ids: batch.dismissed_ids,
          unresolve_thread_ids: batch.resolved_thread_ids,
        }),
        (err) => err,
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
          log.warn({ err }, 'undo failed');
          // Put the batch back so `u` can retry.
          appStore.setState((s) => ({
            ...s,
            dismissBatches: pushBatch(s.dismissBatches, batch),
          }));
          addToast({
            type: 'error',
            message: 'Undo failed',
            dismissible: true,
          });
        },
      );
    };

    document.addEventListener(DISMISS_NITS_EVENT, onDismissNits);
    document.addEventListener(DISMISS_BOTS_EVENT, onDismissBots);
    document.addEventListener(UNDO_DISMISS_EVENT, onUndo);
    return () => {
      document.removeEventListener(DISMISS_NITS_EVENT, onDismissNits);
      document.removeEventListener(DISMISS_BOTS_EVENT, onDismissBots);
      document.removeEventListener(UNDO_DISMISS_EVENT, onUndo);
    };
  }, [prId, comments, request]);
}
