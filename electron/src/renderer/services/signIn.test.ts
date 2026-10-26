import { okAsync } from '@thourum/fault';
import { afterEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { openUrlInBrowser } from '../utils/openUrl';
import { cancelSignIn, startSignIn } from './signIn';
import { wsClient } from './ws/connection';
import { handleAuthRedirect } from './ws/events/auth';

vi.mock('../components/ToastContainer', () => ({ addToast: vi.fn() }));
vi.mock('../utils/openUrl', () => ({
  openUrlInBrowser: vi.fn(() => okAsync(undefined)),
}));
vi.mock('./ws/connection', () => ({ wsClient: { send: vi.fn() } }));

afterEach(() => {
  appStore.setState(() => initialUIState);
  vi.clearAllMocks();
});

it('ignores the cancelled start response even after a fresh sign-in starts', () => {
  appStore.setState(() => ({ ...initialUIState, isWsOpen: true }));
  vi.mocked(wsClient.send)
    .mockReturnValueOnce('start-a')
    .mockReturnValueOnce('cancel-a')
    .mockReturnValueOnce('start-b');
  const redirect = (id: string) => ({
    id,
    type: 'AUTH_REDIRECT' as const,
    data: {
      provider: 'github',
      url: `https://github.com/login/oauth/authorize?state=${id}`,
    },
  });
  startSignIn('github');
  cancelSignIn();
  handleAuthRedirect(redirect('start-a'));
  expect(appStore.state.authLoading).toBe(false);
  expect(appStore.state.authUrl).toBeNull();
  expect(openUrlInBrowser).not.toHaveBeenCalled();
  startSignIn('github');
  handleAuthRedirect(redirect('start-a'));
  expect(appStore.state.authUrl).toBeNull();
  expect(openUrlInBrowser).not.toHaveBeenCalled();
  handleAuthRedirect(redirect('start-b'));
  expect(appStore.state.authUrl).toBe(redirect('start-b').data.url);
  expect(openUrlInBrowser).toHaveBeenCalledExactlyOnceWith(
    redirect('start-b').data.url,
  );
});
