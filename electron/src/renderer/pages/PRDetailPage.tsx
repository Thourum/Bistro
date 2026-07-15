import { Link, useParams } from '@tanstack/react-router';
import { useEffect, useMemo, useRef, useState } from 'react';
import ActionFeedback from '../components/ActionFeedback';
import { FooterCenterSlot } from '../components/FooterSlot';
import { Markdown } from '../components/Markdown';
import { MergeReadinessBadge } from '../components/MergeReadinessBadge';
import { PatchsetTimeline } from '../components/PatchsetTimeline';
import StatusSidebar from '../components/StatusSidebar';
import { UnresolvedThreadsView } from '../components/UnresolvedThreadsView';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { ShortcutBadge } from '../components/ui/shortcut-badge';
import { Textarea } from '../components/ui/textarea';
import { useActionDispatch } from '../hooks/useActionDispatch';
import { useDismissals } from '../hooks/useDismissals';
import { usePatchsets } from '../hooks/usePatchsets';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { CICheck, Comment, Patchset, PR } from '../types';
import {
  filterBySeverity,
  groupVisibleFindings,
  isAutoCollapsed,
  sortBySeverity,
} from '../utils/classification';
import { touchesYouCount } from '../utils/expertise';
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
  const commentsRevision = useStore(
    appStore,
    (s: UIState) => s.commentsRevision,
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

  const { request } = useWebSocket();
  const { patchsets } = usePatchsets(pr?.id);

  function handleSelectPair(from: Patchset, to: Patchset): void {
    appStore.setState((s) => ({
      ...s,
      selectedPatchsetPair: [from.number, to.number],
    }));
  }

  const {
    dispatchApprove,
    dispatchRequestChanges,
    dispatchPostComment,
    clearActionResult,
  } = useActionDispatch(pr?.id);

  // Reset patchset pair selection when the user navigates to a different PR.
  useEffect(() => {
    if (selectedPrId === undefined) return;
    appStore.setState((s) => ({ ...s, selectedPatchsetPair: null }));
  }, [selectedPrId]);

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
    log.debug({ commentsRevision }, 'fetching comments');
    request('REQUEST_COMMENTS', { pr_id: pr.id })
      .then((data) => {
        const payload = data as { comments?: Comment[] };
        setComments(payload.comments ?? []);
      })
      .catch((err) => {
        log.warn({ err }, 'failed to fetch comments');
      });
  }, [pr, request, commentsRevision]);

  // d n / d b / u bulk-dismiss keystrokes act on the loaded comment list.
  useDismissals(pr?.id, comments);

  useEffect(() => {
    if (pr === undefined) return;

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

  if (pr === undefined) {
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

        {confirmingAction === 'comment' && (
          <PRCommentComposer
            title="Add PR comment"
            ariaLabel="PR comment"
            placeholder="Write a PR comment…"
            submitLabel="Comment"
            onCancel={() => {
              appStore.setState((s) => ({ ...s, confirmingAction: null }));
            }}
            onSubmit={dispatchPostComment}
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
            onSubmit={dispatchRequestChanges}
          />
        )}

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

function PRCommentComposer({
  title,
  ariaLabel,
  placeholder,
  submitLabel,
  onCancel,
  onSubmit,
}: {
  title: string;
  ariaLabel: string;
  placeholder: string;
  submitLabel: string;
  onCancel: () => void;
  onSubmit: (body: string) => void;
}): React.ReactElement {
  const [body, setBody] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const trimmed = body.trim();

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const submit = (): void => {
    if (trimmed.length === 0) return;
    onSubmit(trimmed);
    setBody('');
  };

  return (
    <section className="max-w-3xl border border-outline-variant/40 bg-surface-container-low p-5 space-y-3">
      <div className="flex items-center justify-between gap-4">
        <h2 className="font-mono text-[11px] font-bold tracking-[0.25em] uppercase">
          {title}
        </h2>
      </div>
      <Textarea
        ref={textareaRef}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            submit();
          }
          if (e.key === 'Escape') {
            e.preventDefault();
            onCancel();
          }
        }}
        rows={5}
        placeholder={placeholder}
        aria-label={ariaLabel}
      />
      <div className="flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          variant="default"
          size="sm"
          onClick={submit}
          disabled={trimmed.length === 0}
        >
          {submitLabel}
          <ShortcutBadge hotkey="Mod+Enter" />
        </Button>
      </div>
    </section>
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
        <MetaCell
          label="Source"
          value={
            <Badge
              variant="surface"
              className="text-xs normal-case tracking-normal"
            >
              {pr.head_branch}
            </Badge>
          }
        />
        <MetaCell
          label="Target"
          value={
            <Badge
              variant="surface"
              className="text-xs normal-case tracking-normal"
            >
              {pr.base_branch}
            </Badge>
          }
        />
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
      <div className="border-l-2 border-primary/50 pl-6 py-2">
        <Markdown body={pr.body} />
      </div>
    </section>
  );
}

