// DiffPage — route-owned diff screen with stable file identity via canonical path keys
//
// PR resolved by $prId from /prs/$prId/diff route param (NEVER selectedPRIndex).
// File resolved by ?file canonical key via findFileByKey (NEVER selectedFileIndex).
// Unknown ?file → first-file fallback.
// Back button navigates to /prs/$prId via router.
// diffFileKeys are cleared when prId changes to prevent stale key carryover.

import { useNavigate, useParams, useSearch } from '@tanstack/react-router';
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { PierreDiffView } from '../components/PierreDiffView';
import { RangeDiffView } from '../components/RangeDiffView';
import { StatusSquare } from '../components/StatusSquare';
import { addToast } from '../components/ToastContainer';
import { useActionDispatch } from '../hooks/useActionDispatch';
import { useExpertise } from '../hooks/useExpertise';
import { useMarkFileViewed } from '../hooks/useMarkFileViewed';
import { usePRHydration } from '../hooks/usePRHydration';
import { type RangeDiffRequest, useRangeDiff } from '../hooks/useRangeDiff';
import { useReviewThreads } from '../hooks/useReviewThreads';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import type { Comment, DiffFile } from '../types';
import { repoIdForPR } from '../utils/expertise';
import { createLogger } from '../utils/logger';

const log = createLogger('DiffPage');

/**
 * Full-screen diff view — matches `diff_view_minimal_light/code.html`.
 *
 * Two diff surfaces share this page:
 *   - the full PR diff (default), rendered by {@link PierreDiffView}
 *   - a range diff (since-last-review or a timeline patchset pair), rendered
 *     by {@link RangeDiffView}
 *
 * PR resolved by $prId route param; file resolved by ?file canonical key.
 *
 * Keyboard-first affordances (registered in `keybindings.ts`, driven here):
 *   - `s` toggles the since-last-review range diff
 *   - `v` marks the current file viewed
 */
