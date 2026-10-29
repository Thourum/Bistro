import type { ActionComplete } from '@bindings/services/models';
import { addToast } from '../../components/ToastContainer';
import { appStore } from '../../store';
import type { ActionResult } from '../../types';
import { formatErrorMessage } from '../../utils/error';

const ACTION_TYPES = {
  approve: true,
  request_changes: true,
  post_comment: true,
  reply_comment: true,
  submit_review: true,
} satisfies Record<ActionResult['type'], boolean>;

function isActionType(action: string): action is ActionResult['type'] {
  return Object.hasOwn(ACTION_TYPES, action);
}

export function handleActionComplete(data: Readonly<ActionComplete>): void {
  const action = data.action;
  // Thread feedback belongs to the resolve service, not the pending PR review.
  if (action === 'resolve_thread' || action === 'unresolve_thread') return;
  if (!isActionType(action)) {
    appStore.setState((state) => ({ ...state, pendingAction: null }));
    addToast({
      type: 'error',
      message: 'Unknown completed review action',
      dismissible: true,
    });
    return;
  }
  appStore.setState((state) => ({
    ...state,
    pendingAction: null,
    actionResult: {
      type: action,
      prId: data.pr,
      success: data.success,
      message: data.success ? '' : data.error,
    },
    confirmingAction: null,
    lastSyncAt: data.success ? new Date().toISOString() : state.lastSyncAt,
  }));
  if (data.success) return;
  addToast({
    type: 'error',
    message: formatErrorMessage(data.error),
    dismissible: true,
  });
}
