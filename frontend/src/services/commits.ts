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
  listCache.set(listKey(login, prId), result);
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
  const key = listKey(appStore.state.viewerLogin, prId);
  return shareRequest(listInFlight, key, () =>
    call(() => Commits.List({ pr_id: prId })).map((result) => {
      if (result.available) listCache.set(key, result);
      else listCache.delete(key);
      return result;
    }),
  );
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
