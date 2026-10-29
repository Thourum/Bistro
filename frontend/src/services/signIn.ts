import { Auth, type AuthIdentity, type AuthStatus } from '@bindings/services';
import { Fault, fromThrowable, ResultAsync } from '@thourum/fault';
import type { CancellablePromise } from '@wailsio/runtime';
import { addToast } from '../components/ToastContainer';
import { PROVIDERS, type ProviderId } from '../providers';
import { appStore } from '../store';
import { createLogger } from '../utils/logger';
import { openUrlInBrowser } from '../utils/openUrl';
import { call, faultReason } from './engine/call';

const log = createLogger('SignIn');
// The engine allows one OAuth attempt; keep its cancellation handle private.
const attempt: { pending: CancellablePromise<unknown> | null } = {
  pending: null,
};

export function handleAuthStatus(status: Readonly<AuthStatus>): void {
  appStore.setState((state) => ({
    ...state,
    authChecked: true,
    isAuthenticated: status.authenticated,
    connectedProviders: status.providers ?? [],
    authExchange:
      status.exchange === 'proxy' || status.exchange === 'local'
        ? status.exchange
        : null,
  }));
}

export function handleAuthCheckFailed(fault: Fault): void {
  log.error({ err: fault.message }, 'authentication check failed');
  appStore.setState((state) => ({
    ...state,
    authChecked: true,
    isAuthenticated: false,
    syncStatus: 'error',
    syncError: fault.message,
  }));
}

export function checkAuth(): Promise<void> {
  return call(() => Auth.Check()).match(
    handleAuthStatus,
    handleAuthCheckFailed,
  );
}

function finishSignIn(identity: Readonly<AuthIdentity>): void {
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
    syncError: null,
    syncStatus: 'connected',
    connectedProviders: [
      ...state.connectedProviders.filter(
        (provider) => provider.provider !== identity.provider,
      ),
      identity,
    ],
  }));
  void checkAuth();
}

function failSignIn(fault: Fault): void {
  const reason = faultReason(fault);
  const isCancelled = fault.tag === 'CANCELLED' || reason === 'cancelled';
  const isTimeout = fault.tag === 'TIMEOUT_ERROR' || reason === 'timeout';
  appStore.setState((state) => ({
    ...state,
    authLoading: false,
    authUrl: null,
    authStartedAt: null,
    authTimedOut: isTimeout,
    authInterrupted: isCancelled && state.authInterrupted,
    authError: isCancelled || isTimeout ? null : fault.message,
  }));
  if (!isCancelled && !isTimeout) showSignInError(fault);
}

function showSignInError(fault: Fault): void {
  addToast({ type: 'error', message: fault.message, dismissible: true });
}

export function startSignIn(provider: ProviderId): void {
  const definition = PROVIDERS.find((entry) => entry.id === provider);
  if (!definition?.isAvailable || appStore.state.authLoading) return;
  appStore.setState((state) => ({
    ...state,
    authLoading: true,
    authError: null,
    authTimedOut: false,
    authInterrupted: false,
    authUrl: null,
    authStartedAt: null,
  }));
  void call(() => Auth.Start({ provider }), {
    timeoutMs: null,
    onStart: (promise) => {
      attempt.pending = promise;
    },
  }).match(
    (identity) => {
      attempt.pending = null;
      finishSignIn(identity);
    },
    (fault) => {
      attempt.pending = null;
      failSignIn(fault);
    },
  );
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
  void attempt.pending?.cancel();
}

export function retrySignIn(): void {
  startSignIn('github');
}
