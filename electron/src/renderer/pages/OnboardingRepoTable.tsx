import { type ReactElement, useEffect, useRef } from 'react';
import { StatusSquare } from '../components/StatusSquare';
import { ActionKeyHint } from '../components/ui/action-key-hint';
import { Button } from '../components/ui/button';
import { useStore } from '../hooks/useStore';
import {
  consumeOnboardingIntent,
  updateOnboardingSelection,
} from '../services/onboardingSync';
import { appStore } from '../store';
import type { OnboardingSyncState } from '../store/ui';
import type { RepoInfo } from '../types';
import {
  groupOnboardingRepos,
  type OnboardingRepoGroup,
  type RepoGroupKey,
  selectAllOnboardingRepos,
  setOnboardingRepoFilter,
  setOnboardingRepoWatch,
} from '../utils/onboardingRepos';

const LABEL = 'mono-text text-[10px] uppercase tracking-widest';
const HEALTHY_LOCATION = 'ok';
const RECENT_LABEL = 'Reviewed before, active in the last 30 days';

interface RowContext {
  readonly state: OnboardingSyncState;
  readonly counts: ReadonlyMap<string, number>;
  readonly hasRequests: boolean;
}

function queueStatus(state: OnboardingSyncState): string {
  if (state.queuePending) return 'Reading…';
  const queue = state.selection.queue;
  if (queue === null) return '';
  if (queue.requested.length === 0) return '0 waiting on you';
  const prs = queue.requested.reduce((sum, request) => sum + request.count, 0);
  return `${prs} PRs need you // ${queue.requested.length} repos`;
}

function isSelected(state: OnboardingSyncState, repo: RepoInfo): boolean {
  return state.selection.selected[repo.id] ?? repo.watched;
}

function setCursor(cursor: number): void {
  updateOnboardingSelection((selection) => ({ ...selection, cursor }));
}

function CloneCell({
  repo,
  isScanning,
}: {
  readonly repo: RepoInfo;
  readonly isScanning: boolean;
}): ReactElement {
  const isDegraded =
    repo.location_status !== null &&
    repo.location_status !== undefined &&
    repo.location_status !== HEALTHY_LOCATION;
  if (isDegraded) {
    const reason = repo.location_error ?? 'Repository location unavailable.';
    return (
      <span
        className="flex items-center gap-2 min-w-0 text-error"
        title={reason}
      >
        <StatusSquare
          status="failure"
          size="w-2 h-2"
          label="Location unavailable"
        />
        <span className="truncate">
          {repo.local_path || 'Location unavailable'}
        </span>
      </span>
    );
  }
  if (repo.local_path)
    return (
      <span
        className="block truncate text-on-surface-variant"
        title={repo.local_path}
      >
        {repo.local_path}
      </span>
    );
  if (isScanning)
    return (
      <span className="flex items-center gap-2 text-outline">
        <StatusSquare status="running" size="w-2 h-2" label="Matching" />
        Matching…
      </span>
    );
  return <span className="text-on-surface-variant">Clones on first open</span>;
}

function RepoRow({
  repo,
  index,
  context,
}: {
  readonly repo: RepoInfo;
  readonly index: number;
  readonly context: RowContext;
}): ReactElement {
  const { state, counts, hasRequests } = context;
  const isFocused = state.selection.cursor === index;
  const isOn = isSelected(state, repo);
  const needsYou = counts.get(repo.id) ?? 0;
  const rowRef = useRef<HTMLTableRowElement>(null);
  useEffect(() => {
    if (isFocused) rowRef.current?.scrollIntoView({ block: 'nearest' });
  }, [isFocused]);
  return (
    <tr
      ref={rowRef}
      aria-selected={isFocused}
      className={`border-b border-outline-variant/20 ${isFocused ? 'bg-surface-container-low' : ''}`}
      onClick={() => setCursor(index)}
    >
      <td
        className={`border-l-2 py-2 pl-4 pr-2 ${isFocused ? 'border-primary' : 'border-transparent'}`}
      >
        <button
          type="button"
          aria-pressed={isOn}
          aria-label={`Sync ${repo.owner}/${repo.repo}`}
          disabled={state.selection.sealed}
          className={`flex items-center gap-2 ${LABEL} font-bold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary ${isOn ? 'text-primary' : 'text-outline'}`}
          onClick={() =>
            updateOnboardingSelection((selection) =>
              setOnboardingRepoWatch(selection, {
                repoId: repo.id,
                watched: !isOn,
              }),
            )
          }
        >
          <span
            aria-hidden="true"
            className={`w-2 h-2 border ${isOn ? 'bg-primary border-primary' : 'border-outline'}`}
          />
          {isOn ? 'On' : 'Off'}
        </button>
      </td>
      <td
        className={`px-2 py-2 truncate ${isOn ? 'text-on-surface' : 'text-on-surface-variant'} ${isFocused ? 'font-bold' : ''}`}
      >
        {repo.owner}/{repo.repo}
      </td>
      {hasRequests && (
        <td
          className={`px-2 py-2 text-right ${needsYou > 0 ? 'text-primary font-bold' : 'text-outline-variant'}`}
        >
          {needsYou}
        </td>
      )}
      <td className="px-2 py-2 pr-4 max-w-0">
        <CloneCell
          repo={repo}
          isScanning={state.folder.status === 'scanning'}
        />
      </td>
    </tr>
  );
}

