import { Link, useParams } from '@tanstack/react-router';
import { useCallback, useEffect } from 'react';
import ActionFeedback from '../components/ActionFeedback';
import { ExpertiseNudge } from '../components/ExpertiseNudge';
import { FooterCenterSlot } from '../components/FooterSlot';
import { MergeReadinessBadge } from '../components/MergeReadinessBadge';
import { PatchsetTimeline } from '../components/PatchsetTimeline';
import StatusSidebar from '../components/StatusSidebar';
import { UnresolvedThreadsView } from '../components/UnresolvedThreadsView';
import { useDismissals } from '../hooks/useDismissals';
import { useExpertise } from '../hooks/useExpertise';
import { usePatchsets } from '../hooks/usePatchsets';
import { usePRHydration } from '../hooks/usePRHydration';
import { useStore } from '../hooks/useStore';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { useOverlays } from '../overlays/OverlayContext';
import {
  approvePR,
  clearActionResult,
  postComment,
  requestChanges,
} from '../services/prActions';
import { markPRsRead } from '../services/readState';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { Patchset } from '../types';
import { repoIdForPR, touchesYouCount } from '../utils/expertise';
import {
  Description,
  FileList,
  Header,
  PRCommentComposer,
  RecentComments,
} from './PRDetailSections';
import { usePRDetailData } from './usePRDetailData';

/**
 * PR overview — matches `pr_overview_minimal_refined/code.html`.
 *
 * 90/10 split with the editorial article on the left and a vertical CI /
 * review status rail on the right. Press `SPACE` on an open PR to switch to
 * the diff view. Inline comments are intentionally disabled this pass —
 * re-enabled once `@pierre/diffs` gains first-class comment gutters.
 */