function RecentComments({ comments }: { comments: Comment[] }) {
  const [expandedByCommentId, setExpandedByCommentId] = useState<
    Record<string, boolean>
  >({});
  const commentMinSeverity = useStore(
    appStore,
    (s: UIState) => s.commentMinSeverity,
  );
  const commentHumanOnly = useStore(
    appStore,
    (s: UIState) => s.commentHumanOnly,
  );
  const activeComments = useMemo(
    () =>
      sortBySeverity(
        filterBySeverity(
          comments.filter((comment) => comment.dismissed !== true),
          {
            ...(commentMinSeverity !== null
              ? { minSeverity: commentMinSeverity }
              : {}),
            humanOnly: commentHumanOnly,
          },
        ),
      ),
    [comments, commentMinSeverity, commentHumanOnly],
  );
  // Dedup groups are a VIEW over visible comments: grouping runs AFTER the
  // dismissed filter so members recompute and fully-hidden groups disappear.
  const findings = useMemo(
    () => groupVisibleFindings(activeComments),
    [activeComments],
  );
  const dismissedComments = useMemo(
    () =>
      sortBySeverity(comments.filter((comment) => comment.dismissed === true)),
    [comments],
  );
  const unresolved = activeComments.length; // TODO: real resolution tracking

  const toggleExpanded = (commentId: string): void => {
    setExpandedByCommentId((current) => ({
      ...current,
      [commentId]: current[commentId] !== true,
    }));
  };

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
      <div className="space-y-3">
        {findings.map((finding) => {
          const c = finding.anchor;
          const isMerged = finding.members.length > 1;
          const isExpandable = isMerged || isAutoCollapsed(c);
          const isExpanded = expandedByCommentId[c.id] === true;
          const isCollapsed = isExpandable && !isExpanded;
          const severity = isMerged ? finding.severity : c.severity;
          const severityLabel = severity ?? 'unclassified';
          const vendorLabel = finding.vendors
            .map((vendor) => vendor.charAt(0).toUpperCase() + vendor.slice(1))
            .join(' + ');

          return (
            <div
              key={c.id}
              data-severity={severity ?? 'none'}
              className="cursor-crosshair border-l-2 border-outline-variant/30 bg-surface-container-lowest p-6 transition-colors duration-150 data-[severity=critical]:border-l-error data-[severity=warning]:border-l-tertiary data-[severity=suggestion]:border-l-primary data-[severity=nit]:border-l-secondary data-[severity=info]:border-l-outline hover:border-primary hover:bg-surface-container-low flex flex-col space-y-3"
            >
              <button
                type="button"
                className="flex w-full items-center justify-between gap-4 border-0 p-0 text-left"
                aria-expanded={!isCollapsed}
                onClick={() => {
                  if (isExpandable) toggleExpanded(c.id);
                }}
              >
                <span className="flex items-center gap-2 min-w-0">
                  {isExpandable && (
                    <span className="font-mono text-[10px] text-outline">
                      {isCollapsed ? '▶' : '▼'}
                    </span>
                  )}
                  <span className="font-mono text-[10px] font-bold text-on-surface uppercase tracking-widest">
                    {isMerged
                      ? `Flagged by ${vendorLabel} (${finding.members.length} sources)`
                      : `@${c.author}`}
                  </span>
                  <span className="font-mono text-[9px] uppercase tracking-widest text-outline">
                    {severityLabel}
                  </span>
                </span>
                <span className="font-mono text-[10px] text-outline shrink-0">
                  {relativeTime(c.created_at)}
                </span>
              </button>
              {!isCollapsed &&
                (isMerged ? (
                  <div className="space-y-3">
                    {finding.members.map((member) => (
                      <div
                        key={member.id}
                        className="border-l border-outline-variant/30 pl-4"
                      >
                        <span className="font-mono text-[9px] uppercase tracking-widest text-outline">
                          @{member.author}
                        </span>
                        <Markdown body={member.body} compact />
                      </div>
                    ))}
                  </div>
                ) : (
                  <Markdown body={c.body} compact />
                ))}
            </div>
          );
        })}
      </div>
      {dismissedComments.length > 0 && (
        <section className="border-t border-outline-variant/30 pt-4 space-y-2">
          <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline">
            Dismissed ({dismissedComments.length})
          </h3>
          {dismissedComments.map((c) => {
            const isExpanded = expandedByCommentId[c.id] === true;
            return (
              <div
                key={c.id}
                className="border-l-2 border-outline-variant/20 bg-surface-container-lowest/50 px-6 py-2 opacity-60"
              >
                <button
                  type="button"
                  className="flex w-full items-center justify-between gap-4 border-0 p-0 text-left"
                  aria-expanded={isExpanded}
                  onClick={() => toggleExpanded(c.id)}
                >
                  <span className="flex items-center gap-2 min-w-0">
                    <span className="font-mono text-[10px] text-outline">
                      {isExpanded ? '▼' : '▶'}
                    </span>
                    <span className="font-mono text-[10px] font-bold text-on-surface uppercase tracking-widest">
                      @{c.author}
                    </span>
                    <span className="font-mono text-[9px] uppercase tracking-widest text-outline">
                      {c.severity ?? 'unclassified'}
                    </span>
                  </span>
                  <span className="font-mono text-[10px] text-outline shrink-0">
                    {relativeTime(c.created_at)}
                  </span>
                </button>
                {isExpanded && <Markdown body={c.body} compact />}
              </div>
            );
          })}
        </section>
      )}
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
            className="grid grid-cols-[1fr_auto_auto] gap-4 items-center py-2 px-3 cursor-crosshair border-l-2 border-transparent hover:border-primary hover:bg-surface-container-low transition-colors duration-150 font-mono text-xs"
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