function GroupHeading({
  label,
  isAccent,
  columns,
}: {
  readonly label: string;
  readonly isAccent: boolean;
  readonly columns: number;
}): ReactElement {
  return (
    <tr className="border-b border-outline-variant/30 bg-surface-container-lowest">
      <th
        scope="colgroup"
        colSpan={columns}
        className={`px-4 py-2 text-left font-normal ${LABEL} ${isAccent ? 'text-primary' : 'text-outline'}`}
      >
        {label}
      </th>
    </tr>
  );
}

/** Headings per group: requested/nothing-waiting split only when the queue has requests. */
function groupLabels(
  groups: readonly OnboardingRepoGroup[],
  totals: ReadonlyMap<RepoGroupKey, number>,
  hasRequests: boolean,
): ReadonlyMap<RepoGroupKey, readonly string[]> {
  const total = (key: RepoGroupKey): number => totals.get(key) ?? 0;
  const shown = new Set(groups.map((group) => group.key));
  const recent = `${RECENT_LABEL} (${total('recent')})`;
  const other = shown.has('recent') ? [`Other repos (${total('other')})`] : [];
  if (!hasRequests)
    return new Map([
      ['recent', [recent]],
      ['other', other],
    ]);
  const waiting = `Nothing waiting (${total('recent') + total('other')})`;
  return new Map([
    ['requested', [`Waiting on you (${total('requested')})`]],
    ['recent', [waiting, recent]],
    ['other', shown.has('recent') ? other : [waiting]],
  ]);
}

function RepoRows({
  state,
}: {
  readonly state: OnboardingSyncState;
}): ReactElement {
  const { selection } = state;
  const hasRequests = (selection.queue?.requested.length ?? 0) > 0;
  const columns = hasRequests ? 4 : 3;
  const groups = groupOnboardingRepos(selection);
  const totals = new Map(
    groupOnboardingRepos({ ...selection, filter: '' }).map((group) => [
      group.key,
      group.total,
    ]),
  );
  const labels = groupLabels(groups, totals, hasRequests);
  const context: RowContext = {
    state,
    hasRequests,
    counts: new Map(
      selection.queue?.requested.map((request) => [
        request.repo_id,
        request.count,
      ]),
    ),
  };
  const offsets = groups.map((_, groupIndex) =>
    groups
      .slice(0, groupIndex)
      .reduce((sum, group) => sum + group.rows.length, 0),
  );
  return (
    <table className="w-full table-fixed border-collapse mono-text text-xs">
      <thead className="sticky top-0 bg-surface-container-low">
        <tr
          className={`border-b border-outline-variant/30 text-outline ${LABEL}`}
        >
          <th scope="col" className="w-24 py-2 pl-4 pr-2 text-left font-normal">
            Sync
          </th>
          <th scope="col" className="px-2 py-2 text-left font-normal">
            Repo
          </th>
          {hasRequests && (
            <th scope="col" className="w-24 px-2 py-2 text-right font-normal">
              Needs you
            </th>
          )}
          <th
            scope="col"
            className="w-2/5 px-2 py-2 pr-4 text-left font-normal"
          >
            Clone
          </th>
        </tr>
      </thead>
      <tbody>
        {groups.map((group, groupIndex) => [
          ...(labels.get(group.key) ?? []).map((label) => (
            <GroupHeading
              key={`${group.key}-${label}`}
              label={label}
              isAccent={group.key === 'requested'}
              columns={columns}
            />
          )),
          ...group.rows.map((repo, rowIndex) => (
            <RepoRow
              key={repo.id}
              repo={repo}
              index={(offsets[groupIndex] ?? 0) + rowIndex}
              context={context}
            />
          )),
        ])}
      </tbody>
    </table>
  );
}

