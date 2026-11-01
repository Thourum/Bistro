// @vitest-environment jsdom
import {
  DevFolderScan,
  type RepoListResponse,
  Repos,
  ReviewQueue,
  type ScanDevFolderResult,
  type WatchResult,
} from '@bindings/services';
import { Fault, ResultAsync } from '@thourum/fault';
import { act, createElement } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { ReviewQueue as ReviewQueuePayload } from '../../bindings/github.com/itterno/bistro/internal/provider/models';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { cancellable } from '../test/cancellable';
import { useOnboardingSync } from './useOnboardingSync';

vi.mock('@bindings/services', () => ({
  DevFolderScan: { Scan: vi.fn() },
  Repos: { List: vi.fn(), SetWatched: vi.fn() },
  ReviewQueue: { Get: vi.fn() },
}));
vi.mock('../store/settings', () => ({
  settingsStore: { state: { repositories: { devFolderPath: '/dev' } } },
  updateSettings: vi.fn(),
}));

const repo = (name: string, path?: string) => ({
  id: `github:acme/${name}`,
  provider: 'github',
  owner: 'acme',
  repo: name,
  watched: false,
  ...(path === undefined ? {} : { local_path: path }),
});
const queue = {
  requested: [{ repo_id: 'github:acme/api', count: 2 }],
  recently_reviewed: [],
};

function deferredResponse<T = unknown>() {
  let resolve: (response: T) => void = () => undefined;
  let reject: (error: unknown) => void = () => undefined;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return {
    resolve,
    reject,
    binding: cancellable(promise),
    result: ResultAsync.fromPromise(promise, (cause) =>
      new Fault('request failed').withTag('EXTERNAL_ERROR').withCause(cause),
    ),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  appStore.setState(() => ({
    ...initialUIState,
    repos: [repo('clone', '/dev/clone'), repo('cached')],
  }));
});
afterEach(() => vi.unstubAllGlobals());

async function mountSync() {
  const latest: { sync: ReturnType<typeof useOnboardingSync> | null } = {
    sync: null,
  };
  function HookHost() {
    latest.sync = useOnboardingSync();
    return null;
  }
  const root = createRoot(document.createElement('div'));
  await act(async () => {
    root.render(createElement(HookHost));
  });
  return {
    latest,
    unmount: async () => {
      await act(async () => root.unmount());
    },
  };
}

it('shows clone cache immediately and keeps scans alive after Continue and unmount', async () => {
  const list = deferredResponse<RepoListResponse>();
  const review = deferredResponse<ReviewQueuePayload>();
  const refreshed = deferredResponse<RepoListResponse>();
  const scan = deferredResponse<ScanDevFolderResult>();
  vi.mocked(Repos.List)
    .mockReturnValueOnce(list.binding)
    .mockReturnValueOnce(refreshed.binding);
  vi.mocked(ReviewQueue.Get).mockReturnValueOnce(review.binding);
  vi.mocked(DevFolderScan.Scan).mockReturnValueOnce(scan.binding);
  const hook = await mountSync();
  expect(
    hook.latest.sync?.state.selection.repos.map(
      (repository) => repository.repo,
    ),
  ).toEqual(['clone']);
  expect(hook.latest.sync?.canContinue).toBe(true);
  expect(hook.latest.sync?.state.folder).toEqual({
    status: 'done',
    path: '/dev',
    partial: false,
  });
  expect(DevFolderScan.Scan).not.toHaveBeenCalled();
  await act(async () => {
    list.resolve({ repos: [repo('api'), repo('clone', '/dev/clone')] });
  });
  expect(
    hook.latest.sync?.state.selection.repos.map(
      (repository) => repository.repo,
    ),
  ).toEqual(['clone']);
  await act(async () => {
    review.resolve(queue);
  });
  expect(hook.latest.sync?.state.selection.selected['github:acme/api']).toBe(
    true,
  );
  expect(hook.latest.sync?.canContinue).toBe(true);
  await act(async () => {
    refreshed.resolve({ repos: [repo('api'), repo('clone', '/dev/clone')] });
  });
  expect(hook.latest.sync?.canContinue).toBe(true);
  await act(async () => {
    hook.latest.sync?.scanAgain();
  });
  expect(hook.latest.sync?.state.folder.status).toBe('scanning');
  // One independent dispatch for the default-on choice; never wait on its reply.
  vi.mocked(Repos.SetWatched).mockReturnValueOnce(
    deferredResponse<WatchResult>().binding,
  );
  await act(async () => {
    hook.latest.sync?.continueFromSync();
  });
  expect(appStore.state.onboardingStep).toBe('practice');
  expect(appStore.state.onboardingSync?.selection.sealed).toBe(true);
  await hook.unmount();
  expect(scan.binding.cancel).not.toHaveBeenCalled();
  const requests = vi.mocked(DevFolderScan.Scan).mock.calls.length;
  await act(async () => {
    scan.resolve({
      discovered: 1,
      healthy: 1,
      degraded: 0,
      skipped: 0,
      partial: true,
    });
  });
  expect(appStore.state.onboardingSync).toBeNull();
  expect(DevFolderScan.Scan).toHaveBeenCalledTimes(requests);
});

