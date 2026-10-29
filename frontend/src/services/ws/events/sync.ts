import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import { formatErrorMessage } from '../../../utils/error';
import { createLogger } from '../../../utils/logger';

const log = createLogger('WebSocket');

export function handleSyncComplete({
  data,
}: Readonly<WSInboundMessage<'SYNC_COMPLETE'>>): void {
  appStore.setState((state) => ({
    ...state,
    syncStatus: 'ready',
    lastSyncAt: new Date().toISOString(),
    prListRevision: state.prListRevision + 1,
    syncStats: {
      prCount: data.pr_count,
      newPRs: data.new_prs,
      updatedPRs: data.updated_prs,
    },
  }));
}

export function handleSyncTriggered(
  message: Readonly<WSInboundMessage<'SYNC_TRIGGERED'>>,
): void {
  if (message.data.queued === true) {
    appStore.setState((state) => ({ ...state, syncStatus: 'syncing' }));
    return;
  }
  log.warn(
    { type: message.type, id: message.id },
    'manual sync not queued — sync already in progress',
  );
}

export function handleSyncError({
  data,
}: Readonly<WSInboundMessage<'SYNC_ERROR'>>): void {
  appStore.setState((state) => ({
    ...state,
    syncStatus: 'error',
    syncError: `${data.error_code}: ${data.message}`,
  }));
  addToast({
    type: 'error',
    message: formatErrorMessage(data.error_code),
    dismissible: true,
  });
}

export function handleRateLimited({
  data,
}: Readonly<WSInboundMessage<'RATE_LIMITED'>>): void {
  appStore.setState((state) => ({
    ...state,
    syncStatus: 'rate_limited',
    syncError: `Rate limited until ${data.reset_at}`,
  }));
}
