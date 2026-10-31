import type {
  RateLimited,
  SyncComplete,
  SyncError,
  SyncStatus,
} from '@bindings/services/models';
import { addToast } from '../../components/ToastContainer';
import { appStore } from '../../store';
import { formatErrorMessage } from '../../utils/error';

export function handleSyncComplete(_data: Readonly<SyncComplete>): void {
  appStore.setState((state) => ({
    ...state,
    syncStatus: 'ready',
    lastSyncAt: new Date().toISOString(),
    prListRevision: state.prListRevision + 1,
  }));
}

export function handleSyncError(data: Readonly<SyncError>): void {
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

export function handleRateLimited(data: Readonly<RateLimited>): void {
  const MILLISECONDS_PER_SECOND = 1_000;
  const resetAt = new Date(
    Date.now() + data.reset_in * MILLISECONDS_PER_SECOND,
  ).toISOString();
  appStore.setState((state) => ({
    ...state,
    syncStatus: 'rate_limited',
    syncError: `Rate limited until ${resetAt}`,
  }));
}

export function handleSyncStatus(data: Readonly<SyncStatus>): void {
  appStore.setState((state) => ({ ...state, engineSyncStatus: data }));
}
