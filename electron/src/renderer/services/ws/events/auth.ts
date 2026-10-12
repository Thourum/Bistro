import { Fault, ResultAsync } from '@thourum/fault';
import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import { formatErrorMessage } from '../../../utils/error';

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

function handleAuthRedirectError(message: string): void {
  appStore.setState((state) => ({
    ...state,
    authLoading: false,
    authError: message,
  }));
  addToast({ type: 'error', message, dismissible: true });
}

export function handleAuthRedirect({
  data,
}: Readonly<WSInboundMessage<'AUTH_REDIRECT'>>): void {
  const openExternal = window.electron?.openExternal;
  if (!openExternal) {
    appStore.setState((state) => ({
      ...state,
      authLoading: false,
      authError: 'Browser launch is unavailable.',
    }));
    return;
  }
  void ResultAsync.fromPromise(openExternal(data.url), (cause) =>
    new Fault(cause instanceof Error ? cause : String(cause))
      .withTag('EXTERNAL_ERROR')
      .withCause(cause),
  ).match(
    (result) => {
      if (result && 'err' in result) {
        handleAuthRedirectError(result.err);
        return;
      }
      appStore.setState((state) => ({
        ...state,
        authLoading: true,
        authError: null,
      }));
    },
    (fault) => handleAuthRedirectError(fault.message),
  );
}

export function handleAuthCheckResult({
  data,
}: Readonly<WSInboundMessage<'AUTH_CHECK_RESULT'>>): void {
  appStore.setState((state) => ({
    ...state,
    authChecked: true,
    authLoading: false,
    isAuthenticated: data.authenticated,
    connectedProviders: data.providers ?? [],
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
  appStore.setState((state) => ({
    ...state,
    authLoading: false,
    authError: data.error,
  }));
  addToast({ type: 'error', message: data.error, dismissible: true });
}
