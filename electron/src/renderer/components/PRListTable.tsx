import { useNavigate } from '@tanstack/react-router';
import { Fault } from '@thourum/fault';
import type React from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import { toggleRepoGroup } from '../store/ui';
import type { PR, RepoInfo } from '../types';
import { LANES, laneSections } from '../utils/lanes';
import { findActionKeys } from '../utils/legendPresets';
import { RepoRecoveryDialog } from './RepoRecoveryDialog';

export type PRRepoGroup = readonly [repoKey: string, prs: PR[]];

function groupPRsByRepo(prs: PR[]): PRRepoGroup[] {
  const groups = new Map<string, PR[]>();
  for (const pr of prs) {
    const key = `${pr.owner}/${pr.repo}`;
    const existing = groups.get(key) ?? [];
    groups.set(key, [...existing, pr]);
  }
  return Array.from(groups.entries());
}

/**
 * One cursor row in the grouped PR list. An expanded group contributes one
 * row per PR; a collapsed group contributes a single header row (pr: null).
 * This is the single source of truth for keyboard navigation — j/k walk
 * these rows, and `visiblePRIds` / `visiblePRRepoKeys` are projections of it.
 */
export interface PRListRow {
  pr: PR | null;
  repoKey: string;
}

export function visiblePRListRows(
  prs: PR[],
  collapsedRepoKeys: readonly string[],
): PRListRow[] {
  return groupPRsByRepo(prs).flatMap(([repoKey, repoPRs]): PRListRow[] =>
    collapsedRepoKeys.includes(repoKey)
      ? [{ pr: null, repoKey }]
      : repoPRs.map((pr) => ({ pr, repoKey })),
  );
}

/**
 * Minimalist PR inbox matching `pr_inbox_minimal_light/code.html`.
 *
 * - Flat rows on a 5-col grid `[24px_60px_1fr_120px_100px]`
 * - Repo grouping via `<details>/<summary>`; open state is controlled by
 *   `collapsedRepoKeys` in the app store so Space (keybinding) and mouse
 *   clicks share one toggle path.
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
  degradedRepo: RepoInfo | null;
  onRecover: (repo: RepoInfo) => void;
  /** Lanes mix repos, so the row names its repo; grouped rows already sit under a repo header. */
  showRepo?: boolean;
}

