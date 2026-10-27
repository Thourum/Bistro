import { useEffect, useMemo, useRef, useState } from 'react';
import { Markdown } from '../components/Markdown';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { ShortcutBadge } from '../components/ui/shortcut-badge';
import { Textarea } from '../components/ui/textarea';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { isDraftOutdated } from '../services/reviewDrafts';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { Comment, ExpertiseFileData, PR, ReviewDraft } from '../types';
import {
  filterBySeverity,
  groupVisibleFindings,
  isAutoCollapsed,
  sortBySeverity,
} from '../utils/classification';
import { formatWhy } from '../utils/expertise';
import { isTextareaSubmit, TEXTAREA_SUBMIT_HOTKEY } from '../utils/keybindings';
import { findActionKeys } from '../utils/legendPresets';

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

interface PendingDraftsNoticeProps {
  drafts: readonly ReviewDraft[];
  headSha: string;
}

/** Explains that pending drafts ride along with the next review action. */
export function PendingDraftsNotice({
  drafts,
  headSha,
}: PendingDraftsNoticeProps): React.ReactElement | null {
  const bindings = useEffectiveBindings();
  if (drafts.length === 0) return null;
  const approveKeys = findActionKeys(bindings, ['detail.approve']);
  const requestChangesKeys = findActionKeys(bindings, [
    'detail.requestChanges',
  ]);
  const outdated = drafts.filter((draft) =>
    isDraftOutdated(draft, headSha),
  ).length;
  return (
    <p className="font-mono text-[10px] uppercase tracking-widest text-primary">
      {drafts.length} pending {drafts.length === 1 ? 'draft' : 'drafts'}
      {outdated > 0 ? ` (${outdated} outdated)` : ''} — sent with your next
      approve{approveKeys !== undefined ? ` (${approveKeys})` : ''} or request
      changes
      {requestChangesKeys !== undefined ? ` (${requestChangesKeys})` : ''}, or
      submit from the command palette
    </p>
  );
}

interface PRCommentComposerProps {
  title: string;
  ariaLabel: string;
  placeholder: string;
  submitLabel: string;
  onCancel: () => void;
  onSubmit: (body: string) => void;
}

export function PRCommentComposer({
  title,
  ariaLabel,
  placeholder,
  submitLabel,
  onCancel,
  onSubmit,
}: PRCommentComposerProps): React.ReactElement {
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
          if (isTextareaSubmit(e)) {
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
          <ShortcutBadge hotkey={TEXTAREA_SUBMIT_HOTKEY} />
        </Button>
      </div>
    </section>
  );
}

interface HeaderProps {
  pr: PR;
  stats: { filesCount: number; additions: number; deletions: number };
}

export function Header({ pr, stats }: HeaderProps): React.ReactElement {
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

export function Description({ pr }: { pr: PR }): React.ReactElement | null {
  if (pr.body === null || pr.body.trim().length === 0) return null;
  return (
    <section className="max-w-3xl space-y-6">
      <div className="border-l-2 border-primary/50 pl-6 py-2">
        <Markdown body={pr.body} />
      </div>
    </section>
  );
}

export function RecentComments({
  comments,
}: {
  comments: Comment[];
}): React.ReactElement {
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
          {activeComments.length.toString().padStart(2, '0')} VISIBLE
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

interface FileListProps {
  files: Array<{ path: string; additions: number; deletions: number }>;
  expertiseFiles: ExpertiseFileData[];
  expertiseError: string | null;
  /** Index of the file that n/p keybindings select. */
  selectedIndex: number;
}

export function FileList({
  files,
  expertiseFiles,
  expertiseError,
  selectedIndex,
}: FileListProps): React.ReactElement {
  const expertiseByPath = useMemo(
    () => new Map(expertiseFiles.map((file) => [file.file_path, file])),
    [expertiseFiles],
  );
  const rowsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    rowsRef.current?.children[selectedIndex]?.scrollIntoView({
      block: 'nearest',
    });
  }, [selectedIndex]);

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
      {expertiseError !== null && (
        <p className="font-mono text-[10px] uppercase tracking-widest text-error">
          Expertise unavailable: {expertiseError}
        </p>
      )}
      <div ref={rowsRef} className="space-y-1">
        {files.map((f, index) => {
          const expertise = expertiseByPath.get(f.path);
          const isSelected = index === selectedIndex;
          return (
            <div
              key={f.path}
              data-selected={isSelected ? '' : undefined}
              className={`border-l-2 hover:border-primary hover:bg-surface-container-low transition-colors duration-150 ${
                isSelected
                  ? 'border-primary bg-surface-container-low'
                  : 'border-transparent'
              }`}
            >
              <div className="grid grid-cols-[1fr_auto_auto] gap-4 items-center py-2 px-3 cursor-crosshair font-mono text-xs">
                <span className="truncate text-on-surface">{f.path}</span>
                <span className="text-primary">+{f.additions}</span>
                <span className="text-error">-{f.deletions}</span>
              </div>
              {expertise !== undefined && expertise.experts.length > 0 && (
                <details className="px-3 pb-3 font-mono text-[10px] text-outline">
                  <summary className="cursor-crosshair uppercase tracking-widest text-on-surface-variant hover:text-on-surface">
                    Expertise why
                  </summary>
                  <ul className="mt-2 space-y-1 border-l border-outline-variant/30 pl-3">
                    {expertise.experts.map((expert) => (
                      <li key={`${f.path}-${expert.author_email}`}>
                        {formatWhy(expert)}
                      </li>
                    ))}
                  </ul>
                  {expertise.your_rank !== null && (
                    <p className="mt-2 text-primary">
                      Your rank: #{expertise.your_rank}
                    </p>
                  )}
                </details>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
