import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';
import {
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
): CommitsData {
  const [result, setResult] = useState<CommitsResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const resultRef = useRef(result);
  resultRef.current = result;
  const scopeRef = useRef({ prId, selectedSha, active: true });
  const pendingRef = useRef(new Set<string>());

  useLayoutEffect(() => {
    void prId;
    void selectedSha;
    clearCommitList();
  }, [prId, selectedSha]);

  useEffect(() => {
    const scope = { prId, selectedSha, active: true };
    scopeRef.current = scope;
    pendingRef.current = new Set();
    resultRef.current = null;
    setResult(null);
    setLoading(true);
    setError(null);
    void loadCommits(prId).match(
      (payload) => {
        if (!scope.active) return;
        resultRef.current = payload;
        setResult(payload);
        setLoading(false);
        const shas = (payload.commits ?? []).map((commit) => commit.sha);
        setCommitList(shas, selectedSha);
      },
      (fault) => {
        if (!scope.active) return;
        setError(fault.message);
        setLoading(false);
      },
    );
    return () => {
      scope.active = false;
    };
  }, [prId, selectedSha]);

  const updateViewed = useCallback(
    async (sha: string, viewed: boolean): Promise<CommitViewedAck | null> => {
      const scope = scopeRef.current;
      const previous = resultRef.current;
      const commit = previous?.commits?.find((entry) => entry.sha === sha);
      if (
        !scope.active ||
        scope.prId !== prId ||
        commit === undefined ||
        pendingRef.current.has(sha)
      )
        return null;
      pendingRef.current.add(sha);
      const applyViewed = (value: boolean): void => {
        const current = resultRef.current;
        if (current === null) return;
        const next = {
          ...current,
          commits: (current.commits ?? []).map((entry) =>
            entry.sha === sha ? { ...entry, viewed: value } : entry,
          ),
        };
        resultRef.current = next;
        setResult(next);
      };
      applyViewed(viewed);
      const ack = await setCommitViewed(prId, sha, viewed);
      if (!scope.active || scopeRef.current !== scope) return null;
      pendingRef.current.delete(sha);
      applyViewed(ack?.viewed ?? commit.viewed);
      return ack;
    },
    [prId],
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
