import { useCallback } from 'react';
import { appStore } from '../store';
import { useWebSocket } from './useWebSocket';

/**
 * Hook that provides dispatch functions for PR actions (approve, request-changes).
 * Sends the action via WebSocket and sets pendingAction in the store.
 * The store fields pendingAction / actionResult are updated by ACTION_COMPLETE / ACTION_FAILED handlers.
 */
export function useActionDispatch(prId: string | undefined) {
  const { request, send } = useWebSocket();

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
    [prId, send],
  );

  const clearActionResult = useCallback(() => {
    appStore.setState((s) => ({ ...s, actionResult: null }));
  }, []);

  const dispatchPostComment = useCallback(
    (body: string) => {
      if (prId === undefined) return;
      const trimmed = body.trim();
      if (trimmed.length === 0) return;

      appStore.setState((s) => ({
        ...s,
        pendingAction: { type: 'post_comment', prId, startedAt: Date.now() },
        confirmingAction: null,
      }));

      void request('SUBMIT_ACTION', {
        pr_id: prId,
        action: 'post_comment',
        body: trimmed,
      })
        .then((data) => {
          const result = data as { success?: boolean; error?: string };
          const success = result.success === true;
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'post_comment',
              prId,
              success,
              message: success
                ? 'Comment posted'
                : (result.error ?? 'Comment failed'),
            },
            commentsRevision: success
              ? s.commentsRevision + 1
              : s.commentsRevision,
          }));
        })
        .catch((error: unknown) => {
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'post_comment',
              prId,
              success: false,
              message:
                error instanceof Error ? error.message : 'Comment failed',
            },
          }));
        });
    },
    [prId, request],
  );

  const dispatchReplyComment = useCallback(
    (body: string, target: { threadId?: string; commentId?: string }) => {
      if (prId === undefined) return;
      const trimmed = body.trim();
      if (trimmed.length === 0) return;

      appStore.setState((s) => ({
        ...s,
        pendingAction: { type: 'reply_comment', prId, startedAt: Date.now() },
        confirmingAction: null,
      }));

      void request('SUBMIT_ACTION', {
        pr_id: prId,
        action: 'reply_comment',
        body: trimmed,
        thread_id: target.threadId ?? '',
        comment_id: target.commentId ?? '',
      })
        .then((data) => {
          const result = data as { success?: boolean; error?: string };
          const success = result.success === true;
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'reply_comment',
              prId,
              success,
              message: success
                ? 'Reply posted'
                : (result.error ?? 'Reply failed'),
            },
            commentsRevision: success
              ? s.commentsRevision + 1
              : s.commentsRevision,
            threadsRevision: success
              ? s.threadsRevision + 1
              : s.threadsRevision,
          }));
        })
        .catch((error: unknown) => {
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'reply_comment',
              prId,
              success: false,
              message: error instanceof Error ? error.message : 'Reply failed',
            },
          }));
        });
    },
    [prId, request],
  );

  return {
    dispatchApprove,
    dispatchRequestChanges,
    dispatchPostComment,
    dispatchReplyComment,
    clearActionResult,
  };
}
