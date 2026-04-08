import { useCallback } from 'react';
import { appStore } from '../store';
import { useWebSocket } from './useWebSocket';

/**
 * Hook that provides dispatch functions for PR actions (approve, request-changes).
 * Sends the action via WebSocket and sets pendingAction in the store.
 * The store fields pendingAction / actionResult are updated by ACTION_COMPLETE / ACTION_FAILED handlers.
 */
export function useActionDispatch(prId: string | undefined) {
  const { send } = useWebSocket();

  const dispatchApprove = useCallback(() => {
    if (prId === undefined) return;

    appStore.setState((s) => ({
      ...s,
      pendingAction: { type: 'approve', prId, startedAt: Date.now() },
      confirmingAction: null,
    }));

    send('APPROVE_PR', { pr_id: prId });
  }, [prId, send]);

  const dispatchRequestChanges = useCallback(
    (comment: string) => {
      if (prId === undefined) return;

      appStore.setState((s) => ({
        ...s,
        pendingAction: { type: 'request_changes', prId, startedAt: Date.now() },
        confirmingAction: null,
      }));

      send('REQUEST_CHANGES', { pr_id: prId, comment });
    },
    [prId, send]
  );

  const clearActionResult = useCallback(() => {
    appStore.setState((s) => ({ ...s, actionResult: null }));
  }, []);

  return { dispatchApprove, dispatchRequestChanges, clearActionResult };
}
