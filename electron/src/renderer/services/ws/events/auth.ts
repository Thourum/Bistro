import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import { formatErrorMessage } from '../../../utils/error';
import { openUrlInBrowser } from '../../../utils/openUrl';

export function handleAuthExpired({
  data,
}: Readonly<WSInboundMessage<'AUTH_TOKEN_EXPIRED'>>): void {
  appStore.setState((state) => ({
    ...state,
    syncStatus: 'error',
    syncError: `Auth expired (${data.provider}): ${data.message}`,
    isAuthenticated: false,
  }));
  addToast({
    type: 'error',
    message: formatErrorMessage('AUTH_TOKEN_EXPIRED'),
    dismissible: true,
  });
}

export function handleAuthRedirect({
  data,
}: Readonly<WSInboundMessage<'AUTH_REDIRECT'>>): void {
  appStore.setState((state) => ({
    ...state,
    authUrl: data.url,
    authStartedAt: Date.now(),
    authLoading: true,
    authError: null,
    authTimedOut: false,
    authInterrupted: false,
  }));
  void openUrlInBrowser(data.url).match(
    () => undefined,
    (fault) =>
      addToast({ type: 'error', message: fault.message, dismissible: true }),
  );
}

export function handleAuthCheckResult({
  data,
}: Readonly<WSInboundMessage<'AUTH_CHECK_RESULT'>>): void {
  appStore.setState((state) => ({
    ...state,
    authChecked: true,
    isAuthenticated: data.authenticated,
    connectedProviders: data.providers ?? [],
    authExchange: data.exchange ?? null,
  }));
}

export function handleAuthDisconnectResult({
  data,
}: Readonly<WSInboundMessage<'AUTH_DISCONNECT_RESULT'>>): void {
  appStore.setState((state) => ({
    ...state,
    isAuthenticated: data.authenticated,
    connectedProviders: data.providers ?? [],
  }));
}

export function handleAuthSuccess({
  data,
}: Readonly<WSInboundMessage<'AUTH_SUCCESS'>>): void {
  appStore.setState((state) => ({
    ...state,
    isAuthenticated: true,
    authChecked: true,
    authLoading: false,
    authUrl: null,
    authStartedAt: null,
    authTimedOut: false,
    authInterrupted: false,
    authError: null,
    connectedProviders: [
      ...state.connectedProviders.filter(
        (provider) => provider.provider !== data.provider,
      ),
      { provider: data.provider, username: data.username ?? '' },
    ],
  }));
}

export function handleAuthFailed({
  data,
}: Readonly<WSInboundMessage<'AUTH_FAILED'>>): void {
  const isCancelled = data.error === 'cancelled';
  const isTimeout = data.error === 'timeout';
  appStore.setState((state) => ({
    ...state,
    authLoading: false,
    authUrl: null,
    authStartedAt: null,
    authTimedOut: isTimeout,
    // The reconnect cleanup cancel must not erase the interruption notice.
    authInterrupted: isCancelled && state.authInterrupted,
    authError: isCancelled || isTimeout ? null : data.error,
  }));
  if (!isCancelled && !isTimeout) {
    addToast({ type: 'error', message: data.error, dismissible: true });
  }
}
