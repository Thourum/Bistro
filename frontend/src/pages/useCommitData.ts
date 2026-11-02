import { useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { loadCommitDiff } from '../services/commits';
import { faultReason } from '../services/engine/call';
import type {
  CommitDiffResult,
  CommitSummary,
  CommitViewedAck,
} from '../types';
import {
  COMMIT_VIEWED_AND_NEXT_EVENT,
  type CommitPosition,
  commitPosition,
} from '../utils/commitNavigation';
import { useCommitsData } from './useCommitsData';

interface CommitData {
  commit: CommitSummary | null;
  position: CommitPosition | null;
  diff: CommitDiffResult | null;
  oversizedPatchBytes: ReadonlyMap<string, number>;
  loading: boolean;
  error: string | null;
  notInPr: boolean;
  unavailableReason: string | null;
  toggleViewed: () => Promise<CommitViewedAck | null>;
}

/** List metadata and the commit patch are requested independently. */
export function useCommitData(prId: string, sha: string): CommitData {
  const navigate = useNavigate();
  const [response, setDiffState] = useState({
    prId,
    sha,
    diff: null as CommitDiffResult | null,
    loading: true,
    error: null as string | null,
    notInPr: false,
  });
  const diffState =
    response.prId === prId && response.sha === sha
      ? response
      : { diff: null, loading: true, error: null, notInPr: false };
  useEffect(() => {
    let isActive = true;
    setDiffState({
      prId,
      sha,
      diff: null,
      loading: true,
      error: null,
      notInPr: false,
    });
    void loadCommitDiff(prId, sha).match(
      (diff) => {
        if (!isActive) return;
        setDiffState({
          prId,
          sha,
          diff,
          loading: false,
          error: null,
          notInPr: false,
        });
      },
      (fault) => {
        if (!isActive) return;
        const notInPr = faultReason(fault) === 'commit_not_in_pr';
        setDiffState({
          prId,
          sha,
          diff: null,
          loading: false,
          error: notInPr ? null : fault.message,
          notInPr,
        });
      },
    );
    return () => {
      isActive = false;
    };
  }, [prId, sha]);

  const commits = useCommitsData(prId, sha, {
    revalidateAfter: diffState.diff ?? diffState.error ?? diffState.notInPr,
  });
  const oversizedPatchBytes = useMemo(
    () =>
      new Map(
        (diffState.diff?.files ?? [])
          .filter((file) => !file.is_binary && file.diff_status !== 'ok')
          .map((file) => [file.path, file.patch_bytes]),
      ),
    [diffState.diff],
  );
  const listed = commits.result?.commits ?? [];
  const commit = listed.find((entry) => entry.sha === sha) ?? null;
  const position = commitPosition(
    listed.map((entry) => entry.sha),
    sha,
  );
  const nextSha = position?.nextSha;
  const commitRef = useRef(commit);
  commitRef.current = commit;
  const { markViewed, isPending } = commits;
  useEffect(() => {
    let isActive = true;
    const viewedAndNext = async (): Promise<void> => {
      const current = commitRef.current;
      if (current === null || isPending(sha)) return;
      if (!current.viewed && (await markViewed(sha)) === null) return;
      if (!isActive || nextSha === undefined || nextSha === null) return;
      void navigate({
        to: '/prs/$prId/commits/$sha',
        params: { prId, sha: nextSha },
        search: { file: '', side: 'right' },
      });
    };
    const handle = (): void => {
      void viewedAndNext();
    };
    document.addEventListener(COMMIT_VIEWED_AND_NEXT_EVENT, handle);
    return () => {
      isActive = false;
      document.removeEventListener(COMMIT_VIEWED_AND_NEXT_EVENT, handle);
    };
  }, [nextSha, prId, sha, markViewed, isPending, navigate]);

  const toggleViewed = useCallback(
    () => commits.toggleViewed(sha),
    [commits.toggleViewed, sha],
  );
  return {
    commit,
    position,
    ...diffState,
    oversizedPatchBytes,
    loading: diffState.loading,
    error: diffState.diff !== null ? null : (diffState.error ?? commits.error),
    unavailableReason:
      commits.result?.available === false
        ? commits.result.reason
        : diffState.diff?.available === false
          ? diffState.diff.reason
          : null,
    toggleViewed,
  };
}
