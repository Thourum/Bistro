import { useCallback, useState } from 'react';
import { useWebSocket } from './useWebSocket';

/**
 * Dispatches resolve / unresolve writes for review threads.
 *
 * Both route through the existing offline action queue via `SUBMIT_ACTION`
 * (the same envelope `approve` / `request_changes` use). The Go side validates
 * the PR id and rejects `state_known=false` threads; we additionally gate the
 * affordance in the UI so the user never fires an action the backend will
 * refuse.
 *
 * The hook tracks which thread ids currently have an in-flight write so the
 * view can disable that row's button and show a pending label. The set is
 * cleared when a `THREAD_RESOLVED` / `THREAD_REOPENED` push lands (the threads
 * view re-fetches and the row reflects the new state) or after a short
 * fallback timeout, so a dropped event never wedges a button forever.
 */
const PENDING_FALLBACK_MS = 8000;

export function useThreadActions(prId: string | undefined) {
  const { send } = useWebSocket();
  const [pendingThreadIds, setPendingThreadIds] = useState<Set<string>>(
    () => new Set(),
  );

  const dispatch = useCallback(
    (action: 'resolve_thread' | 'unresolve_thread', threadId: string) => {
      if (prId === undefined || threadId.length === 0) return;
      if (pendingThreadIds.has(threadId)) return;

      setPendingThreadIds((prev) => {
        const next = new Set(prev);
        next.add(threadId);
        return next;
      });

      // `body` carries the thread node id, matching the Go ActionRequest shape.
      send('SUBMIT_ACTION', {
        pr_id: prId,
        action,
        body: threadId,
      });

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
    [prId, send, pendingThreadIds],
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