export function DiffPage() {
  // Resolve PR by stable id from route params — NEVER by selectedPRIndex.
  const { prId } = useParams({ strict: false }) as { prId: string };
  const { file: fileKey } = useSearch({ strict: false }) as { file: string };
  const navigate = useNavigate();

  // Phase 2 store reads: range-diff intent and file-viewed tracking.
  const selectedFileIndex = useStore(
    appStore,
    (s: UIState) => s.selectedFileIndex,
  );
  const confirmingAction = useStore(
    appStore,
    (s: UIState) => s.confirmingAction,
  );
  const selectedPatchsetPair = useStore(
    appStore,
    (s: UIState) => s.selectedPatchsetPair,
  );
  const commentsRevision = useStore(
    appStore,
    (s: UIState) => s.commentsRevision,
  );

  const theme = useStore(settingsStore, (s) => s.appearance.theme);

  // Fetch ALL PRs so deep-linked PRs (any status) are always resolvable.
  const { data: allPRs = [], isLoading: prLoading } = useWebSocketPRList('all');
  const pr = allPRs.find((p) => p.id === prId);
  const { isHydrating, error: hydrationError, retry } = usePRHydration(pr);
  const isAwaitingHydration = pr?.hydrated === 0;

  const { request } = useWebSocket();
  const { dispatchReplyComment } = useActionDispatch(pr?.id);
  const { markViewed } = useMarkFileViewed();
  const { expertise } = useExpertise(pr?.id);
  const expertiseStatus = useStore(appStore, (s: UIState) =>
    pr !== undefined ? s.expertiseRepoStatuses?.[repoIdForPR(pr)] : undefined,
  );

  const [diff, setDiff] = useState<string>('');
  const [files, setFiles] = useState<DiffFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [findQuery, setFindQuery] = useState('');
  const [comments, setComments] = useState<Comment[]>([]);

  // Review threads for inline annotations (full diff path only).
  const { threads } = useReviewThreads(pr?.id);

  // ── Range-diff intent ──────────────────────────────────────────────
  const sinceLastReview = confirmingAction === 'since-last-review';

  const rangeReq = useMemo<RangeDiffRequest | null>(() => {
    if (sinceLastReview) return { kind: 'sinceLastReview' };
    if (selectedPatchsetPair !== null) {
      const [from, to] = selectedPatchsetPair;
      if (from !== to) {
        return { kind: 'pair', fromPatchset: from, toPatchset: to };
      }
    }
    return null;
  }, [sinceLastReview, selectedPatchsetPair]);

  const {
    result: rangeResult,
    loading: rangeLoading,
    error: rangeError,
  } = useRangeDiff(pr?.id, rangeReq);

  const showRangeDiff = rangeReq !== null;

  // ── Full diff (default surface) ────────────────────────────────────
  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    setLoading(true);
    setDiffError(null);
    request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .then((data) => {
        const payload = data as { diff?: string; files?: DiffFile[] };
        setDiff(payload.diff ?? '');
        setFiles(payload.files ?? []);
        setLoading(false);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        log.error({ err: msg }, 'failed to fetch diff');
        setDiffError(msg);
        setLoading(false);
      });
  }, [isAwaitingHydration, pr, request]);

  // Fetch comments for inline thread annotations (full diff surface only).
  useEffect(() => {
    if (pr === undefined) return;
    log.debug({ commentsRevision }, 'fetching comments for diff annotations');
    request('REQUEST_COMMENTS', { pr_id: pr.id })
      .then((data) => {
        const payload = data as { comments?: Comment[] };
        setComments(payload.comments ?? []);
      })
      .catch((err: unknown) => {
        log.warn({ err }, 'failed to fetch comments for diff annotations');
      });
  }, [pr, request, commentsRevision]);

  // Clear stale diffFileKeys whenever the target PR changes.
  // This prevents keybindings from using keys from a previous PR's diff.
  useLayoutEffect(() => {
    if (prId === '') return;
    appStore.setState((s) => ({ ...s, diffFileKeys: [] }));
  }, [prId]);

  // Consume onFilesResolved via useLayoutEffect to ensure diffFileKeys
  // commits synchronously before the next user input (mirrors visiblePRIds pattern).
  const handleFilesResolved = (canonicalKeys: string[]): void => {
    appStore.setState((s) => ({ ...s, diffFileKeys: canonicalKeys }));
  };

  // ── Mark-file-viewed (triggered by `v`) ────────────────────────────
  const currentFile = files[selectedFileIndex];
  const markInFlight = useRef(false);

  const doMarkViewed = useCallback(() => {
    if (pr === undefined || currentFile === undefined) return;
    if (markInFlight.current) return;
    markInFlight.current = true;
    const { path, content_hash } = currentFile;
    void markViewed(pr.id, path, content_hash)
      .then((ack) => {
        if (ack?.ok) {
          setFiles((prev) =>
            prev.map((f) => (f.path === path ? { ...f, viewed: true } : f)),
          );
        } else if (ack?.reason) {
          addToast({
            type: 'warning',
            message: `Could not mark viewed: ${ack.reason}`,
            dismissible: true,
          });
        } else {
          addToast({
            type: 'warning',
            message: 'Could not mark viewed: network or communication error',
            dismissible: true,
          });
        }
      })
      .finally(() => {
        markInFlight.current = false;
      });
  }, [pr, currentFile, markViewed]);

  useEffect(() => {
    if (confirmingAction !== 'mark-viewed') return;
    doMarkViewed();
    appStore.setState((s) => ({ ...s, confirmingAction: null }));
  }, [confirmingAction, doMarkViewed]);

  // Clear range-diff intent when leaving the diff view.
  useEffect(() => {
    return () => {
      appStore.setState((s) =>
        s.confirmingAction === 'since-last-review'
          ? { ...s, confirmingAction: null }
          : s,
      );
    };
  }, []);

  const handleBack = (): void => {
    void navigate({ to: '/prs/$prId', params: { prId } });
  };

  if (!prLoading && pr === undefined) {
    return (
      <div className="flex-1 flex items-center justify-center text-error">
        PR not found
      </div>
    );
  }

  const viewedCount = files.filter((f) => f.viewed).length;

  return (
    <main className="flex-1 flex flex-col overflow-hidden bg-surface text-on-surface">
      {/* Top bar */}
      <div className="shrink-0 h-10 flex items-center justify-between px-4 border-b border-outline-variant/30 bg-surface-container-lowest">
        <div className="flex items-center gap-4 min-w-0">
          <button
            type="button"
            onClick={handleBack}
            className="font-mono text-[10px] uppercase tracking-widest text-outline hover:text-on-surface transition-colors"
          >
            ← Back
          </button>
          {pr !== undefined && (
            <>
              <span className="font-mono text-[11px] font-bold tracking-widest text-primary">
                PR-{pr.number}
              </span>
              <span className="font-mono text-[10px] text-outline truncate">
                {pr.head_branch}
                <span className="text-outline-variant"> → </span>
                {pr.base_branch}
              </span>
              {showRangeDiff && (
                <span className="font-mono text-[10px] uppercase tracking-widest text-primary border border-primary/40 px-2 py-0.5">
                  {sinceLastReview
                    ? 'Since last review'
                    : `P${selectedPatchsetPair?.[0]} → P${selectedPatchsetPair?.[1]}`}
                </span>
              )}
            </>
          )}
        </div>
        <div className="flex items-center gap-3">
          {!showRangeDiff && (
            <input
              type="text"
              value={findQuery}
              onChange={(e) => setFindQuery(e.target.value)}
              placeholder="Find in diff…"
              className="font-mono text-[11px] px-3 py-1 w-64 bg-surface border border-outline-variant/40 placeholder:text-outline-variant"
            />
          )}
          <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
            ESC to exit
          </span>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 min-h-0">
        {isAwaitingHydration ? (
          <HydrationState
            error={hydrationError}
            isHydrating={isHydrating}
            onRetry={retry}
          />
        ) : showRangeDiff ? (
          rangeLoading ? (
            <LoadingState label="Loading range diff…" />
          ) : rangeError !== null ? (
            <ErrorState message={`Failed to load range diff: ${rangeError}`} />
          ) : rangeResult !== null ? (
            <RangeDiffView result={rangeResult} />
          ) : (
            <LoadingState label="Loading range diff…" />
          )
        ) : loading ? (
          <LoadingState label="Loading diff…" />
        ) : diffError !== null ? (
          <ErrorState message={`Failed to load diff: ${diffError}`} />
        ) : (
          <PierreDiffView
            diff={diff}
            theme={theme}
            findQuery={findQuery}
            selectedFileKey={fileKey}
            onFilesResolved={handleFilesResolved}
            threads={threads}
            comments={comments}
            onThreadReply={(threadId, body) => {
              dispatchReplyComment(body, { threadId });
            }}
            expertiseFiles={expertise?.files ?? []}
            expertiseError={
              expertiseStatus?.status === 'error'
                ? (expertiseStatus.message ?? expertiseStatus.code ?? 'unknown')
                : null
            }
          />
        )}
      </div>

      {/* Status bar (LOC / encoding / viewed) */}
      <div className="shrink-0 h-6 px-4 flex items-center justify-between border-t border-outline-variant/30 bg-surface-container-low">
        <div className="flex items-center gap-4 font-mono text-[10px] uppercase tracking-widest text-outline">
          <span>
            {showRangeDiff && rangeResult !== null
              ? `${rangeResult.raw.split('\n').length} lines`
              : `${diff.split('\n').length} lines`}
          </span>
          {currentFile !== undefined && (
            <span className="flex items-center gap-1.5">
              <StatusSquare
                status={currentFile.viewed ? 'success' : 'neutral'}
                size="w-2 h-2"
                label={currentFile.viewed ? 'VIEWED' : 'NOT VIEWED'}
              />
              <span
                className={currentFile.viewed ? 'text-primary' : 'text-outline'}
              >
                {currentFile.viewed ? 'viewed' : 'press v to mark viewed'}
              </span>
            </span>
          )}
        </div>
        <div className="flex items-center gap-6 font-mono text-[10px] uppercase tracking-widest text-outline">
          {files.length > 0 && (
            <span>
              {viewedCount}/{files.length} viewed
            </span>
          )}
          <span>UTF-8</span>
          <span>LF</span>
          <span>DIFF</span>
        </div>
      </div>
    </main>
  );
}

function HydrationState({
  error,
  isHydrating,
  onRetry,
}: {
  error: string | null;
  isHydrating: boolean;
  onRetry: () => void;
}): React.ReactElement {
  if (isHydrating || error === null) {
    return <LoadingState label="Preparing pull request details…" />;
  }

  return (
    <div className="w-full h-full flex flex-col items-center justify-center gap-3 text-error text-sm">
      <span>Failed to prepare pull request details: {error}</span>
      <button
        type="button"
        onClick={onRetry}
        className="font-mono text-[10px] uppercase tracking-widest border border-error/60 px-3 py-1 text-error hover:bg-error/10"
      >
        Retry hydration
      </button>
    </div>
  );
}

function LoadingState({ label }: { label: string }): React.ReactElement {
  return (
    <div className="w-full h-full flex items-center justify-center text-on-surface-variant font-mono uppercase tracking-widest text-xs">
      {label}
    </div>
  );
}

function ErrorState({ message }: { message: string }): React.ReactElement {
  return (
    <div className="w-full h-full flex items-center justify-center text-error text-sm">
      {message}
    </div>
  );
}
