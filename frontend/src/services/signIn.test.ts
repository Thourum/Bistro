import { okAsync } from '@thourum/fault';
import { CancelError } from '@wailsio/runtime';
import { afterEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { cancellable } from '../test/cancellable';
import { handleAuthRedirect } from './events/auth';
import { cancelSignIn, checkAuth, startSignIn } from './signIn';

const { Start, Check } = vi.hoisted(() => ({ Start: vi.fn(), Check: vi.fn() }));
vi.mock('@bindings/services', () => ({ Auth: { Start, Check } }));
vi.mock('../components/ToastContainer', () => ({ addToast: vi.fn() }));
vi.mock('../utils/openUrl', () => ({
  openUrlInBrowser: vi.fn(() => okAsync(undefined)),
}));

afterEach(() => {
  appStore.setState(() => initialUIState);
  vi.clearAllMocks();
});

it('clears loading and the redirect when the pending OAuth call is cancelled', async () => {
  const controls: { reject?: (error: unknown) => void } = {};
  const pending = cancellable(
    new Promise<never>((_, reject) => {
      controls.reject = reject;
    }),
  );
  pending.cancel.mockImplementation(() =>
    controls.reject?.(new CancelError('cancelled')),
  );
  Start.mockReturnValue(pending);
  startSignIn('github');
  handleAuthRedirect({
    provider: 'github',
    url: 'https://github.com/login/oauth/authorize',
  });
  expect(appStore.state.authLoading).toBe(true);
  cancelSignIn();
  await vi.waitFor(() => expect(appStore.state.authLoading).toBe(false));
  expect(appStore.state.authUrl).toBeNull();
  expect(appStore.state.authError).toBeNull();
});

it('finishes the startup gate and exposes a rejected auth check', async () => {
  Check.mockReturnValue(
    cancellable(Promise.reject(new Error('keychain unavailable'))),
  );
  await checkAuth();
  expect(appStore.state.authChecked).toBe(true);
  expect(appStore.state.isAuthenticated).toBe(false);
  expect(appStore.state.syncStatus).toBe('error');
  expect(appStore.state.syncError).toBe('keychain unavailable');
});

it('authenticates and refreshes local status after OAuth succeeds', async () => {
  Start.mockReturnValue(
    cancellable(Promise.resolve({ provider: 'github', username: 'reviewer' })),
  );
  Check.mockReturnValue(
    cancellable(
      Promise.resolve({
        authenticated: true,
        providers: [{ provider: 'github', username: 'reviewer' }],
        exchange: 'proxy',
      }),
    ),
  );
  startSignIn('github');
  await vi.waitFor(() => expect(appStore.state.authExchange).toBe('proxy'));
  expect(appStore.state.authLoading).toBe(false);
  expect(appStore.state.isAuthenticated).toBe(true);
  expect(appStore.state.connectedProviders).toEqual([
    { provider: 'github', username: 'reviewer' },
  ]);
});
