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
  viewer_login: initialState.viewerLogin,
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
  const firstAccount = { ...COMMIT_LIST, viewer_login: 'a' };
  const otherAccount = { ...COMMIT_LIST, viewer_login: 'b', truncated: true };
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve(otherAccount)),
  );
  expect(await services.loadCommits('pr').unwrapOr(null)).toBe(otherAccount);
  pending.resolve(firstAccount);
  await first;
  expect(services.cachedCommitList('pr')).toBe(otherAccount);
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  expect(services.cachedCommitList('pr')).toBe(firstAccount);
  expect(Commits.List).toHaveBeenCalledTimes(2);
});

it('does not populate the current account cache from a previous account completion', async () => {
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  const pending = deferred<CommitsResult>();
  vi.mocked(Commits.List).mockReturnValueOnce(cancellable(pending.promise));
  const request = services.loadCommits('pr');
  appStore.setState((state) => ({ ...state, viewerLogin: 'b' }));
  const firstAccount = { ...COMMIT_LIST, viewer_login: 'a' };
  pending.resolve(firstAccount);
  await request;
  expect(services.cachedCommitList('pr')).toBeUndefined();
  services.cacheCommitList('a', 'pr', firstAccount);
  expect(services.cachedCommitList('pr')).toBeUndefined();
  appStore.setState((state) => ({ ...state, viewerLogin: 'a' }));
  expect(services.cachedCommitList('pr')).toBe(firstAccount);
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

it('never recaches an unavailable list when a pending viewed write settles', async () => {
  const login = appStore.state.viewerLogin;
  services.cacheCommitList(login, 'pr', COMMIT_LIST);
  const generation = services.beginCommitViewed(login, 'pr', {
    sha: 'sha',
    viewed: true,
  });
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve({ ...COMMIT_LIST, available: false })),
  );
  await services.loadCommits('pr');
  services.settleCommitViewed(login, 'pr', {
    sha: 'sha',
    viewed: true,
    generation,
  });
  expect(services.cachedCommitList('pr')).toBeUndefined();
  services.cacheCommitList(login, 'pr', { ...COMMIT_LIST, available: false });
  expect(services.cachedCommitList('pr')).toBeUndefined();
});

it('settles a late viewed write on the latest cache without replacing other changes', () => {
  const login = appStore.state.viewerLogin;
  const first = {
    sha: 'first',
    parent_shas: [],
    subject: '',
    body: '',
    author_name: '',
    authored_at: '',
    is_merge: false,
    additions: 0,
    deletions: 0,
    file_count: 0,
    viewed: false,
  };
  services.cacheCommitList(login, 'pr', { ...COMMIT_LIST, commits: [first] });
  const generation = services.beginCommitViewed(login, 'pr', {
    sha: 'first',
    viewed: true,
  });
  const added = { ...first, sha: 'added', viewed: true };
  services.cacheCommitList(login, 'pr', {
    ...COMMIT_LIST,
    commits: [{ ...first, subject: 'new subject' }, added],
  });
  services.settleCommitViewed(login, 'pr', {
    sha: 'first',
    viewed: true,
    generation,
  });
  expect(services.cachedCommitList('pr')?.commits).toEqual([
    { ...first, subject: 'new subject', viewed: true },
    added,
  ]);
});

it('drops a list whose viewed marks belong to a different account', async () => {
  appStore.setState((state) => ({ ...state, viewerLogin: 'starting-account' }));
  const wrongAccount = { ...COMMIT_LIST, viewer_login: 'other-account' };
  vi.mocked(Commits.List).mockReturnValueOnce(
    cancellable(Promise.resolve(wrongAccount)),
  );
  await services.loadCommits('pr');
  expect(services.cachedCommitList('pr')).toBeUndefined();
});

it('evicts the least recently used kept list on the twenty-first PR', () => {
  const cacheLimit = 20;
  for (let index = 0; index < cacheLimit; index++) {
    services.cacheCommitList(
      initialState.viewerLogin,
      `pr-${index}`,
      COMMIT_LIST,
    );
  }
  expect(services.cachedCommitList('pr-0')).toBe(COMMIT_LIST);
  services.cacheCommitList(initialState.viewerLogin, 'pr-20', COMMIT_LIST);
  expect(services.cachedCommitList('pr-1')).toBeUndefined();
  expect(services.cachedCommitList('pr-0')).toBe(COMMIT_LIST);
  expect(services.cachedCommitList('pr-20')).toBe(COMMIT_LIST);
});
