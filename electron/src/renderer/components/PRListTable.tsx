import { useNavigate } from '@tanstack/react-router';
import type React from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import type { PR } from '../types';

/**
 * Minimalist PR inbox matching `pr_inbox_minimal_light/code.html`.
 *
 * - Flat rows on a 5-col grid `[24px_60px_1fr_120px_100px]`
 * - Optional repo grouping via `<details>/<summary>` when
 *   `settings.prList.view === 'grouped'`
 * - CI status dots (green / red / yellow / neutral)
 *
 * `prs`, `isLoading`, `error` are provided by the parent (PRListPage),
 * which calls `useVisiblePRs()`. The internal hook call has been removed so that
 * this component no longer has its own data source (the router is the single
 * source of navigation truth).
 */

function ciColor(pr: PR): string {
  switch (pr.ci_status) {
    case 'success':
      return 'bg-green-500';
    case 'failure':
      return 'bg-error';
    case 'pending':
    case 'running':
      return 'bg-yellow-500';
    default:
      return 'bg-outline-variant';
  }
}

function ciTitle(pr: PR): string {
  switch (pr.ci_status) {
    case 'success':
      return 'CI SUCCESS';
    case 'failure':
      return 'CI FAILED';
    case 'pending':
    case 'running':
      return 'CI RUNNING';
    default:
      return 'CI UNKNOWN';
  }
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour}h ago`;
  const day = Math.floor(hour / 24);
  return `${day}d ago`;
}

interface PRRowProps {
  pr: PR;
  globalIndex: number;
  selectedIndex: number;
}

function PRRow({
  pr,
  globalIndex,
  selectedIndex,
}: PRRowProps): React.ReactElement {
  const isSelected = selectedIndex === globalIndex;
  const navigate = useNavigate();
  const open = (): void => {
    // Navigate by stable pr.id — globalIndex is cursor only.
    void navigate({ to: '/prs/$prId', params: { prId: pr.id } });
    appStore.setState((s) => ({
      ...s,
      selectedPRIndex: globalIndex,
    }));
  };
  return (
    <button
      type="button"
      onClick={open}
      className={[
        'grid grid-cols-[24px_60px_1fr_120px_100px] items-center gap-4 py-3 px-4 w-full text-left',
        'cursor-crosshair transition-colors border-l-2',
        isSelected
          ? 'bg-surface-container-low border-primary'
          : 'border-transparent hover:bg-surface-container-low hover:border-primary',
      ].join(' ')}
    >
      <span
        className={`w-3 h-3 inline-block ${ciColor(pr)}`}
        title={ciTitle(pr)}
        role="img"
        aria-label={ciTitle(pr)}
      />
      <div className="font-mono text-[11px] text-outline">#{pr.number}</div>
      <div className="font-medium text-[14px] text-on-surface truncate">
        {pr.title}
      </div>
      <div className="font-mono text-[11px] text-outline-variant text-right truncate">
        @{pr.author}
      </div>
      <div
        className={`font-mono text-[11px] text-right ${
          isSelected ? 'text-primary' : 'text-outline-variant'
        }`}
      >
        {relativeTime(pr.updated_at)}
      </div>
    </button>
  );
}

interface PRGroupProps {
  title: string;
  prs: PR[];
  startIndex: number;
  selectedIndex: number;
  accent: boolean;
}

function PRGroup({
  title,
  prs,
  startIndex,
  selectedIndex,
  accent,
}: PRGroupProps): React.ReactElement {
  return (
    <details
      className={accent ? 'group' : 'group mt-8'}
      open={accent ? true : prs.length > 0}
    >
      <summary className="list-none cursor-pointer flex items-center justify-between py-4 group-open:bg-surface-container-low px-2 -mx-2 transition-colors">
        <div className="flex items-center gap-4">
          <span
            className={`text-[10px] font-mono px-1.5 py-0.5 border ${
              accent
                ? 'text-primary border-primary'
                : 'text-outline border-outline'
            }`}
          >
            GROUP
          </span>
          <span className="font-bold text-sm tracking-tight">{title}</span>
        </div>
        <span className="font-mono text-[10px] text-outline">
          TOGGLE [SPACE]
        </span>
      </summary>
      <div className="mt-2">
        {prs.map((pr, i) => (
          <PRRow
            key={pr.id}
            pr={pr}
            globalIndex={startIndex + i}
            selectedIndex={selectedIndex}
          />
        ))}
      </div>
    </details>
  );
}

export interface PRListTableProps {
  prs: PR[];
  isLoading: boolean;
  error: Error | null;
}

export function PRListTable({
  prs: visiblePRs,
  isLoading,
  error,
}: PRListTableProps) {
  const selectedIndex = useStore(
    appStore,
    (state: UIState) => state.selectedPRIndex,
  );
  const prListView = useStore(settingsStore, (s) => s.prList.view);
  const repos = useStore(appStore, (state: UIState) => state.repos);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16 text-on-surface-variant font-mono uppercase tracking-widest text-xs">
        Loading PRs…
      </div>
    );
  }

  if (error !== null) {
    return (
      <div className="flex items-center justify-center py-16 text-error font-mono text-sm">
        Failed to load PRs: {(error as Error).message}
      </div>
    );
  }

  const watchedRepos = (repos ?? []).filter((r) => r.watched);
  if (visiblePRs.length === 0 && watchedRepos.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-on-surface-variant space-y-2">
        <p className="text-lg font-semibold">No repositories watched</p>
        <p className="text-sm">
          Press{' '}
          <kbd className="px-1 py-0.5 bg-surface-container text-xs font-mono">
            :
          </kbd>{' '}
          then type{' '}
          <kbd className="px-1 py-0.5 bg-surface-container text-xs font-mono">
            repos
          </kbd>{' '}
          to discover and watch your GitHub repositories.
        </p>
      </div>
    );
  }

  if (prListView === 'grouped') {
    const groups = new Map<string, PR[]>();
    for (const pr of visiblePRs) {
      const key = `${pr.owner}/${pr.repo}`;
      const existing = groups.get(key);
      if (existing) existing.push(pr);
      else groups.set(key, [pr]);
    }
    let cursor = 0;
    return (
      <div className="space-y-0">
        {Array.from(groups.entries()).map(([repoKey, repoPRs], idx) => {
          const start = cursor;
          cursor += repoPRs.length;
          return (
            <PRGroup
              key={repoKey}
              title={repoKey}
              prs={repoPRs}
              startIndex={start}
              selectedIndex={selectedIndex}
              accent={idx === 0}
            />
          );
        })}
      </div>
    );
  }

  return (
    <div className="space-y-0">
      {visiblePRs.map((pr, idx) => (
        <PRRow
          key={pr.id}
          pr={pr}
          globalIndex={idx}
          selectedIndex={selectedIndex}
        />
      ))}
    </div>
  );
}