function RepoToolbar({
  state,
}: {
  readonly state: OnboardingSyncState;
}): ReactElement {
  const { selection } = state;
  const filterRef = useRef<HTMLInputElement>(null);
  const intent = useStore(appStore, (store) => store.onboardingIntent);
  useEffect(() => {
    if (intent !== 'focusRepoFilter') return;
    consumeOnboardingIntent(intent);
    filterRef.current?.focus();
  }, [intent]);
  const selectedCount = selection.repos.filter((repo) =>
    isSelected(state, repo),
  ).length;
  const selectAll = (watched: boolean): void =>
    updateOnboardingSelection((current) =>
      selectAllOnboardingRepos(current, { watched }),
    );
  return (
    <div className="flex flex-wrap items-center gap-4 px-4 py-3 border-b border-outline-variant/30">
      <label className="flex items-center gap-2 grow max-w-xs bg-surface-container-low px-3 py-2">
        <span
          aria-hidden="true"
          className="material-symbols-outlined text-base text-outline"
        >
          search
        </span>
        <input
          ref={filterRef}
          data-onboarding-repo-filter
          aria-label="Filter repositories"
          placeholder="Filter repos…"
          value={selection.filter}
          className="w-full bg-transparent text-sm text-on-surface placeholder:text-outline focus:outline-none"
          onChange={(event) => {
            const filter = event.currentTarget.value;
            updateOnboardingSelection((current) =>
              setOnboardingRepoFilter(current, filter),
            );
          }}
        />
      </label>
      <span className="mono-text text-[11px] text-on-surface-variant">
        <span className="text-primary font-bold">
          {selectedCount} of {selection.repos.length}
        </span>{' '}
        selected
      </span>
      <span className="ml-auto flex items-center gap-2">
        <Button
          size="sm"
          variant="ghost"
          disabled={selection.sealed}
          onClick={() => selectAll(true)}
        >
          All <ActionKeyHint actionId="onboarding.repoAll" />
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={selection.sealed}
          onClick={() => selectAll(false)}
        >
          None <ActionKeyHint actionId="onboarding.repoNone" />
        </Button>
      </span>
    </div>
  );
}

function PanelBody({
  state,
}: {
  readonly state: OnboardingSyncState;
}): ReactElement {
  const { selection } = state;
  const isLoading =
    selection.repos.length === 0 &&
    state.repoError === null &&
    (state.initialReposPending || state.queuePending);
  if (isLoading)
    return (
      <div className="flex grow flex-col items-center justify-center gap-3 p-8 text-center">
        <span
          className={`flex items-center gap-2 font-bold text-on-surface ${LABEL}`}
        >
          <StatusSquare status="running" size="w-2 h-2" label="Reading" />
          Reading your review requests…
        </span>
        <p className="max-w-sm text-sm text-on-surface-variant">
          Repos with PRs waiting on you will be switched on. You can change any
          of them before you continue.
        </p>
      </div>
    );
  const hasNoMatches =
    selection.filter !== '' &&
    groupOnboardingRepos(selection).length === 0 &&
    selection.repos.length > 0;
  return (
    <>
      {selection.repos.length > 0 && <RepoToolbar state={state} />}
      <div className="min-h-0 grow overflow-auto">
        {hasNoMatches ? (
          <p className="p-6 text-sm text-on-surface-variant">
            No repositories match this filter.
          </p>
        ) : (
          <RepoRows state={state} />
        )}
      </div>
    </>
  );
}

/** Step-1 repository chooser; keyboard actions reach it only through useAppHotkeys. */
export function OnboardingRepoTable({
  state,
}: {
  readonly state: OnboardingSyncState;
}): ReactElement {
  const queue = state.selection.queue;
  const isQueueEmpty =
    !state.queuePending && queue !== null && queue.requested.length === 0;
  return (
    <section
      aria-label="Repositories"
      className="flex min-h-96 flex-col bg-surface-container-lowest border border-outline-variant/30"
    >
      <header
        className={`flex items-center justify-between px-4 py-3 bg-surface-container-low border-b border-outline-variant/30 text-outline ${LABEL}`}
      >
        <h2 className="font-normal">Repositories</h2>
        <span>{queueStatus(state)}</span>
      </header>
      {state.queueError !== null && (
        <p
          role="alert"
          className="px-4 py-3 text-sm text-error border-b border-outline-variant/30"
        >
          Couldn't read your review requests: {state.queueError}.
        </p>
      )}
      {state.repoError !== null && (
        <p
          role="alert"
          className="px-4 py-3 text-sm text-error border-b border-outline-variant/30"
        >
          Couldn't load your repositories: {state.repoError}.
        </p>
      )}
      {isQueueEmpty && (
        <div className="px-6 py-5 border-b border-outline-variant/30">
          <h3 className="text-xl font-bold text-on-surface mb-2">
            Nothing is waiting on you right now
          </h3>
          <p className="max-w-2xl text-sm text-on-surface-variant">
            When someone requests your review, it shows up in your inbox. You
            can also turn on repos you review often.
          </p>
        </div>
      )}
      <PanelBody state={state} />
      <footer className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 border-t border-outline-variant/30 mono-text text-[10px] text-outline">
        <span className="flex items-center gap-1 uppercase tracking-widest">
          <ActionKeyHint actionId="onboarding.repoDown" />
          <ActionKeyHint actionId="onboarding.repoUp" /> Move
        </span>
        <span className="flex items-center gap-1 uppercase tracking-widest">
          <ActionKeyHint actionId="onboarding.repoToggle" /> Toggle
        </span>
        <span className="flex items-center gap-1 uppercase tracking-widest">
          <ActionKeyHint actionId="onboarding.repoFilter" /> Filter
        </span>
        <span className="ml-auto">
          PRs that involve you always reach your inbox. ON also syncs every open
          PR in the repo. Change this any time in Settings → Repos.
        </span>
      </footer>
    </section>
  );
}
