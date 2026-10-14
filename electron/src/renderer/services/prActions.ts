import { Fault } from '@thourum/fault';
import { z } from 'zod';
import { appStore } from '../store';
import type { PendingAction } from '../types';
import { wsClient } from './ws/connection';

const actionResponseSchema = z.object({
  success: z.boolean(),
  error: z.string().optional(),
});

type ActionType = PendingAction['type'];

interface ReplyTarget {
  threadId?: string;
  commentId?: string;
}

function setPendingAction(type: ActionType, prId: string): void {
  appStore.setState((s) => ({
    ...s,
    pendingAction: { type, prId, startedAt: Date.now() },
    confirmingAction: null,
  }));
}

function setFailedAction(
  type: ActionType,
  prId: string,
  message: string,
): void {
  appStore.setState((s) => ({
    ...s,
    pendingAction: null,
    actionResult: { type, prId, success: false, message },
  }));
}

/** Dispatches approval; ACTION_COMPLETE / ACTION_FAILED finish the store transition. */
export function approvePR(prId: string): void {
  setPendingAction('approve', prId);
  if (wsClient.send('APPROVE_PR', { pr_id: prId }) === undefined) {
    setFailedAction('approve', prId, 'WebSocket not connected');
  }
}

/** Dispatches a change request; push events finish the store transition. */
export function requestChanges(prId: string, comment: string): void {
  setPendingAction('request_changes', prId);
  if (
    wsClient.send('REQUEST_CHANGES', { pr_id: prId, comment }) === undefined
  ) {
    setFailedAction('request_changes', prId, 'WebSocket not connected');
  }
}

/** Clears the last PR action's feedback. */
export function clearActionResult(): void {
  appStore.setState((s) => ({ ...s, actionResult: null }));
}

/** Posts a PR comment and refreshes comments after a successful response. */
export function postComment(prId: string, body: string): void {
  const trimmed = body.trim();
  if (trimmed.length === 0) return;
  setPendingAction('post_comment', prId);

  void wsClient
    .request('SUBMIT_ACTION', {
      pr_id: prId,
      action: 'post_comment',
      body: trimmed,
    })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .match(
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
      (message) => setFailedAction('post_comment', prId, message.message),
    );
}

/** Replies to a comment and refreshes comments and threads on success. */
export function replyComment(
  prId: string,
  body: string,
  target: Readonly<ReplyTarget>,
): void {
  const trimmed = body.trim();
  if (trimmed.length === 0) return;
  setPendingAction('reply_comment', prId);

  void wsClient
    .request('SUBMIT_ACTION', {
      pr_id: prId,
      action: 'reply_comment',
      body: trimmed,
      thread_id: target.threadId ?? '',
      comment_id: target.commentId ?? '',
    })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .match(
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
              : ((parsed.success ? parsed.data.error : null) ?? 'Reply failed'),
          },
          commentsRevision: success
            ? s.commentsRevision + 1
            : s.commentsRevision,
          threadsRevision: success ? s.threadsRevision + 1 : s.threadsRevision,
        }));
      },
      (message) => setFailedAction('reply_comment', prId, message.message),
    );
}
