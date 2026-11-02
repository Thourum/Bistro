import { Commits } from '@bindings/services';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { cancellable } from '../test/cancellable';
import { deferred } from '../test/deferred';
import type { CommitDiffResult, CommitsResult } from '../types';

vi.mock('@bindings/services', () => ({
  Commits: { List: vi.fn(), Get: vi.fn(), SetViewed: vi.fn() },
}));
vi.mock('../store', async (importOriginal) => importOriginal());

const initialState = appStore.state;
let services: typeof import('./commits');
beforeEach(async () => {
  vi.resetModules();
  services = await import('./commits');
});
afterEach(() => {
  appStore.setState(() => initialState);
  vi.resetAllMocks();
});

const COMMIT_LIST: CommitsResult = {
  pr_id: 'pr',
  available: true,
  reason: '',
  truncated: false,
  commits: [],
};
const COMMIT_DIFF: CommitDiffResult = {
  pr_id: 'pr',
  sha: 'sha',
  available: true,
  reason: '',
  is_merge: false,
  diff: '',
  files: [],
  additions: 0,
  deletions: 0,
};

it('shares concurrent lists, keeps successful lists, and revalidates later calls', async () => {
  const pending = deferred<CommitsResult>();
  vi.mocked(Commits.List).mockReturnValueOnce(cancellable(pending.promise));
  const first = services.loadCommits('pr');
  const second = services.loadCommits('pr');
  expect(Commits.List).toHaveBeenCalledOnce();
  pending.resolve(COMMIT_LIST);
  expect(await first.unwrapOr(null)).toBe(COMMIT_LIST);
  expect(await second.unwrapOr(null)).toBe(COMMIT_LIST);
  expect(services.cachedCommitList('pr')).toBe(COMMIT_LIST);

  const updated = { ...COMMIT_LIST, truncated: true };
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve(updated)),
  );
  expect(await services.loadCommits('pr').unwrapOr(null)).toBe(updated);
  expect(Commits.List).toHaveBeenCalledTimes(2);
  expect(services.cachedCommitList('pr')).toBe(updated);
});

it('isolates list requests and late completions by the starting account', async () => {
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  const pending = deferred<CommitsResult>();
  vi.mocked(Commits.List).mockReturnValueOnce(cancellable(pending.promise));
  const first = services.loadCommits('pr');
  appStore.setState((state) => ({ ...state, viewerLogin: 'b' }));
  const otherAccount = { ...COMMIT_LIST, truncated: true };
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve(otherAccount)),
  );
  expect(await services.loadCommits('pr').unwrapOr(null)).toBe(otherAccount);
  pending.resolve(COMMIT_LIST);
  await first;
  expect(services.cachedCommitList('pr')).toBe(otherAccount);
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  expect(services.cachedCommitList('pr')).toBe(COMMIT_LIST);
  expect(Commits.List).toHaveBeenCalledTimes(2);
});

it('does not populate the current account cache from a previous account completion', async () => {
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  const pending = deferred<CommitsResult>();
  vi.mocked(Commits.List).mockReturnValueOnce(cancellable(pending.promise));
  const request = services.loadCommits('pr');
  appStore.setState((state) => ({ ...state, viewerLogin: 'b' }));
  pending.resolve(COMMIT_LIST);
  await request;
  expect(services.cachedCommitList('pr')).toBeUndefined();
  services.cacheCommitList('a', 'pr', COMMIT_LIST);
  expect(services.cachedCommitList('pr')).toBeUndefined();
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  expect(services.cachedCommitList('pr')).toBe(COMMIT_LIST);
});

it('evicts unavailable lists and retries failed requests', async () => {
  services.cacheCommitList(appStore.state.viewerLogin, 'pr', COMMIT_LIST);
  const unavailable = { ...COMMIT_LIST, available: false };
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve(unavailable)),
  );
  await services.loadCommits('pr');
  expect(services.cachedCommitList('pr')).toBeUndefined();

  const failed = deferred<CommitsResult>();
  vi.mocked(Commits.List).mockReturnValueOnce(cancellable(failed.promise));
  const first = services.loadCommits('pr');
  const second = services.loadCommits('pr');
  failed.reject(new Error('offline'));
  expect((await first).isErr()).toBe(true);
  expect((await second).isErr()).toBe(true);
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve(COMMIT_LIST)),
  );
  expect(await services.loadCommits('pr').unwrapOr(null)).toBe(COMMIT_LIST);
  expect(Commits.List).toHaveBeenCalledTimes(3);
});

it('shares concurrent commit diffs without retaining settled results', async () => {
  const pending = deferred<CommitDiffResult>();
  vi.mocked(Commits.Get).mockReturnValueOnce(cancellable(pending.promise));
  const first = services.loadCommitDiff('pr', 'sha');
  const second = services.loadCommitDiff('pr', 'sha');
  expect(Commits.Get).toHaveBeenCalledOnce();
  pending.resolve(COMMIT_DIFF);
  expect(await first.unwrapOr(null)).toBe(COMMIT_DIFF);
  expect(await second.unwrapOr(null)).toBe(COMMIT_DIFF);
  vi.mocked(Commits.Get).mockReturnValueOnce(
    cancellable(Promise.resolve(COMMIT_DIFF)),
  );
  await services.loadCommitDiff('pr', 'sha');
  expect(Commits.Get).toHaveBeenCalledTimes(2);
});

it('releases failed diff requests and keeps different SHAs independent', async () => {
  const pending = deferred<CommitDiffResult>();
  vi.mocked(Commits.Get).mockReturnValueOnce(cancellable(pending.promise));
  const failed = services.loadCommitDiff('pr', 'sha');
  vi.mocked(Commits.Get).mockReturnValueOnce(
    cancellable(Promise.resolve(COMMIT_DIFF)),
  );
  await services.loadCommitDiff('pr', 'other');
  pending.reject(new Error('offline'));
  expect((await failed).isErr()).toBe(true);
  vi.mocked(Commits.Get).mockReturnValueOnce(
    cancellable(Promise.resolve(COMMIT_DIFF)),
  );
  await services.loadCommitDiff('pr', 'sha');
  expect(Commits.Get).toHaveBeenCalledTimes(3);
});
