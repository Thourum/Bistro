// @vitest-environment jsdom
import { Fault, ResultAsync } from '@thourum/fault';
import { act, createElement } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { useOnboardingSync } from './useOnboardingSync';

vi.mock('../services/ws/connection', () => ({
  wsClient: { request: vi.fn(), send: vi.fn() },
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

function deferredResponse() {
  let resolve: (response: unknown) => void = () => undefined;
  let reject: (error: unknown) => void = () => undefined;
  const promise = new Promise<unknown>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return {
    resolve,
    reject,
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

it('shows clone cache immediately, waits for queue defaults, and keeps scans alive after Continue and unmount', async () => {
  const list = deferredResponse();
  const review = deferredResponse();
  const refreshed = deferredResponse();
  const scan = deferredResponse();
  vi.mocked(wsClient.request)
    .mockReturnValueOnce(list.result)
    .mockReturnValueOnce(review.result)
    .mockReturnValueOnce(refreshed.result)
    .mockReturnValueOnce(scan.result);
  const hook = await mountSync();
  expect(
    hook.latest.sync?.state.selection.repos.map(
      (repository) => repository.repo,
    ),
  ).toEqual(['clone']);
  expect(hook.latest.sync?.canContinue).toBe(false);
  expect(hook.latest.sync?.state.folder).toEqual({
    status: 'done',
    path: '/dev',
    partial: false,
  });
  expect(wsClient.request).not.toHaveBeenCalledWith(
    'SCAN_DEV_FOLDER',
    expect.anything(),
  );
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
  await act(async () => {
    hook.latest.sync?.scanAgain();
  });
  expect(hook.latest.sync?.state.folder.status).toBe('scanning');
  // One independent dispatch for the default-on choice; never wait on its reply.
  vi.mocked(wsClient.request).mockReturnValueOnce(deferredResponse().result);
  await act(async () => {
    hook.latest.sync?.continueFromSync();
  });
  expect(appStore.state.onboardingStep).toBe('practice');
  expect(appStore.state.onboardingSync?.selection.sealed).toBe(true);
  await hook.unmount();
  expect(wsClient.send).not.toHaveBeenCalled();
  const requests = vi.mocked(wsClient.request).mock.calls.length;
  await act(async () => {
    scan.resolve({
      ok: true,
      discovered: 1,
      healthy: 1,
      degraded: 0,
      skipped: 0,
      partial: true,
    });
  });
  expect(appStore.state.onboardingSync).toBeNull();
  expect(wsClient.request).toHaveBeenCalledTimes(requests);
});

it('reveals held nonlocal cache and permits an empty failed read once the queue fails', async () => {
  const list = deferredResponse();
  const review = deferredResponse();
  vi.mocked(wsClient.request)
    .mockReturnValueOnce(list.result)
    .mockReturnValueOnce(review.result);
  const hook = await mountSync();
  await act(async () => {
    list.reject('offline');
    review.reject('offline');
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
  const list = deferredResponse();
  const review = deferredResponse();
  const scan = deferredResponse();
  const refreshed = deferredResponse();
  vi.mocked(wsClient.request)
    .mockReturnValueOnce(list.result)
    .mockReturnValueOnce(review.result)
    .mockReturnValueOnce(scan.result)
    .mockReturnValueOnce(refreshed.result);
  const hook = await mountSync();
  await act(async () => {
    list.resolve({ repos: [] });
    review.reject('offline');
  });
  await act(async () => {
    hook.latest.sync?.scanAgain();
    hook.latest.sync?.stopScan();
  });
  expect(wsClient.send).toHaveBeenCalledExactlyOnceWith('SCAN_CANCEL', {});
  await act(async () => {
    scan.resolve({
      ok: true,
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
  expect(wsClient.send).toHaveBeenCalledTimes(1);
});