it('continues from cached clones while queue discovery is pending and seals choices against late defaults', async () => {
  const list = deferredResponse<RepoListResponse>();
  const review = deferredResponse<ReviewQueuePayload>();
  const refreshed = deferredResponse<RepoListResponse>();
  vi.mocked(Repos.List)
    .mockReturnValueOnce(list.binding)
    .mockReturnValueOnce(refreshed.binding);
  vi.mocked(ReviewQueue.Get).mockReturnValueOnce(review.binding);
  const hook = await mountSync();
  await act(async () => {
    list.resolve({ repos: [repo('clone', '/dev/clone')] });
  });
  expect(hook.latest.sync?.state.queuePending).toBe(true);
  expect(hook.latest.sync?.canContinue).toBe(true);
  const selection = hook.latest.sync?.state.selection.selected;
  await act(async () => hook.latest.sync?.continueFromSync());
  expect(appStore.state.onboardingStep).toBe('practice');
  expect(hook.latest.sync?.state.selection.sealed).toBe(true);
  await act(async () => {
    review.resolve({
      requested: [
        ...queue.requested,
        { repo_id: 'github:acme/clone', count: 1 },
      ],
      recently_reviewed: [],
    });
  });
  await act(async () => {
    refreshed.resolve({
      repos: [
        { ...repo('api'), watched: true },
        { ...repo('clone', '/dev/clone'), watched: true },
      ],
    });
  });
  expect(hook.latest.sync?.state.selection.selected).toBe(selection);
  expect(hook.latest.sync?.state.selection.repos[0]?.watched).toBe(true);
  expect(hook.latest.sync?.canContinue).toBe(false);
  expect(DevFolderScan.Scan).not.toHaveBeenCalled();
  await hook.unmount();
});

it('blocks only the initial empty repo read, not pending queue enrichment', async () => {
  appStore.setState((state) => ({ ...state, repos: [] }));
  const list = deferredResponse<RepoListResponse>();
  vi.mocked(Repos.List).mockReturnValueOnce(list.binding);
  vi.mocked(ReviewQueue.Get).mockReturnValueOnce(
    deferredResponse<ReviewQueuePayload>().binding,
  );
  const hook = await mountSync();
  expect(hook.latest.sync?.canContinue).toBe(false);
  await act(async () => list.resolve({ repos: [] }));
  expect(hook.latest.sync?.state.queuePending).toBe(true);
  expect(hook.latest.sync?.canContinue).toBe(true);
  await act(async () => hook.latest.sync?.continueFromSync());
  expect(appStore.state.onboardingStep).toBe('practice');
  await hook.unmount();
});

it('reveals held nonlocal cache and permits an empty failed read once the queue fails', async () => {
  const list = deferredResponse<RepoListResponse>();
  const review = deferredResponse<ReviewQueuePayload>();
  vi.mocked(Repos.List).mockReturnValueOnce(list.binding);
  vi.mocked(ReviewQueue.Get).mockReturnValueOnce(review.binding);
  const hook = await mountSync();
  await act(async () => {
    list.reject(new Error('request failed'));
    review.reject(new Error('request failed'));
  });
  expect(hook.latest.sync?.state.queueError).toBe('request failed');
  expect(hook.latest.sync?.state.repoError).toBe('request failed');
  expect(
    hook.latest.sync?.state.selection.repos.map(
      (repository) => repository.repo,
    ),
  ).toEqual(['clone', 'cached']);
  expect(hook.latest.sync?.canContinue).toBe(true);
  await hook.unmount();
});

it('only explicit Stop cancels and partial completion refreshes clone metadata', async () => {
  const list = deferredResponse<RepoListResponse>();
  const review = deferredResponse<ReviewQueuePayload>();
  const scan = deferredResponse<ScanDevFolderResult>();
  const refreshed = deferredResponse<RepoListResponse>();
  vi.mocked(Repos.List)
    .mockReturnValueOnce(list.binding)
    .mockReturnValueOnce(refreshed.binding);
  vi.mocked(ReviewQueue.Get).mockReturnValueOnce(review.binding);
  vi.mocked(DevFolderScan.Scan).mockReturnValueOnce(scan.binding);
  const hook = await mountSync();
  await act(async () => {
    list.resolve({ repos: [] });
    review.reject(new Error('request failed'));
  });
  await act(async () => {
    hook.latest.sync?.scanAgain();
  });
  expect(scan.binding.cancel).not.toHaveBeenCalled();
  const otherScan = deferredResponse<ScanDevFolderResult>();
  vi.mocked(DevFolderScan.Scan).mockReturnValueOnce(otherScan.binding);
  DevFolderScan.Scan({ path: '/other', reason: 'startup' });
  await act(async () => hook.latest.sync?.stopScan());
  expect(scan.binding.cancel).toHaveBeenCalledOnce();
  expect(otherScan.binding.cancel).not.toHaveBeenCalled();
  await act(async () => {
    scan.resolve({
      discovered: 1,
      healthy: 1,
      degraded: 0,
      skipped: 0,
      partial: true,
    });
  });
  expect(hook.latest.sync?.state.folder).toEqual({
    status: 'done',
    path: '/dev',
    partial: true,
  });
  await act(async () => {
    refreshed.resolve({ repos: [repo('cached', '/dev/cached')] });
  });
  expect(
    hook.latest.sync?.state.selection.repos.find(
      (repository) => repository.repo === 'cached',
    )?.local_path,
  ).toBe('/dev/cached');
  await hook.unmount();
  expect(scan.binding.cancel).toHaveBeenCalledOnce();
});
