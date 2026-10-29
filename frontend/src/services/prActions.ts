import { Fault } from '@thourum/fault';
import { z } from 'zod';
import { appStore } from '../store';
import type { PendingAction } from '../types';
import { recordFirstRunAction } from './firstRunRail';
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

export type ReviewEvent = 'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT';

interface ActionDispatch {
  type: ActionType;
  prId: string;
  payload: Readonly<Record<string, unknown>>;
  successMessage: string;
  failureMessage: string;
}

/** Sends SUBMIT_ACTION and settles pendingAction/actionResult from its reply. */
function dispatchAction({
  type,
  prId,
  payload,
  successMessage,
  failureMessage,
}: ActionDispatch): void {
  setPendingAction(type, prId);
  void wsClient
    .request('SUBMIT_ACTION', { pr_id: prId, action: type, ...payload })
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
            type,
            prId,
            success,
            message: success
              ? successMessage
              : ((parsed.success ? parsed.data.error : null) ?? failureMessage),
          },
          commentsRevision: success
            ? s.commentsRevision + 1
            : s.commentsRevision,
          threadsRevision: success ? s.threadsRevision + 1 : s.threadsRevision,
        }));
      },
      (fault) => setFailedAction(type, prId, fault.message),
    );
}

/** Approves; the engine folds pending drafts into the same review. */
export function approvePR(prId: string): void {
  recordFirstRunAction('detail.approve');
  dispatchAction({
    type: 'approve',
    prId,
    payload: {},
    successMessage: 'Approved',
    failureMessage: 'Approval failed',
  });
}

/** Requests changes; the engine folds pending drafts into the same review. */
export function requestChanges(prId: string, comment: string): void {
  dispatchAction({
    type: 'request_changes',
    prId,
    payload: { body: comment },
    successMessage: 'Changes requested',
    failureMessage: 'Request changes failed',
  });
}

/** Submits pending drafts as one review with the given event. */
export function submitReview(
  prId: string,
  event: ReviewEvent,
  body = '',
): void {
  dispatchAction({
    type: 'submit_review',
    prId,
    payload: { review_event: event, body },
    successMessage: 'Review submitted',
    failureMessage: 'Review failed',
  });
}

/** Clears the last PR action's feedback. */
export function clearActionResult(): void {
  appStore.setState((s) => ({ ...s, actionResult: null }));
}

/** Posts a PR comment and refreshes comments after a successful response. */
export function postComment(prId: string, body: string): void {
  const trimmed = body.trim();
  if (trimmed.length === 0) return;
  recordFirstRunAction('detail.comment');
  dispatchAction({
    type: 'post_comment',
    prId,
    payload: { body: trimmed },
    successMessage: 'Comment posted',
    failureMessage: 'Comment failed',
  });
}

/** Replies to a comment and refreshes comments and threads on success. */
export function replyComment(
  prId: string,
  body: string,
  target: Readonly<ReplyTarget>,
): void {
  const trimmed = body.trim();
  if (trimmed.length === 0) return;
  dispatchAction({
    type: 'reply_comment',
    prId,
    payload: {
      body: trimmed,
      thread_id: target.threadId ?? '',
      comment_id: target.commentId ?? '',
    },
    successMessage: 'Reply posted',
    failureMessage: 'Reply failed',
  });
}