export function PRDetailPage() {
  // Resolve PR by stable id from route params — NEVER by selectedPRIndex.
  const { prId } = useParams({ from: '/authed/prs/$prId' });
  const { activeOverlay } = useOverlays();
  const confirmingAction = useStore(
    appStore,
    (s: UIState) => s.confirmingAction,
  );
  const commentsRevision = useStore(
    appStore,
    (s: UIState) => s.commentsRevision,
  );
  const selectedFileIndex = useStore(
    appStore,
    (s: UIState) => s.selectedFileIndex,
  );
  const pendingAction = useStore(appStore, (s: UIState) => s.pendingAction);
  const actionResult = useStore(appStore, (s: UIState) => s.actionResult);
  const filterState = useStore(appStore, (s: UIState) => s.filterState);

  // Fetch ALL PRs via current filter and find by id.
  // We use 'all' to ensure deep-linked PRs (any status) are always resolvable.
  const { data: allPRs = [], isLoading, error } = useWebSocketPRList('all');
  const { data: openPRs = [] } = useWebSocketPRList('open');
  // Also try the current filter as a fallback source — whichever has the PR.
  const { data: filteredPRs = [] } = useWebSocketPRList(filterState);

  const pr =
    allPRs.find((p) => p.id === prId) ?? filteredPRs.find((p) => p.id === prId);
  const selectedPrId = pr?.id;

  const { isHydrating, error: hydrationError, retry } = usePRHydration(pr);
  const isAwaitingHydration = pr?.hydrated === 0;
  const detailPrId = isAwaitingHydration ? undefined : pr?.id;
  const { patchsets } = usePatchsets(detailPrId);
  const { expertise } = useExpertise(detailPrId);
  const expertiseStatus = useStore(appStore, (s: UIState) =>
    pr !== undefined ? s.expertiseRepoStatuses?.[repoIdForPR(pr)] : undefined,
  );

  const handleSelectPair = useCallback((from: Patchset, to: Patchset): void => {
    appStore.setState((s) => ({
      ...s,
      selectedPatchsetPair: [from.number, to.number],
    }));
  }, []);

  // Reset patchset pair selection when the user navigates to a different PR.
  useEffect(() => {
    if (selectedPrId === undefined) return;
    appStore.setState((s) => ({ ...s, selectedPatchsetPair: null }));
  }, [selectedPrId]);

  // Keep the file cursor when returning from this PR's diff; start at the
  // first file for any other PR.
  useEffect(() => {
    if (selectedPrId === undefined) return;
    if (appStore.state.selectedFilePrId === selectedPrId) return;
    appStore.setState((s) => ({
      ...s,
      selectedFileIndex: 0,
      selectedFilePrId: selectedPrId,
    }));
  }, [selectedPrId]);

  useEffect(() => {
    // Read state is local-only and silently drops when the WebSocket is down.
    void markPRsRead([prId]);
  }, [prId]);

  useEffect(() => {
    if (confirmingAction !== 'approve_confirmed' || selectedPrId === undefined)
      return;
    approvePR(selectedPrId);
  }, [confirmingAction, selectedPrId]);

  const { files, comments, ciChecks, stats } = usePRDetailData(
    pr,
    isAwaitingHydration,
    commentsRevision,
  );

  // n/p and Space read the file list from the store, so publish this page's
  // file paths there (the diff route replaces them with its parsed keys).
  useEffect(() => {
    const fileKeys = files.map((file) => file.path);
    appStore.setState((s) => ({ ...s, diffFileKeys: fileKeys }));
  }, [files]);

  // d n / d b / u bulk-dismiss keystrokes act on the loaded comment list.
  useDismissals(pr?.id, comments);

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center text-on-surface-variant font-mono uppercase tracking-widest text-xs">
        Loading PR…
      </div>
    );
  }

  if (pr === undefined) {
    return (
      <div
        data-testid="pr-not-found"
        className="flex-1 flex flex-col items-center justify-center gap-4 text-error"
      >
        <span>
          {error !== null
            ? `Failed to load PR: ${error.message}`
            : `PR not found: ${prId}`}
        </span>
        <Link
          to="/prs"
          className="font-mono text-xs text-primary underline uppercase tracking-widest"
        >
          ← Back to PRs
        </Link>
      </div>
    );
  }

  if (isAwaitingHydration) {
    return (
      <HydrationState
        error={hydrationError}
        isHydrating={isHydrating}
        onRetry={retry}
      />
    );
  }

  return (
    <main
      data-testid="pr-detail-page"
      className="relative flex-1 flex flex-row overflow-hidden"
    >
      {/* Merge-readiness summary lives in the global footer's center slot. */}
      <FooterCenterSlot>
        <MergeReadinessBadge prId={pr.id} />
      </FooterCenterSlot>

      <article
        data-diff-viewport
        className="w-[90%] overflow-y-auto p-12 lg:p-16 flex flex-col space-y-12 bg-surface"
      >
        <Header pr={pr} stats={stats} />
        <Description pr={pr} />

        <ActionFeedback
          pendingAction={pendingAction}
          actionResult={actionResult}
          onDismiss={clearActionResult}
        />

        <ExpertiseNudge repoId={repoIdForPR(pr)} />

        <PatchsetTimeline
          patchsets={patchsets}
          onSelectPair={handleSelectPair}
        />

        {confirmingAction === 'comment' && (
          <PRCommentComposer
            title="Add PR comment"
            ariaLabel="PR comment"
            placeholder="Write a PR comment…"
            submitLabel="Comment"
            onCancel={() => {
              appStore.setState((s) => ({ ...s, confirmingAction: null }));
            }}
            onSubmit={(body) => postComment(pr.id, body)}
          />
        )}

        {confirmingAction === 'request_changes' && (
          <PRCommentComposer
            title="Request changes"
            ariaLabel="Request changes comment"
            placeholder="Describe the changes needed before approval…"
            submitLabel="Request changes"
            onCancel={() => {
              appStore.setState((s) => ({ ...s, confirmingAction: null }));
            }}
            onSubmit={(comment) => requestChanges(pr.id, comment)}
          />
        )}

        {comments.length > 0 && <RecentComments comments={comments} />}

        {files.length > 0 && (
          <FileList
            files={files}
            selectedIndex={selectedFileIndex}
            expertiseFiles={expertise?.files ?? []}
            expertiseError={
              expertiseStatus?.status === 'error'
                ? (expertiseStatus.message ?? expertiseStatus.code ?? 'unknown')
                : null
            }
          />
        )}

        <footer className="mt-auto pt-12">
          <div className="inline-flex items-center gap-x-6">
            <div className="inline-flex items-center space-x-3 bg-surface-container-high px-4 py-2">
              <span className="material-symbols-outlined text-base">
                keyboard_command_key
              </span>
              <span className="font-mono text-[10px] uppercase tracking-widest text-on-surface">
                Press <span className="font-black text-primary">[SPACE]</span>{' '}
                to view diff
              </span>
            </div>
            <div className="inline-flex items-center space-x-3 bg-surface-container-high px-4 py-2">
              <span className="material-symbols-outlined text-base">forum</span>
              <span className="font-mono text-[10px] uppercase tracking-widest text-on-surface">
                Press <span className="font-black text-primary">[G U]</span> for
                review threads
              </span>
            </div>
          </div>
        </footer>
      </article>

      <StatusSidebar
        pr={pr}
        ciChecks={ciChecks}
        comments={comments}
        touchesYouCount={touchesYouCount(openPRs)}
      />

      {activeOverlay === 'threads' && (
        <UnresolvedThreadsView prId={pr.id} comments={comments} />
      )}
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
    return (
      <div className="flex-1 flex items-center justify-center text-on-surface-variant font-mono uppercase tracking-widest text-xs">
        Preparing pull request details…
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col items-center justify-center gap-3 text-error text-sm">
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
