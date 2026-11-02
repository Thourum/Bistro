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
const listCache = new Map<string, CommitsResult>();
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
  void request.match(
    () => inFlight.delete(key),
    () => inFlight.delete(key),
  );
  return request;
}

function listKey(login: string, prId: string): string {
  return `${login}\u0000${prId}`;
}

export function cachedCommitList(prId: string): CommitsResult | undefined {
  return listCache.get(listKey(appStore.state.viewerLogin, prId));
}

export function cacheCommitList(
  login: string,
  prId: string,
  result: CommitsResult,
): void {
  const key = listKey(login, prId);
  if (result.available) listCache.set(key, result);
  else listCache.delete(key);
}

export function beginCommitViewed(
  login: string,
  prId: string,
  change: { sha: string; viewed: boolean },
): number {
  const key = listKey(login, prId);
  const mutations =
    viewedMutations.get(key) ?? new Map<string, ViewedMutation>();
  const generation = (mutations.get(change.sha)?.generation ?? 0) + 1;
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
  mutation.pending = false;
  mutation.viewed = change.viewed;
  updateCachedViewed(key, change.sha, change.viewed);
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
  listCache.set(key, next);
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
