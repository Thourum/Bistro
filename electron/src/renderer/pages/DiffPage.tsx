// DiffPage — route-owned diff screen with stable file identity via canonical path keys
//
// PR resolved by $prId from /prs/$prId/diff route param (NEVER selectedPRIndex).
// File resolved by ?file canonical key via findFileByKey (NEVER selectedFileIndex).
// Unknown ?file → first-file fallback.
// Back button navigates to /prs/$prId via router.
// diffFileKeys are cleared when prId changes to prevent stale key carryover.

import { useNavigate, useParams, useSearch } from '@tanstack/react-router';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { DiffView } from '../components/diff/DiffView';
import { splitUnifiedDiff } from '../components/diff/diffPathUtils';
import { OpenInEditorDialog } from '../components/OpenInEditorDialog';
import { useExpertise } from '../hooks/useExpertise';
import { type InterdiffRequest, useInterdiff } from '../hooks/useInterdiff';
import { usePRHydration } from '../hooks/usePRHydration';
import { useReviewDrafts } from '../hooks/useReviewDrafts';
import { useReviewThreads } from '../hooks/useReviewThreads';
import { useStore } from '../hooks/useStore';
import { type TriageRequest, useTriage } from '../hooks/useTriage';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { replyComment } from '../services/prActions';
import { isDraftOutdated } from '../services/reviewDrafts';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import { repoIdForPR } from '../utils/expertise';
import { createLogger } from '../utils/logger';
import { filterDiffToPaths, triageSummary } from '../utils/sinceLastReview';
import { prBrowserUrl } from '../utils/triage';
import {
  DiffPageBody,
  DiffPageStatusBar,
  DiffPageToolbar,
} from './DiffPageChrome';
import { InterdiffPane } from './DiffPageInterdiff';
import { useDiffPageActions } from './useDiffPageActions';
import { useDiffPageData } from './useDiffPageData';

const log = createLogger('DiffPage');

/**
 * Full-screen diff view — matches `diff_view_minimal_light/code.html`.
 *
 * One diff surface: the normal current PR diff, optionally filtered to files
 * changed since your last review or between a timeline patchset pair.
 *
 * PR resolved by $prId route param; file resolved by ?file canonical key.
 *
 * Keyboard-first affordances (registered in `keybindings.ts`, driven here):
 *   - `s` filters to files changed since your last review
 *   - `i` toggles the current file's interdiff
 *   - `v` marks the current file viewed
 */