function PRRow({
  pr,
  globalIndex,
  selectedIndex,
  degradedRepo,
  onRecover,
  showRepo = false,
}: PRRowProps): React.ReactElement {
  const isSelected = selectedIndex === globalIndex;
  const isDegraded = degradedRepo !== null;
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
    <div className={isDegraded ? 'opacity-60' : undefined}>
      <button
        type="button"
        onClick={open}
        aria-label={`Open PR #${pr.number} in ${pr.owner}/${pr.repo}`}
        className={[
          showRepo
            ? 'grid grid-cols-[24px_minmax(0,220px)_1fr_120px_100px]'
            : 'grid grid-cols-[24px_60px_1fr_120px_100px]',
          'items-center gap-4 py-3 px-4 w-full text-left',
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
        <div className="font-mono text-[11px] text-outline truncate">
          {showRepo ? `${pr.owner}/${pr.repo}#${pr.number}` : `#${pr.number}`}
        </div>
        <div
          className={`text-[14px] text-on-surface truncate ${
            pr.unread ? 'font-semibold' : 'font-medium'
          }`}
        >
          {pr.unread && (
            <span className="mr-2 text-primary" role="img" aria-label="Unread">
              ●
            </span>
          )}
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
      {isDegraded && (
        <div className="flex items-center justify-between gap-4 px-4 pb-3 text-xs text-error">
          <span>
            {degradedRepo.location_error ?? 'Repository location unavailable.'}
          </span>
          <button
            type="button"
            onClick={() => onRecover(degradedRepo)}
            className="font-mono underline"
          >
            Recover {pr.owner}/{pr.repo}
          </button>
        </div>
      )}
    </div>
  );
}

interface PRLaneSectionProps {
  label: string;
  lane: number;
  prs: PR[];
  startIndex: number;
  selectedIndex: number;
  repoByKey: ReadonlyMap<string, RepoInfo>;
  onRecover: (repo: RepoInfo) => void;
}

function PRLaneSection({
  label,
  lane,
  prs,
  startIndex,
  selectedIndex,
  repoByKey,
  onRecover,
}: PRLaneSectionProps): React.ReactElement {
  return (
    <section className="mt-8 first:mt-0">
      <div className="py-4 px-2 -mx-2 border-b border-outline-variant/30 text-[10px] font-mono text-outline uppercase tracking-widest">
        {lane} {label} — {prs.length}
      </div>
      {prs.length === 0 ? (
        <div className="py-3 px-4 font-mono text-[11px] text-outline-variant">
          — clear —
        </div>
      ) : (
        prs.map((pr, index) => {
          const repo = repoByKey.get(`${pr.owner}/${pr.repo}`) ?? null;
          const degradedRepo =
            repo !== null &&
            repo.location_status !== null &&
            repo.location_status !== undefined &&
            repo.location_status !== 'ok'
              ? repo
              : null;
          return (
            <PRRow
              key={pr.id}
              pr={pr}
              globalIndex={startIndex + index}
              selectedIndex={selectedIndex}
              degradedRepo={degradedRepo}
              onRecover={onRecover}
              showRepo
            />
          );
        })
      )}
    </section>
  );
}

interface PRGroupProps {
  repoKey: string;
  prs: PR[];
  collapsed: boolean;
  /** Cursor index of this group's first row (header row when collapsed). */
  startIndex: number;
  selectedIndex: number;
  accent: boolean;
  degradedRepo: RepoInfo | null;
  onRecover: (repo: RepoInfo) => void;
}

function PRGroup({
  repoKey,
  prs,
  collapsed,
  startIndex,
  selectedIndex,
  accent,
  degradedRepo,
  onRecover,
}: PRGroupProps): React.ReactElement {
  // A collapsed group is a single cursor row; highlight its header.
  const headerSelected = collapsed && selectedIndex === startIndex;
  const toggleKeys = findActionKeys(useEffectiveBindings(), [
    'list.toggleGroup',
  ]);
  const toggle = (e: React.MouseEvent): void => {
    // The store owns open/collapsed state — block the native <details> toggle
    // so mouse clicks and the Space keybinding share one code path.
    e.preventDefault();
    appStore.setState((s) => toggleRepoGroup(s, repoKey));
  };
  return (
    <details className={accent ? 'group' : 'group mt-8'} open={!collapsed}>
      <summary
        onClick={toggle}
        className={[
          'list-none cursor-pointer flex items-center justify-between py-4 px-2 -mx-2 transition-colors border-l-2',
          headerSelected
            ? 'bg-surface-container-low border-primary'
            : 'border-transparent group-open:bg-surface-container-low',
        ].join(' ')}
      >
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
          <span className="font-bold text-sm tracking-tight">{repoKey}</span>
          <span className="font-mono text-[10px] text-outline">
            {prs.length} {prs.length === 1 ? 'PR' : 'PRS'}
          </span>
        </div>
        {toggleKeys !== undefined && (
          <span className="font-mono text-[10px] text-outline uppercase">
            Toggle [{toggleKeys}]
          </span>
        )}
      </summary>
      <div className="mt-2">
        {!collapsed &&
          prs.map((pr, i) => (
            <PRRow
              key={pr.id}
              pr={pr}
              globalIndex={startIndex + i}
              selectedIndex={selectedIndex}
              degradedRepo={degradedRepo}
              onRecover={onRecover}
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
  const collapsedRepoKeys = useStore(
    appStore,
    (state: UIState) => state.collapsedRepoKeys,
  );
  const repos = useStore(appStore, (state: UIState) => state.repos);
  const viewerLogin = useStore(appStore, (state: UIState) => state.viewerLogin);
  const view = useStore(settingsStore, (state) => state.prList.view);
  const [recoveryRepo, setRecoveryRepo] = useState<RepoInfo | null>(null);
  const commandKeys = findActionKeys(useEffectiveBindings(), [
    'global.command',
  ]);
  const sections = useMemo(
    () => laneSections(visiblePRs, viewerLogin),
    [viewerLogin, visiblePRs],
  );

  useEffect(() => {
    appStore.setState((state) => ({
      ...state,
      reposLoading: true,
    }));
    void wsClient
      .request('REQUEST_REPO_LIST', {})
      .mapErr((cause) =>
        new Fault(cause).withTag('CONNECTION_ERROR').withCause(cause),
      )
      .match(
        () => undefined,
        () => {
          appStore.setState((state) => ({
            ...state,
            reposLoading: false,
          }));
        },
      );
  }, []);

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
        Failed to load PRs: {error.message}
      </div>
    );
  }

  const watchedRepos = (repos ?? []).filter((r) => r.watched);
  if (visiblePRs.length === 0 && watchedRepos.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-on-surface-variant space-y-2">
        <p className="text-lg font-semibold">No repositories watched</p>
        <p className="text-sm">
          {commandKeys === undefined ? (
            'Open the command palette and type '
          ) : (
            <>
              Press{' '}
              <kbd className="px-1 py-0.5 bg-surface-container text-xs font-mono">
                {commandKeys}
              </kbd>{' '}
              then type{' '}
            </>
          )}
          <kbd className="px-1 py-0.5 bg-surface-container text-xs font-mono">
            repos
          </kbd>{' '}
          to discover and watch your GitHub repositories.
        </p>
      </div>
    );
  }

  const groups = groupPRsByRepo(visiblePRs);
  const repoByKey = new Map(
    repos.map((repo) => [`${repo.owner}/${repo.repo}`, repo]),
  );
  let cursor = 0;

  return (
    <>
      <div className="space-y-0">
        {view !== 'grouped'
          ? sections.map((section) => {
              const lane = LANES.find(({ id }) => id === section.lane);
              return (
                <PRLaneSection
                  key={section.lane}
                  lane={section.lane}
                  label={lane?.label ?? ''}
                  prs={visiblePRs.slice(
                    section.startIndex,
                    section.startIndex + section.count,
                  )}
                  startIndex={section.startIndex}
                  selectedIndex={selectedIndex}
                  repoByKey={repoByKey}
                  onRecover={setRecoveryRepo}
                />
              );
            })
          : groups.map(([repoKey, repoPRs], idx) => {
              const collapsed = collapsedRepoKeys.includes(repoKey);
              const repo = repoByKey.get(repoKey) ?? null;
              const degradedRepo =
                repo !== null &&
                repo.location_status !== null &&
                repo.location_status !== undefined &&
                repo.location_status !== 'ok'
                  ? repo
                  : null;
              const start = cursor;
              cursor += collapsed ? 1 : repoPRs.length;
              return (
                <PRGroup
                  key={repoKey}
                  repoKey={repoKey}
                  prs={repoPRs}
                  collapsed={collapsed}
                  startIndex={start}
                  selectedIndex={selectedIndex}
                  accent={idx === 0}
                  degradedRepo={degradedRepo}
                  onRecover={setRecoveryRepo}
                />
              );
            })}
      </div>
      {recoveryRepo !== null && (
        <RepoRecoveryDialog
          repo={recoveryRepo}
          onClose={() => setRecoveryRepo(null)}
        />
      )}
    </>
  );
}
