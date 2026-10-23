// DiffPage — route-owned diff screen with stable file identity via canonical path keys
//
// PR resolved by $prId from /prs/$prId/diff route param (NEVER selectedPRIndex).
// File resolved by ?file canonical key via findFileByKey (NEVER selectedFileIndex).
// Unknown ?file → first-file fallback.
// Back button navigates to /prs/$prId via router.
// diffFileKeys are cleared when prId changes to prevent stale key carryover.

import { useNavigate, useParams, useSearch } from '@tanstack/react-router';
import { useLayoutEffect, useMemo, useState } from 'react';
import { DiffView } from '../components/diff/DiffView';
import { OpenInEditorDialog } from '../components/OpenInEditorDialog';
import { useExpertise } from '../hooks/useExpertise';
import { usePRHydration } from '../hooks/usePRHydration';
import { type RangeDiffRequest, useRangeDiff } from '../hooks/useRangeDiff';
import { useReviewThreads } from '../hooks/useReviewThreads';
import { useStore } from '../hooks/useStore';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { replyComment } from '../services/prActions';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import { repoIdForPR } from '../utils/expertise';
import { prBrowserUrl } from '../utils/triage';
import {
  DiffPageBody,
  DiffPageStatusBar,
  DiffPageToolbar,
} from './DiffPageChrome';
import { useDiffPageActions } from './useDiffPageActions';
import { useDiffPageData } from './useDiffPageData';

/**
 * Full-screen diff view — matches `diff_view_minimal_light/code.html`.
 *
 * Two diff surfaces share this page:
 *   - the full PR diff (default), rendered by {@link DiffView}
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
  const { prId } = useParams({ from: '/authed/prs/$prId/diff' });
  const { file: fileKey } = useSearch({ from: '/authed/prs/$prId/diff' });
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
  const { diff, files, setFiles, loading, diffError, comments } =
    useDiffPageData({ pr, isAwaitingHydration, commentsRevision });

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

  const currentFile = files[selectedFileIndex];
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
  // 'too_large' only appears on rows synced before the per-PR budget was removed.
  const oversizedPatchBytes = useMemo(
    () =>
      new Map(
        files
          .filter((f) => !f.is_binary && f.diff_status !== 'ok')
          .map((f) => [f.path, f.patch_bytes]),
      ),
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
        showRangeDiff={showRangeDiff}
        sinceLastReview={sinceLastReview}
        selectedPatchsetPair={selectedPatchsetPair}
        findQuery={findQuery}
        onFindQueryChange={setFindQuery}
        onBack={handleBack}
      />

      <DiffPageBody
        isAwaitingHydration={isAwaitingHydration}
        hydrationError={hydrationError}
        isHydrating={isHydrating}
        onRetry={retry}
        showRangeDiff={showRangeDiff}
        rangeLoading={rangeLoading}
        rangeError={rangeError}
        rangeResult={rangeResult}
        fullDiffLoading={loading}
        fullDiffError={diffError}
      >
        <DiffView
          diff={diff}
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
              ? (expertiseStatus.message ?? expertiseStatus.code ?? 'unknown')
              : null
          }
          viewedPaths={viewedPaths}
          oversizedPatchBytes={oversizedPatchBytes}
        />
      </DiffPageBody>

      <DiffPageStatusBar
        diff={diff}
        files={files}
        currentFile={currentFile}
        showRangeDiff={showRangeDiff}
        rangeResult={rangeResult}
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
