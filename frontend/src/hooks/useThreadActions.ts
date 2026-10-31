import { Threads } from '@bindings/services';
import { useCallback, useState } from 'react';
import { addToast } from '../components/ToastContainer';
import { callWrite, faultReason } from '../services/engine/call';

/**
 * Dispatches resolve / unresolve writes for review threads.
 *
 * Both route through the existing offline action queue via `Threads.SubmitAction`
 * (the same envelope `approve` / `request_changes` use). The Go side validates
 * the PR id and rejects `state_known=false` threads; we additionally gate the
 * affordance in the UI so the user never fires an action the backend will
 * refuse.
 *
 * The hook tracks which thread ids currently have an in-flight write so the
 * view can disable that row's button and show a pending label. The set is
 * cleared when the binding settles (success, failure, or
 * queued-for-retry) or after a short fallback timeout, so a dropped response
 * never wedges a button forever. Successful thread events still bump the shared
 * threads revision through the engine event handlers, causing the view to re-fetch.
 */
const PENDING_FALLBACK_MS = 8000;

export function useThreadActions(prId: string | undefined) {
  const [pendingThreadIds, setPendingThreadIds] = useState<Set<string>>(
    () => new Set(),
  );

  const clearPending = useCallback((threadId: string) => {
    setPendingThreadIds((prev) => {
      if (!prev.has(threadId)) return prev;
      const next = new Set(prev);
      next.delete(threadId);
      return next;
    });
  }, []);

  const dispatch = useCallback(
    (action: 'resolve_thread' | 'unresolve_thread', threadId: string) => {
      if (prId === undefined || threadId.length === 0) return;
      if (pendingThreadIds.has(threadId)) return;

      setPendingThreadIds((prev) => {
        const next = new Set(prev);
        next.add(threadId);
        return next;
      });

      const label = action === 'resolve_thread' ? 'resolve' : 'unresolve';

      // `thread_id` is the canonical field; `body` is kept as a compatibility
      // alias for older sync engines that only understood ActionRequest.Body.
      void callWrite(() =>
        Threads.SubmitAction({
          pr_id: prId,
          action,
          thread_id: threadId,
          body: threadId,
          comment_id: '',
          in_reply_to: '',
          line: 0,
          review_event: '',
        }),
      ).match(
        () => {
          clearPending(threadId);
          addToast({
            type: 'success',
            message: `Thread ${label}d`,
            dismissible: false,
          });
        },
        (error) => {
          clearPending(threadId);
          const message = error.message;
          const isQueued = faultReason(error) === 'action_queued';
          addToast({
            type: isQueued ? 'warning' : 'error',
            message: isQueued ? `Thread ${label} queued for retry` : message,
            dismissible: true,
          });
        },
      );

      // Self-heal: if no push event arrives (engine offline, queued for retry),
      // release the pending lock so the button becomes usable again.
      setTimeout(() => {
        setPendingThreadIds((prev) => {
          if (!prev.has(threadId)) return prev;
          const next = new Set(prev);
          next.delete(threadId);
          return next;
        });
      }, PENDING_FALLBACK_MS);
    },
    [prId, pendingThreadIds, clearPending],
  );

  const resolveThread = useCallback(
    (threadId: string) => dispatch('resolve_thread', threadId),
    [dispatch],
  );

  const unresolveThread = useCallback(
    (threadId: string) => dispatch('unresolve_thread', threadId),
    [dispatch],
  );

  const isThreadPending = useCallback(
    (threadId: string): boolean => pendingThreadIds.has(threadId),
    [pendingThreadIds],
  );

  return { resolveThread, unresolveThread, isThreadPending };
}
