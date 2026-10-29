import { Repos, ReviewQueue } from '@bindings/services';
import { errAsync, Fault, okAsync } from '@thourum/fault';
import { beforeEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import { initialUIState } from '../store/ui';
import { cancellable } from '../test/cancellable';
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
import { loadRepos } from './repos';
import { wsClient } from './ws/connection';

vi.mock('@bindings/services', () => ({
  Repos: { List: vi.fn(), SetWatched: vi.fn() },
  ReviewQueue: { Get: vi.fn() },
}));
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
  vi.mocked(Repos.SetWatched).mockReturnValue(
    cancellable(Promise.resolve({ repo_id: 'github:acme/api', watched: true })),
  );
  commitOnboardingSelection(selected());
  expect(Repos.SetWatched).toHaveBeenCalledExactlyOnceWith({
    repo_id: 'github:acme/api',
    watched: true,
  });
});

it('seals the selection and advances without waiting for watch replies', () => {
  vi.mocked(Repos.SetWatched).mockReturnValue(
    cancellable(Promise.resolve({ repo_id: 'github:acme/api', watched: true })),
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
  vi.mocked(Repos.SetWatched).mockReturnValue(
    cancellable(Promise.reject(new Error('offline'))),
  );
  commitOnboardingSelection(selected());
  await vi.waitFor(() =>
    expect(appStore.state.toasts.map((toast) => toast.message)).toEqual([
      "Couldn't switch acme/api on. Change it in Settings → Repos.",
    ]),
  );
});

it('loads cached repos into the store and reads the generated review queue', async () => {
  vi.mocked(Repos.List).mockReturnValue(
    cancellable(Promise.resolve({ repos: [...selected().repos] })),
  );
  expect((await requestCachedOnboardingRepos())._unsafeUnwrap()).toEqual(
    selected().repos,
  );
  expect(appStore.state.repos).toEqual(selected().repos);
  expect(appStore.state.reposLoading).toBe(false);
  expect(Repos.List).toHaveBeenCalledWith({ cache_only: true });
  vi.mocked(ReviewQueue.Get).mockReturnValue(
    cancellable(Promise.resolve({ requested: null, recently_reviewed: null })),
  );
  expect((await requestOnboardingReviewQueue())._unsafeUnwrap()).toEqual({
    requested: null,
    recently_reviewed: null,
  });
});

it('clears repo loading on failure without discarding cached rows', async () => {
  appStore.setState((state) => ({ ...state, repos: [...selected().repos] }));
  vi.mocked(Repos.List).mockReturnValue(
    cancellable(Promise.reject(new Error('offline'))),
  );
  expect((await loadRepos()).isErr()).toBe(true);
  expect(appStore.state.reposLoading).toBe(false);
  expect(appStore.state.repos).toEqual(selected().repos);
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
