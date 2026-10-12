import { Fault, ResultAsync } from '@thourum/fault';
import { useCallback } from 'react';
import { z } from 'zod';
import { appStore } from '../store';
import { useWebSocket } from './useWebSocket';

const actionResponseSchema = z.object({
  success: z.boolean(),
  error: z.string().optional(),
});

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

    if (send('APPROVE_PR', { pr_id: prId }) === undefined) {
      appStore.setState((s) => ({
        ...s,
        pendingAction: null,
        actionResult: {
          type: 'approve',
          prId,
          success: false,
          message: 'WebSocket not connected',
        },
      }));
    }
  }, [prId, send]);

  const dispatchRequestChanges = useCallback(
    (comment: string) => {
      if (prId === undefined) return;

      appStore.setState((s) => ({
        ...s,
        pendingAction: { type: 'request_changes', prId, startedAt: Date.now() },
        confirmingAction: null,
      }));

      if (send('REQUEST_CHANGES', { pr_id: prId, comment }) === undefined) {
        appStore.setState((s) => ({
          ...s,
          pendingAction: null,
          actionResult: {
            type: 'request_changes',
            prId,
            success: false,
            message: 'WebSocket not connected',
          },
        }));
      }
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

      void ResultAsync.fromPromise(
        request('SUBMIT_ACTION', {
          pr_id: prId,
          action: 'post_comment',
          body: trimmed,
        }),
        (cause) =>
          new Fault(cause instanceof Error ? cause : 'Comment failed')
            .withTag('NETWORK_ERROR')
            .withCause(cause),
      ).match(
        (data) => {
          const parsed = actionResponseSchema.safeParse(data);
          const success = parsed.success && parsed.data.success;
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'post_comment',
              prId,
              success,
              message: success
                ? 'Comment posted'
                : ((parsed.success ? parsed.data.error : null) ??
                  'Comment failed'),
            },
            commentsRevision: success
              ? s.commentsRevision + 1
              : s.commentsRevision,
          }));
        },
        (message) => {
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'post_comment',
              prId,
              success: false,
              message: message.message,
            },
          }));
        },
      );
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

      void ResultAsync.fromPromise(
        request('SUBMIT_ACTION', {
          pr_id: prId,
          action: 'reply_comment',
          body: trimmed,
          thread_id: target.threadId ?? '',
          comment_id: target.commentId ?? '',
        }),
        (cause) =>
          new Fault(cause instanceof Error ? cause : 'Reply failed')
            .withTag('NETWORK_ERROR')
            .withCause(cause),
      ).match(
        (data) => {
          const parsed = actionResponseSchema.safeParse(data);
          const success = parsed.success && parsed.data.success;
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'reply_comment',
              prId,
              success,
              message: success
                ? 'Reply posted'
                : ((parsed.success ? parsed.data.error : null) ??
                  'Reply failed'),
            },
            commentsRevision: success
              ? s.commentsRevision + 1
              : s.commentsRevision,
            threadsRevision: success
              ? s.threadsRevision + 1
              : s.threadsRevision,
          }));
        },
        (message) => {
          appStore.setState((s) => ({
            ...s,
            pendingAction: null,
            actionResult: {
              type: 'reply_comment',
              prId,
              success: false,
              message: message.message,
            },
          }));
        },
      );
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
