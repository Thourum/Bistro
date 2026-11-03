import { Commits } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { appStore } from '../store';
import type {
  CommitDiffResult,
  CommitsResult,
  CommitViewedAck,
} from '../types';
import { createLogger } from '../utils/logger';
import { call, callWrite, faultReason } from './engine/call';
import { addToast } from './toasts';

const log = createLogger('commits');
const MAX_KEPT_LISTS = 20;
const listCache = new Map<string, CommitsResult>();
const listListeners = new Map<string, Set<(result: CommitsResult) => void>>();
// Generations must remain unique after settled mutation records are removed.
const mutationSequence = { generation: 0 };
const listInFlight = new Map<string, ResultAsync<CommitsResult, Fault>>();
const diffInFlight = new Map<string, ResultAsync<CommitDiffResult, Fault>>();
interface ViewedMutation {
  generation: number;
  pending: boolean;
  viewed: boolean;
}
const viewedMutations = new Map<string, Map<string, ViewedMutation>>();

function shareRequest<T>(
  inFlight: Map<string, ResultAsync<T, Fault>>,
  key: string,
  start: () => ResultAsync<T, Fault>,
): ResultAsync<T, Fault> {
  const existing = inFlight.get(key);
  if (existing !== undefined) return existing;
  const request = start();
  inFlight.set(key, request);
  const release = (): void => {
    inFlight.delete(key);
    pruneSettledMutations(key);
  };
  void request.match(release, release);
  return request;
}

function listKey(login: string, prId: string): string {
  return `${login}\u0000${prId}`;
}

export function cachedCommitList(prId: string): CommitsResult | undefined {
  const key = listKey(appStore.state.viewerLogin, prId);
  const result = listCache.get(key);
  if (result !== undefined) {
    listCache.delete(key);
    listCache.set(key, result);
  }
  return result;
}

export function cacheCommitList(
  login: string,
  prId: string,
  result: CommitsResult,
): void {
  if (result.viewer_login !== login) return;
  publishCommitList(listKey(login, prId), result);
}

function publishCommitList(key: string, result: CommitsResult): void {
  listCache.delete(key);
  if (result.available) listCache.set(key, result);
  if (listCache.size > MAX_KEPT_LISTS) {
    const oldest = listCache.keys().next().value;
    if (oldest !== undefined) listCache.delete(oldest);
  }
  for (const listener of listListeners.get(key) ?? []) listener(result);
}

/** Observes kept-list changes, including writes completed after a remount. */
export function subscribeCommitList(
  login: string,
  prId: string,
  listener: (result: CommitsResult) => void,
): () => void {
  const key = listKey(login, prId);
  const listeners = listListeners.get(key) ?? new Set();
  listeners.add(listener);
  listListeners.set(key, listeners);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) listListeners.delete(key);
  };
}

function pruneSettledMutations(key: string): void {
  if (listInFlight.has(key)) return;
  const mutations = viewedMutations.get(key);
  if (mutations === undefined) return;
  for (const [sha, mutation] of mutations) {
    if (!mutation.pending) mutations.delete(sha);
  }
  if (mutations.size === 0) viewedMutations.delete(key);
}

export function beginCommitViewed(
  login: string,
  prId: string,
  change: { sha: string; viewed: boolean },
): number {
  const key = listKey(login, prId);
  const mutations =
    viewedMutations.get(key) ?? new Map<string, ViewedMutation>();
  const generation = ++mutationSequence.generation;
  mutations.set(change.sha, {
    generation,
    pending: true,
    viewed: change.viewed,
  });
  viewedMutations.set(key, mutations);
  updateCachedViewed(key, change.sha, change.viewed);
  return generation;
}

export function settleCommitViewed(
  login: string,
  prId: string,
  change: { sha: string; viewed: boolean; generation: number },
): boolean {
  const key = listKey(login, prId);
  const mutation = viewedMutations.get(key)?.get(change.sha);
  if (mutation?.generation !== change.generation) return false;
  viewedMutations.get(key)?.set(change.sha, {
    ...mutation,
    pending: false,
    viewed: change.viewed,
  });
  updateCachedViewed(key, change.sha, change.viewed);
  pruneSettledMutations(key);
  return true;
}

