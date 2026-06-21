import { Link, useParams } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import ActionFeedback from '../components/ActionFeedback';
import { FooterCenterSlot } from '../components/FooterSlot';
import { MergeReadinessBadge } from '../components/MergeReadinessBadge';
import { PatchsetTimeline } from '../components/PatchsetTimeline';
import { UnresolvedThreadsView } from '../components/UnresolvedThreadsView';
import { useActionDispatch } from '../hooks/useActionDispatch';
import { usePatchsets } from '../hooks/usePatchsets';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { CICheck, Comment, Patchset, PR } from '../types';
import { createLogger } from '../utils/logger';

// Route API for reading the stable $prId param; detail never resolves by list index.

const log = createLogger('PRDetailPage');

function formatTimestamp(iso: string): string {
  return iso
    .replace('T', ' ')
    .replace(/\.\d+Z$/, '')
    .slice(0, 19);
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}S AGO`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}M AGO`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour}H AGO`;
  const day = Math.floor(hour / 24);
  return `${day}D AGO`;
}

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
  const { prId } = useParams({ strict: false }) as { prId: string };
  const { activeOverlay } = useOverlays();
  const confirmingAction = useStore(
    appStore,
    (s: UIState) => s.confirmingAction,
  );
  const pendingAction = useStore(appStore, (s: UIState) => s.pendingAction);
  const actionResult = useStore(appStore, (s: UIState) => s.actionResult);
  const filterState = useStore(appStore, (s: UIState) => s.filterState);

  // Fetch ALL PRs via current filter and find by id.
  // We use 'all' to ensure deep-linked PRs (any status) are always resolvable.
  const { data: allPRs = [], isLoading, error } = useWebSocketPRList('all');
  // Also try the current filter as a fallback source — whichever has the PR.
  const { data: filteredPRs = [] } = useWebSocketPRList(filterState);

  const pr =
    allPRs.find((p) => p.id === prId) ?? filteredPRs.find((p) => p.id === prId);

  const { request } = useWebSocket();
  const { patchsets } = usePatchsets(pr?.id);

  function handleSelectPair(from: Patchset, to: Patchset): void {
    appStore.setState((s) => ({
      ...s,
      selectedPatchsetPair: [from.number, to.number],
    }));
  }

  const { dispatchApprove, clearActionResult } = useActionDispatch(pr?.id);

  useEffect(() => {
    if (confirmingAction === 'approve_confirmed') {
      dispatchApprove();
    }
  }, [confirmingAction, dispatchApprove]);

  const [files, setFiles] = useState<
    Array<{ path: string; additions: number; deletions: number }>
  >([]);
  const [comments, setComments] = useState<Comment[]>([]);
  const [ciChecks, setCIChecks] = useState<CICheck[]>([]);
  const [stats, setStats] = useState<{
    filesCount: number;
    additions: number;
    deletions: number;
  }>({ filesCount: 0, additions: 0, deletions: 0 });

  useEffect(() => {
    if (pr === undefined) return;
    request('REQUEST_COMMENTS', { pr_id: pr.id })
      .then((data) => {
        const payload = data as { comments?: Comment[] };
        setComments(payload.comments ?? []);
      })
      .catch((err) => {
        log.warn({ err }, 'failed to fetch comments');
      });

    request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .then((data) => {
        const payload = data as {
          files?: Array<{
            path: string;
            additions: number;
            deletions: number;
          }>;
          additions?: number;
          deletions?: number;
        };
        const parsed = payload.files ?? [];
        setFiles(parsed);
        setStats({
          filesCount: parsed.length,
          additions:
            payload.additions ?? parsed.reduce((s, f) => s + f.additions, 0),
          deletions:
            payload.deletions ?? parsed.reduce((s, f) => s + f.deletions, 0),
        });
      })
      .catch((err) => {
        log.warn({ err }, 'failed to fetch diff stats');
      });

    request('REQUEST_CI_CHECKS', { pr_id: pr.id })
      .then((data) => {
        const payload = data as { checks?: CICheck[] };
        setCIChecks(payload.checks ?? []);
      })
      .catch(() => {
        // Non-critical — the Go side may not implement this yet.
      });
  }, [pr, request]);

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center text-on-surface-variant font-mono uppercase tracking-widest text-xs">
        Loading PR…
      </div>
    );
  }

  if (error !== null || pr === undefined) {
    return (
      <div
        data-testid="pr-not-found"
        className="flex-1 flex flex-col items-center justify-center gap-4 text-error"
      >
        <span>
          {error !== null
            ? `Failed to load PR: ${(error as Error).message}`
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

  return (
    <main
      data-testid="pr-detail-page"
      className="relative flex-1 flex flex-row overflow-hidden"
    >
      {/* Merge-readiness summary lives in the global footer's center slot. */}
      <FooterCenterSlot>
        <MergeReadinessBadge prId={pr.id} />
      </FooterCenterSlot>

      <article className="w-[90%] overflow-y-auto p-12 lg:p-16 flex flex-col space-y-12 bg-surface">
        <Header pr={pr} stats={stats} />
        <Description pr={pr} />

        <ActionFeedback
          pendingAction={pendingAction}
          actionResult={actionResult}
          onDismiss={clearActionResult}
        />

        <PatchsetTimeline
          patchsets={patchsets}
          onSelectPair={handleSelectPair}
        />

        {comments.length > 0 && <RecentComments comments={comments} />}

        {files.length > 0 && <FileList files={files} />}

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

      <StatusRail pr={pr} ciChecks={ciChecks} />

      {activeOverlay === 'threads' && (
        <UnresolvedThreadsView prId={pr.id} comments={comments} />
      )}
    </main>
  );
}

interface HeaderProps {
  pr: PR;
  stats: { filesCount: number; additions: number; deletions: number };
}

function Header({ pr, stats }: HeaderProps) {
  return (
    <header className="space-y-4">
      <div className="flex items-center space-x-3 mb-2">
        <span className="font-mono text-[10px] tracking-widest text-primary font-bold">
          PR-{pr.number}
        </span>
        <div className="h-px flex-grow bg-outline-variant opacity-20" />
      </div>
      <h1 className="text-4xl lg:text-5xl font-black tracking-tighter text-on-surface leading-none">
        {pr.title}
      </h1>

      <div className="flex flex-wrap gap-x-8 gap-y-4 pt-2">
        <MetaCell label="Author" value={`@${pr.author}`} />
        <MetaCell label="Date" value={formatTimestamp(pr.created_at)} />
        <MetaCell label="Source" value={<Badge>{pr.head_branch}</Badge>} />
        <MetaCell label="Target" value={<Badge>{pr.base_branch}</Badge>} />
        <MetaCell label="Files" value={stats.filesCount} />
        <MetaCell
          label="Added"
          valueClassName="text-primary"
          value={`+${stats.additions}`}
        />
        <MetaCell
          label="Removed"
          valueClassName="text-error"
          value={`-${stats.deletions}`}
        />
      </div>
    </header>
  );
}

function Badge({ children }: { children: React.ReactNode }) {
  return (
    <span className="font-mono text-xs font-medium bg-surface-container px-2 py-0.5">
      {children}
    </span>
  );
}

interface MetaCellProps {
  label: string;
  value: React.ReactNode;
  valueClassName?: string;
}

function MetaCell({
  label,
  value,
  valueClassName = '',
}: MetaCellProps): React.ReactElement {
  return (
    <div className="flex flex-col">
      <span className="font-mono text-[10px] uppercase text-outline tracking-widest">
        {label}
      </span>
      <span className={`font-mono text-xs font-bold ${valueClassName}`}>
        {value}
      </span>
    </div>
  );
}

function Description({ pr }: { pr: PR }) {
  if (pr.body === null || pr.body.trim().length === 0) return null;
  return (
    <section className="max-w-3xl space-y-6">
      <div className="border-l-2 border-primary-container pl-6 py-2">
        <p className="text-lg leading-relaxed text-on-surface-variant font-medium whitespace-pre-wrap">
          {pr.body}
        </p>
      </div>
    </section>
  );
}

function RecentComments({ comments }: { comments: Comment[] }) {
  const unresolved = comments.length; // TODO: real resolution tracking
  return (
    <section className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="font-mono text-[11px] font-bold tracking-[0.25em] uppercase">
          Recent Comments
        </h2>
        <span className="font-mono text-[10px] text-outline">
          {unresolved.toString().padStart(2, '0')} UNRESOLVED
        </span>
      </div>
      <div className="space-y-4">
        {comments.slice(0, 5).map((c) => (
          <div
            key={c.id}
            className="p-6 bg-surface-container-lowest hover:bg-surface-container-low transition-colors duration-150 flex flex-col space-y-3"
          >
            <div className="flex justify-between items-center">
              <span className="font-mono text-[10px] font-bold text-on-surface uppercase">
                @{c.author}
              </span>
              <span className="font-mono text-[10px] text-outline">
                {relativeTime(c.created_at)}
              </span>
            </div>
            <p className="text-sm text-on-surface-variant font-medium leading-relaxed whitespace-pre-wrap">
              {c.body}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}

function FileList({
  files,
}: {
  files: Array<{ path: string; additions: number; deletions: number }>;
}) {
  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="font-mono text-[11px] font-bold tracking-[0.25em] uppercase">
          Files Changed
        </h2>
        <span className="font-mono text-[10px] text-outline">
          {files.length.toString().padStart(2, '0')} FILES
        </span>
      </div>
      <div className="space-y-1">
        {files.map((f) => (
          <div
            key={f.path}
            className="grid grid-cols-[1fr_auto_auto] gap-4 items-center py-2 px-3 hover:bg-surface-container-low transition-colors font-mono text-xs"
          >
            <span className="truncate text-on-surface">{f.path}</span>
            <span className="text-primary">+{f.additions}</span>
            <span className="text-error">-{f.deletions}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

interface StatusRailProps {
  pr: PR;
  ciChecks: CICheck[];
}

function StatusRail({ pr, ciChecks }: StatusRailProps) {
  const ciPending = ciChecks.some((c) => c.state === 'pending');
  const ciFailed = ciChecks.some((c) => c.state === 'failure');
  const ciLabel = ciFailed
    ? 'Checks Failed'
    : ciPending
      ? 'Checks Running'
      : ciChecks.length > 0
        ? 'Checks Passing'
        : 'No CI';
  const reviewLabel =
    pr.review_status === 'approved'
      ? 'Approved'
      : pr.review_status === 'changes_requested'
        ? 'Changes Requested'
        : 'Awaiting Review';

  return (
    <aside className="w-[10%] min-w-[96px] bg-surface-container-low flex flex-col border-l border-outline-variant/10">
      <div className="flex-1 flex flex-col items-center justify-center space-y-16 py-12">
        <div className="flex flex-col items-center space-y-2">
          <div
            className={`w-8 h-8 border-2 flex items-center justify-center ${
              ciFailed ? 'border-error' : 'border-primary'
            }`}
          >
            <span
              className={`material-symbols-outlined text-lg ${
                ciFailed ? 'text-error' : 'text-primary'
              } ${ciPending ? 'animate-spin' : ''}`}
            >
              {ciFailed ? 'close' : ciPending ? 'sync' : 'check'}
            </span>
          </div>
          <span
            className={`font-mono text-[9px] font-bold tracking-widest uppercase text-center px-2 ${
              ciFailed ? 'text-error' : 'text-primary'
            }`}
          >
            {ciLabel}
          </span>
        </div>

        <div className="flex flex-col items-center space-y-2">
          <div className="w-8 h-8 border-2 border-outline-variant flex items-center justify-center">
            <span className="material-symbols-outlined text-outline text-lg">
              person
            </span>
          </div>
          <span className="font-mono text-[9px] font-bold tracking-widest text-outline uppercase text-center px-2">
            {reviewLabel}
          </span>
        </div>
      </div>
      <div className="h-24" />
    </aside>
  );
}