export function DiffPage() {
  // Resolve PR by stable id from route params — NEVER by selectedPRIndex.
  const { prId } = useParams({ from: '/authed/prs/$prId/diff' });
  const { file: fileKey } = useSearch({ from: '/authed/prs/$prId/diff' });
  const navigate = useNavigate();

  // Store reads: triage intent and file-viewed tracking.
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
  const diffStyle = useStore(settingsStore, (s) => s.diff.style);

  // Fetch ALL PRs so deep-linked PRs (any status) are always resolvable.
  const { data: allPRs = [], isLoading: prLoading } = useWebSocketPRList('all');
  const pr = allPRs.find((p) => p.id === prId);
  const { isHydrating, error: hydrationError, retry } = usePRHydration(pr);
  const isAwaitingHydration = pr?.hydrated === 0;

  const { expertise } = useExpertise(pr?.id);
  const expertiseStatus = useStore(appStore, (s: UIState) =>
    pr !== undefined ? s.expertiseRepoStatuses?.[repoIdForPR(pr)] : undefined,
  );

  const [findQuery, setFindQuery] = useState('');
  const {
    diff,
    files,
    oversizedPatchBytes,
    setFiles,
    loading,
    diffError,
    comments,
  } = useDiffPageData({ pr, isAwaitingHydration, commentsRevision });

  // Review threads for inline annotations on the visible diff.
  const { threads } = useReviewThreads(pr?.id);
  useReviewDrafts(pr?.id);
  const drafts = useStore(appStore, (s: UIState) => s.reviewDrafts);
  const outdatedDraftCount = drafts.filter((draft) =>
    isDraftOutdated(draft, pr?.head_sha ?? ''),
  ).length;

  // ── Triage (since last review, or an explicit patchset pair) ──────
  const sinceLastReview = useStore(
    appStore,
    (s: UIState) => s.sinceLastReviewActive,
  );
  const triageReq = useMemo<TriageRequest | null>(() => {
    if (sinceLastReview) return { kind: 'sinceLastReview' };
    if (selectedPatchsetPair !== null) {
      const [from, to] = selectedPatchsetPair;
      if (from !== to)
        return { kind: 'pair', fromPatchset: from, toPatchset: to };
    }
    return null;
  }, [sinceLastReview, selectedPatchsetPair]);
  const {
    result: triage,
    loading: triageLoading,
    error: triageError,
  } = useTriage(pr?.id, triageReq);
  const triageOk = triage?.state === 'ok' ? triage : null;

  const triageStatuses = useMemo(
    () =>
      triageOk === null
        ? undefined
        : new Map(triageOk.files.map((file) => [file.path, file.status])),
    [triageOk],
  );
  const visibleDiff = useMemo(() => {
    if (triageOk === null) return diff;
    const shown = triageOk.files
      .filter((file) => file.status === 'changed' || file.status === 'new')
      .map((file) => file.path);
    return filterDiffToPaths(diff, new Set(shown));
  }, [diff, triageOk]);
  const removedPaths =
    triageOk?.files
      .filter((file) => file.status === 'removed')
      .map((file) => file.path) ?? [];

  const triageLabel =
    triageReq === null
      ? null
      : triageReq.kind === 'sinceLastReview'
        ? 'Since last review'
        : `P${triageReq.fromPatchset} → P${triageReq.toPatchset}`;
  const triageStatusText =
    triageReq === null
      ? null
      : triageLoading
        ? 'loading triage…'
        : triageError !== null
          ? `triage failed: ${triageError}`
          : triage === null
            ? null
            : triageSummary(triage, new Date());

  // Start of the diff-open measurement; reset whenever the route's PR changes.
  const openedAtRef = useRef(performance.now());
  // Clear stale diffFileKeys whenever the target PR changes.
  // This prevents keybindings from using keys from a previous PR's diff.
  // Collapse and whitespace settings are PR-scoped too; they reset here, not
  // on DiffView unmount, because the interdiff toggle swaps DiffViews mid-PR.
  useLayoutEffect(() => {
    openedAtRef.current = performance.now();
    if (prId === '') return;
    appStore.setState((s) => ({
      ...s,
      diffFileKeys: [],
      diffCollapseOverrides: [],
      hideWhitespaceHunks: false,
    }));
  }, [prId]);

  // One line per diff open so big-PR regressions show up in user logs.
  // rAF after commit ≈ the next frame with the diff committed (pre-paint,
  // before highlighting).
  useEffect(() => {
    if (diff === '') return;
    const frame = requestAnimationFrame(() =>
      log.debug(
        {
          prId,
          fileCount: files.length,
          payloadChars: diff.length,
          firstFileMs: Math.round(performance.now() - openedAtRef.current),
        },
        'diff opened',
      ),
    );
    return () => cancelAnimationFrame(frame);
  }, [prId, diff, files.length]);

  // Consume onFilesResolved via useLayoutEffect to ensure diffFileKeys
  // commits synchronously before the next user input (mirrors visiblePRIds pattern).
  const handleFilesResolved = (canonicalKeys: string[]): void => {
    appStore.setState((s) => ({ ...s, diffFileKeys: canonicalKeys }));
  };

  const currentFile = useMemo(() => {
    const path = splitUnifiedDiff(visibleDiff)[selectedFileIndex]?.path;
    return files.find((file) => file.path === path);
  }, [visibleDiff, selectedFileIndex, files]);
  const interdiffActive = useStore(appStore, (s: UIState) => s.interdiffActive);
  const canInterdiff =
    triageOk !== null &&
    currentFile !== undefined &&
    triageStatuses?.get(currentFile.path) === 'changed';
  const interdiffReq = useMemo<InterdiffRequest | null>(
    () =>
      interdiffActive &&
      canInterdiff &&
      triageOk !== null &&
      currentFile !== undefined
        ? {
            path: currentFile.path,
            fromPatchset: triageOk.from_patchset,
            toPatchset: triageOk.to_patchset,
          }
        : null,
    [interdiffActive, canInterdiff, triageOk, currentFile],
  );
  const interdiff = useInterdiff(pr?.id, interdiffReq);
  // Sidebar and scroll selection only move the store index, so record the
  // current file in ?file before the normal DiffView remounts on return.
  const isShowingInterdiff = interdiffReq !== null;
  useEffect(() => {
    if (!isShowingInterdiff) return;
    const { diffFileKeys, selectedFileIndex: index } = appStore.state;
    const file = diffFileKeys[index];
    if (file === undefined) return;
    void navigate({
      to: '/prs/$prId/diff',
      params: { prId },
      search: (prev) => ({ ...prev, file }),
      replace: true,
    });
  }, [isShowingInterdiff, navigate, prId]);
  const interdiffLabel =
    interdiffReq === null
      ? null
      : `interdiff P${interdiffReq.fromPatchset} → P${triageOk?.to_patchset}`;
  const interdiffHint =
    interdiffActive && !canInterdiff
      ? 'interdiff needs a changed file in triage (press s)'
      : null;
  const {
    state: openInEditorState,
    checkoutAndOpen,
    openCurrentVersion,
    cancel: cancelOpenInEditor,
    pending: openInEditorPending,
  } = useDiffPageActions({
    pr,
    diff,
    currentFile,
    threads,
    confirmingAction,
    setFiles,
  });

  const handleBack = (): void => {
    void navigate({ to: '/prs/$prId', params: { prId } });
  };

  const viewedPaths = useMemo(
    () => new Set(files.filter((f) => f.viewed).map((f) => f.path)),
    [files],
  );

  if (!prLoading && pr === undefined) {
    return (
      <div className="flex-1 flex items-center justify-center text-error">
        PR not found
      </div>
    );
  }

  return (
    <main className="flex-1 flex flex-col overflow-hidden bg-surface text-on-surface">
      <DiffPageToolbar
        pr={pr}
        triageLabel={triageLabel}
        interdiffLabel={interdiffLabel}
        isRebased={interdiff.result?.rebased === true && interdiffReq !== null}
        findQuery={findQuery}
        onFindQueryChange={setFindQuery}
        onBack={handleBack}
      />

      <DiffPageBody
        isAwaitingHydration={isAwaitingHydration}
        hydrationError={hydrationError}
        isHydrating={isHydrating}
        onRetry={retry}
        fullDiffLoading={loading}
        fullDiffError={diffError}
      >
        {isShowingInterdiff ? (
          <InterdiffPane
            result={interdiff.result}
            loading={interdiff.loading}
            error={interdiff.error}
            theme={theme}
            diffStyle={diffStyle}
          />
        ) : (
          <div className="h-full flex flex-col">
            {removedPaths.length > 0 && (
              <div className="shrink-0 px-4 py-1.5 border-b border-outline-variant/30 bg-surface-container-low font-mono text-[10px] text-outline">
                Removed since P{triageOk?.from_patchset}:{' '}
                {removedPaths.join(', ')}
              </div>
            )}
            <div className="flex-1 min-h-0">
              <DiffView
                diff={visibleDiff}
                {...(triageStatuses !== undefined ? { triageStatuses } : {})}
                theme={theme}
                diffStyle={diffStyle}
                findQuery={findQuery}
                selectedFileKey={fileKey}
                onFilesResolved={handleFilesResolved}
                threads={threads}
                comments={comments}
                onThreadReply={(threadId, body) => {
                  if (pr === undefined) return;
                  replyComment(pr.id, body, { threadId });
                }}
                prBrowserUrl={pr === undefined ? null : prBrowserUrl(pr)}
                expertiseFiles={expertise?.files ?? []}
                expertiseError={
                  expertiseStatus?.status === 'error'
                    ? (expertiseStatus.message ??
                      expertiseStatus.code ??
                      'unknown')
                    : null
                }
                viewedPaths={viewedPaths}
                oversizedPatchBytes={oversizedPatchBytes}
                prId={pr?.id}
                headSha={pr?.head_sha}
                drafts={drafts}
              />
            </div>
          </div>
        )}
      </DiffPageBody>

      <DiffPageStatusBar
        diff={visibleDiff}
        files={files}
        currentFile={currentFile}
        triageSummary={interdiffHint ?? triageStatusText}
        draftCount={drafts.length}
        outdatedDraftCount={outdatedDraftCount}
      />

      <OpenInEditorDialog
        dialog={openInEditorState.dialog}
        onCheckoutAndOpen={() => void checkoutAndOpen()}
        onOpenCurrentVersion={() => void openCurrentVersion()}
        onCancel={cancelOpenInEditor}
        pending={openInEditorPending}
      />
    </main>
  );
}
