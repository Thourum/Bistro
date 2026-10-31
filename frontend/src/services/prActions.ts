import { type ActionRequest, Threads } from '@bindings/services';
import { appStore } from '../store';
import type { PendingAction } from '../types';
import { callWrite } from './engine/call';
import { recordFirstRunAction } from './firstRunRail';

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
  payload: Readonly<
    Partial<
      Pick<ActionRequest, 'body' | 'thread_id' | 'comment_id' | 'review_event'>
    >
  >;
  successMessage: string;
}

/** Calls Threads.SubmitAction and settles pendingAction/actionResult from its reply. */
function dispatchAction({
  type,
  prId,
  payload,
  successMessage,
}: ActionDispatch): void {
  setPendingAction(type, prId);
  void callWrite(() =>
    Threads.SubmitAction({
      pr_id: prId,
      action: type,
      body: '',
      thread_id: '',
      comment_id: '',
      in_reply_to: '',
      line: 0,
      review_event: '',
      ...payload,
    }),
  ).match(
    () => {
      appStore.setState((s) => ({
        ...s,
        pendingAction: null,
        actionResult: { type, prId, success: true, message: successMessage },
        commentsRevision: s.commentsRevision + 1,
        threadsRevision: s.threadsRevision + 1,
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
  });
}

/** Requests changes; the engine folds pending drafts into the same review. */
export function requestChanges(prId: string, comment: string): void {
  dispatchAction({
    type: 'request_changes',
    prId,
    payload: { body: comment },
    successMessage: 'Changes requested',
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
  });
}
