import { Fault, fromThrowable, ResultAsync } from '@thourum/fault';
import { addToast } from '../components/ToastContainer';
import { PROVIDERS, type ProviderId } from '../providers';
import { appStore } from '../store';
import { openUrlInBrowser } from '../utils/openUrl';
import { wsClient } from './ws/connection';

function showSignInError(fault: Fault): void {
  addToast({ type: 'error', message: fault.message, dismissible: true });
}

export function startSignIn(provider: ProviderId): void {
  const definition = PROVIDERS.find((entry) => entry.id === provider);
  if (
    !definition?.isAvailable ||
    !appStore.state.isWsOpen ||
    appStore.state.authLoading
  )
    return;
  appStore.setState((state) => ({
    ...state,
    authLoading: true,
    authError: null,
    authTimedOut: false,
    authInterrupted: false,
    authUrl: null,
    authStartedAt: null,
  }));
  const sentId = wsClient.send('AUTH_START', { provider });
  appStore.setState((state) => ({
    ...state,
    authStartRequestId: sentId ?? null,
  }));
  if (sentId === undefined) {
    appStore.setState((state) => ({
      ...state,
      authLoading: false,
      authError: 'Cannot connect to sync engine.',
    }));
  }
}

export function reopenSignIn(): void {
  const url = appStore.state.authUrl;
  if (url === null) return;
  void openUrlInBrowser(url).match(() => undefined, showSignInError);
}

function clipboardFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause.message : String(cause))
    .withTag('CLIPBOARD_ERROR')
    .withCause(cause);
}

export function copySignInLink(): void {
  const url = appStore.state.authUrl;
  if (url === null) return;
  void fromThrowable(() => navigator.clipboard.writeText(url), clipboardFault)()
    .asyncAndThen((promise) => ResultAsync.fromPromise(promise, clipboardFault))
    .match(() => undefined, showSignInError);
}

export function cancelSignIn(): void {
  if (appStore.state.authUrl !== null) {
    if (!appStore.state.isWsOpen) return;
    const sentId = wsClient.send('AUTH_CANCEL', {});
    if (sentId === undefined) {
      addToast({
        type: 'error',
        message: 'Cannot cancel while engine is disconnected.',
        dismissible: true,
      });
    }
    return;
  }
  appStore.setState((state) => ({ ...state, authStartRequestId: null }));
  // Before AUTH_REDIRECT the engine may still start an attempt; cancel it too (no-op when none).
  if (appStore.state.authLoading && appStore.state.isWsOpen) {
    wsClient.send('AUTH_CANCEL', {});
  }
  appStore.setState((state) => ({
    ...state,
    authLoading: false,
    authTimedOut: false,
    authInterrupted: false,
    authError: null,
  }));
}

export function retrySignIn(): void {
  startSignIn('github');
}
