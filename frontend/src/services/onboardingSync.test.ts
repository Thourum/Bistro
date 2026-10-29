import { errAsync, Fault, okAsync } from '@thourum/fault';
import { beforeEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import { initialUIState } from '../store/ui';
import {
  createOnboardingSelection,
  mergeOnboardingQueue,
  mergeOnboardingRepos,
} from '../utils/onboardingRepos';
import {
  advanceOnboardingToPractice,
  chooseOnboardingFolder,
  commitOnboardingSelection,
  requestCachedOnboardingRepos,
  requestOnboardingReviewQueue,
} from './onboardingSync';
import { wsClient } from './ws/connection';

vi.mock('./ws/connection', () => ({
  wsClient: { request: vi.fn(), send: vi.fn() },
}));
vi.mock('../store/settings', () => ({
  settingsStore: { state: { repositories: { devFolderPath: '' } } },
  updateSettings: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  appStore.setState(() => ({ ...initialUIState }));
});
const selected = () =>
  mergeOnboardingQueue(
    mergeOnboardingRepos(createOnboardingSelection(), [
      {
        id: 'github:acme/api',
        provider: 'github',
        owner: 'acme',
        repo: 'api',
        watched: false,
      },
      {
        id: 'github:acme/web',
        provider: 'github',
        owner: 'acme',
        repo: 'web',
        watched: true,
      },
    ]),
    {
      requested: [{ repo_id: 'github:acme/api', count: 1 }],
      recently_reviewed: [],
    },
  );

it('sends explicit differences and does not send unchanged watch flags', () => {
  vi.mocked(wsClient.request).mockReturnValue(
    okAsync({ repo_id: 'github:acme/api', watched: true }),
  );
  commitOnboardingSelection(selected());
  expect(wsClient.request).toHaveBeenCalledExactlyOnceWith(
    'TOGGLE_WATCH_REPO',
    { repo_id: 'github:acme/api', watched: true },
  );
});

it('seals the selection and advances without waiting for watch replies', () => {
  vi.mocked(wsClient.request).mockReturnValue(
    okAsync({ repo_id: 'github:acme/api', watched: true }),
  );
  appStore.setState((state) => ({
    ...state,
    onboardingStep: 'sync',
    onboardingSync: {
      selection: selected(),
      initialReposPending: false,
      repoError: null,
      queuePending: false,
      queueError: null,
      folder: { status: 'none' },
    },
  }));
  advanceOnboardingToPractice();
  expect(appStore.state.onboardingSync?.selection.sealed).toBe(true);
  expect(appStore.state.onboardingStep).toBe('practice');
  expect(appStore.state.repos).toEqual([]);
});

it('shows the prescribed repo-specific failure at one toast boundary', async () => {
  vi.mocked(wsClient.request).mockReturnValue(
    errAsync(new Fault('offline').withTag('CONNECTION_ERROR')),
  );
  commitOnboardingSelection(selected());
  await vi.waitFor(() =>
    expect(appStore.state.toasts.map((toast) => toast.message)).toEqual([
      "Couldn't switch acme/api on. Change it in Settings → Repos.",
    ]),
  );
});

it('requests only cached repositories and rejects invalid queue counts', async () => {
  vi.mocked(wsClient.request).mockReturnValueOnce(
    okAsync({ repos: selected().repos }),
  );
  expect((await requestCachedOnboardingRepos())._unsafeUnwrap()).toEqual(
    selected().repos,
  );
  expect(wsClient.request).toHaveBeenCalledWith('REQUEST_REPO_LIST', {
    cache_only: true,
  });
  vi.mocked(wsClient.request).mockReturnValueOnce(
    okAsync({
      requested: [{ repo_id: 'repo', count: -1 }],
      recently_reviewed: [],
    }),
  );
  expect((await requestOnboardingReviewQueue()).isErr()).toBe(true);
});

it('keeps cancellation silent and gates scanning on canonical-path settings persistence', async () => {
  const pickFolder = vi.fn().mockResolvedValue(null);
  vi.stubGlobal('window', { electron: { pickFolder } });
  expect((await chooseOnboardingFolder())._unsafeUnwrap()).toBeNull();
  expect(wsClient.request).not.toHaveBeenCalled();
  pickFolder.mockResolvedValue('/projects');
  vi.mocked(wsClient.request).mockReturnValue(
    okAsync({ ok: true, canonical_path: '/canonical' }),
  );
  vi.mocked(updateSettings).mockReturnValue(
    errAsync(new Fault('disk full').withTag('SETTINGS_ERROR')),
  );
  expect((await chooseOnboardingFolder())._unsafeUnwrapErr().message).toBe(
    'disk full',
  );
  expect(updateSettings).toHaveBeenCalledWith({
    repositories: {
      ...settingsStore.state.repositories,
      devFolderPath: '/canonical',
    },
  });
  expect(wsClient.request).toHaveBeenCalledExactlyOnceWith(
    'VALIDATE_DEV_FOLDER',
    { path: '/projects' },
  );
  vi.unstubAllGlobals();
});
