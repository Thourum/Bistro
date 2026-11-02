import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';
import { useStore } from '../hooks/useStore';
import {
  cacheCommitList,
  cachedCommitList,
  clearCommitList,
  loadCommits,
  setCommitList,
  setCommitViewed,
} from '../services/commits';
import { appStore } from '../store';
import type { CommitsResult, CommitViewedAck } from '../types';
import { COMMIT_TOGGLE_VIEWED_EVENT } from '../utils/commitNavigation';

interface CommitsData {
  result: CommitsResult | null;
  loading: boolean;
  error: string | null;
  toggleViewed: (sha: string) => Promise<CommitViewedAck | null>;
  markViewed: (sha: string) => Promise<CommitViewedAck | null>;
  isPending: (sha: string) => boolean;
}

/** Also seeds the route cursor when a commit is opened directly. */
export function useCommitsData(
  prId: string,
  selectedSha?: string,
  options?: { revalidateAfter?: unknown },
): CommitsData {
  const login = useStore(appStore, (state) => state.viewerLogin);
  const revalidateAfter =
    options === undefined ? true : options.revalidateAfter;
  const [result, setResult] = useState<CommitsResult | null>(
    () => cachedCommitList(prId) ?? null,
  );
  const [loading, setLoading] = useState(result === null);
  const [error, setError] = useState<string | null>(null);
  const resultRef = useRef(result);
  resultRef.current = result;
  const scopeRef = useRef({ prId, login, active: true });
  const pendingRef = useRef(new Set<string>());

  useLayoutEffect(() => {
    const scope = { prId, login, active: true };
    scopeRef.current = scope;
    pendingRef.current = new Set();
    const cached = cachedCommitList(prId) ?? null;
    resultRef.current = cached;
    setResult(cached);
    setLoading(cached === null);
    setError(null);
    clearCommitList();
    return () => {
      scope.active = false;
    };
  }, [prId, login]);

  useLayoutEffect(() => {
    const scope = scopeRef.current;
    if (scope.prId !== prId || scope.login !== login) return;
    const current = resultRef.current;
    if (current === null || current !== result) return;
    setCommitList(
      (current.commits ?? []).map((commit) => commit.sha),
      selectedSha,
    );
  }, [prId, login, result, selectedSha]);

  useEffect(() => {
    if (revalidateAfter === null || revalidateAfter === false) return;
    const scope = scopeRef.current;
    if (scope.login !== login) return;
    let active = true;
    void loadCommits(prId).match(
      (payload) => {
        if (!active || !scope.active || scopeRef.current !== scope) return;
        const current = resultRef.current;
        const next = {
          ...payload,
          commits:
            payload.commits?.map((commit) => {
              if (!pendingRef.current.has(commit.sha)) return commit;
              const optimistic = current?.commits?.find(
                (entry) => entry.sha === commit.sha,
              );
              return optimistic === undefined
                ? commit
                : { ...commit, viewed: optimistic.viewed };
            }) ?? null,
        };
        if (next.available) cacheCommitList(scope.login, prId, next);
        resultRef.current = next;
        setResult(next);
        setLoading(false);
        setError(null);
      },
      (fault) => {
        if (!active || !scope.active || scopeRef.current !== scope) return;
        setError(fault.message);
        setLoading(false);
      },
    );
    return () => {
      active = false;
    };
  }, [prId, login, revalidateAfter]);

  const updateViewed = useCallback(
    async (sha: string, viewed: boolean): Promise<CommitViewedAck | null> => {
      const scope = scopeRef.current;
      const previous = resultRef.current;
      const commit = previous?.commits?.find((entry) => entry.sha === sha);
      if (
        !scope.active ||
        scope.prId !== prId ||
        scope.login !== login ||
        commit === undefined ||
        pendingRef.current.has(sha)
      )
        return null;
      pendingRef.current.add(sha);
      let scopedResult = previous;
      const applyViewed = (value: boolean): void => {
        const isCurrent = scope.active && scopeRef.current === scope;
        const current = isCurrent ? resultRef.current : scopedResult;
        if (current === null) return;
        const next = {
          ...current,
          commits: (current.commits ?? []).map((entry) =>
            entry.sha === sha ? { ...entry, viewed: value } : entry,
          ),
        };
        scopedResult = next;
        cacheCommitList(scope.login, prId, next);
        if (!isCurrent) return;
        resultRef.current = next;
        setResult(next);
      };
      applyViewed(viewed);
      const ack = await setCommitViewed(prId, sha, viewed);
      applyViewed(ack?.viewed ?? commit.viewed);
      if (!scope.active || scopeRef.current !== scope) return null;
      pendingRef.current.delete(sha);
      return ack;
    },
    [prId, login],
  );

  const toggleViewed = useCallback(
    (sha: string): Promise<CommitViewedAck | null> => {
      const commit = resultRef.current?.commits?.find(
        (entry) => entry.sha === sha,
      );
      return commit === undefined
        ? Promise.resolve(null)
        : updateViewed(sha, !commit.viewed);
    },
    [updateViewed],
  );
  const markViewed = useCallback(
    (sha: string): Promise<CommitViewedAck | null> => {
      const viewed = true;
      return updateViewed(sha, viewed);
    },
    [updateViewed],
  );
  const isPending = useCallback(
    (sha: string): boolean => pendingRef.current.has(sha),
    [],
  );

  useEffect(() => {
    const toggle = (): void => {
      const sha =
        selectedSha ??
        appStore.state.commitShas[appStore.state.selectedCommitIndex];
      if (sha !== undefined) void toggleViewed(sha);
    };
    document.addEventListener(COMMIT_TOGGLE_VIEWED_EVENT, toggle);
    return () =>
      document.removeEventListener(COMMIT_TOGGLE_VIEWED_EVENT, toggle);
  }, [selectedSha, toggleViewed]);

  return { result, loading, error, toggleViewed, markViewed, isPending };
}