function updateCachedViewed(
  key: string,
  sha: string,
  viewed: boolean,
): CommitsResult | undefined {
  const current = listCache.get(key);
  if (current === undefined) return undefined;
  const next = {
    ...current,
    commits:
      current.commits?.map((commit) =>
        commit.sha === sha ? { ...commit, viewed } : commit,
      ) ?? null,
  };
  publishCommitList(key, next);
  return next;
}

/** Clears stale route navigation without losing the list selection. */
export function clearCommitList(): void {
  appStore.setState((state) => ({ ...state, commitShas: [] }));
}

/** Seeds a direct-visit cursor, or clamps the existing list selection. */
export function setCommitList(
  shas: readonly string[],
  selectedSha?: string,
): void {
  appStore.setState((state) => ({
    ...state,
    commitShas: [...shas],
    selectedCommitIndex:
      selectedSha === undefined
        ? Math.min(
            Math.max(state.selectedCommitIndex, 0),
            Math.max(0, shas.length - 1),
          )
        : Math.max(0, shas.indexOf(selectedSha)),
  }));
}

export function selectCommitIndex(index: number): void {
  appStore.setState((state) => ({ ...state, selectedCommitIndex: index }));
}

/** Commit diffs must not inherit file navigation or display overrides. */
export function resetCommitDiffState(): void {
  appStore.setState((state) => ({
    ...state,
    diffFileKeys: [],
    diffCollapseOverrides: [],
    hideWhitespaceHunks: false,
  }));
}

export function setCommitDiffFileKeys(keys: readonly string[]): void {
  appStore.setState((state) => ({ ...state, diffFileKeys: [...keys] }));
}

export function loadCommits(prId: string): ResultAsync<CommitsResult, Fault> {
  const login = appStore.state.viewerLogin;
  const key = listKey(login, prId);
  return shareRequest(listInFlight, key, () => {
    const startedMutations = new Map(
      [...(viewedMutations.get(key) ?? [])].map(
        ([sha, mutation]) => [sha, { ...mutation }] as const,
      ),
    );
    return call(() => Commits.List({ pr_id: prId })).map((result) => {
      if (result.viewer_login !== login) return result;
      const mutations = viewedMutations.get(key);
      const reconciled =
        mutations === undefined
          ? result
          : {
              ...result,
              commits:
                result.commits?.map((commit) => {
                  const mutation = mutations.get(commit.sha);
                  const started = startedMutations.get(commit.sha);
                  if (
                    mutation === undefined ||
                    (!mutation.pending &&
                      !started?.pending &&
                      mutation.generation === started?.generation)
                  )
                    return commit;
                  return { ...commit, viewed: mutation.viewed };
                }) ?? null,
            };
      cacheCommitList(login, prId, reconciled);
      return reconciled;
    });
  });
}

export function loadCommitDiff(
  prId: string,
  sha: string,
): ResultAsync<CommitDiffResult, Fault> {
  return shareRequest(diffInFlight, `${prId}:${sha}`, () =>
    call(() => Commits.Get({ pr_id: prId, sha })),
  );
}

/** Returns the ack, or null after toasting the reason (UI rolls back). */
export function setCommitViewed(
  prId: string,
  sha: string,
  viewed: boolean,
): Promise<CommitViewedAck | null> {
  return callWrite(() => Commits.SetViewed({ pr_id: prId, sha, viewed })).match(
    (ack) => ack,
    (fault) => {
      log.warn({ err: fault.message, sha }, 'failed to set commit viewed');
      const reason = faultReason(fault);
      addToast({
        type: 'warning',
        message: reason
          ? `Could not update viewed: ${reason}`
          : 'Could not update viewed: network or communication error',
        dismissible: true,
      });
      return null;
    },
  );
}
