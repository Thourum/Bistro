import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import { formatErrorMessage } from '../../../utils/error';

export function handleActionComplete({
  data,
}: Readonly<WSInboundMessage<'ACTION_COMPLETE'>>): void {
  appStore.setState((state) => ({
    ...state,
    pendingAction: null,
    actionResult: {
      type: data.action_type,
      prId: data.pr_id,
      success: true,
      message: data.message,
    },
    confirmingAction: null,
    lastSyncAt: new Date().toISOString(),
  }));
}

export function handleActionFailed({
  data,
}: Readonly<WSInboundMessage<'ACTION_FAILED'>>): void {
  appStore.setState((state) => ({
    ...state,
    pendingAction: null,
    actionResult: {
      type: data.action_type,
      prId: data.pr_id,
      success: false,
      message: data.error,
    },
    confirmingAction: null,
  }));
  addToast({
    type: 'error',
    message: formatErrorMessage(data.error),
    dismissible: true,
  });
}
