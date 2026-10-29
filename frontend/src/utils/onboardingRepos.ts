import type { RepoInfo, ReviewQueue } from '../types';

export interface OnboardingSelection {
  readonly repos: readonly RepoInfo[];
  readonly queue: ReviewQueue | null;
  readonly selected: Readonly<Record<string, boolean>>;
  readonly touched: readonly string[];
  readonly filter: string;
  readonly cursor: number;
  readonly sealed: boolean;
}
export type RepoGroupKey = 'requested' | 'recent' | 'other';
export interface OnboardingRepoGroup {
  readonly key: RepoGroupKey;
  readonly rows: readonly RepoInfo[];
  readonly total: number;
}
export interface WatchChange {
  readonly repo_id: string;
  readonly watched: boolean;
}

export function createOnboardingSelection(): OnboardingSelection {
  return {
    repos: [],
    queue: null,
    selected: {},
    touched: [],
    filter: '',
    cursor: 0,
    sealed: false,
  };
}

function compareRepoNames(left: RepoInfo, right: RepoInfo): number {
  return `${left.owner}/${left.repo}`.localeCompare(
    `${right.owner}/${right.repo}`,
  );
}

export function groupOnboardingRepos(
  state: OnboardingSelection,
): readonly OnboardingRepoGroup[] {
  const counts = new Map(
    state.queue?.requested.map((request) => [request.repo_id, request.count]),
  );
  const recentOrder = new Map(
    state.queue?.recently_reviewed.map((id, index) => [id, index]),
  );
  const requested = state.repos
    .filter((repo) => counts.has(repo.id))
    .sort(
      (left, right) =>
        (counts.get(right.id) ?? 0) - (counts.get(left.id) ?? 0) ||
        compareRepoNames(left, right),
    );
  const recent = state.repos
    .filter((repo) => !counts.has(repo.id) && recentOrder.has(repo.id))
    .sort(
      (left, right) =>
        (recentOrder.get(left.id) ?? 0) - (recentOrder.get(right.id) ?? 0),
    );
  const other = state.repos
    .filter((repo) => !counts.has(repo.id) && !recentOrder.has(repo.id))
    .sort(compareRepoNames);
  const groups: readonly OnboardingRepoGroup[] = [
    { key: 'requested', rows: requested, total: requested.length },
    { key: 'recent', rows: recent, total: recent.length },
    { key: 'other', rows: other, total: other.length },
  ];
  const filter = state.filter.toLowerCase();
  return groups
    .map((group) => ({
      ...group,
      rows: group.rows.filter((repo) =>
        `${repo.owner}/${repo.repo}`.toLowerCase().includes(filter),
      ),
    }))
    .filter((group) => group.rows.length > 0);
}

function clampOnboardingCursor(
  state: OnboardingSelection,
): OnboardingSelection {
  const count = groupOnboardingRepos(state).reduce(
    (total, group) => total + group.rows.length,
    0,
  );
  return { ...state, cursor: Math.max(0, Math.min(state.cursor, count - 1)) };
}

export function mergeOnboardingRepos(
  state: OnboardingSelection,
  repos: readonly RepoInfo[],
): OnboardingSelection {
  const byId = new Map(state.repos.map((repo) => [repo.id, repo]));
  for (const repo of repos)
    byId.set(repo.id, { ...byId.get(repo.id), ...repo });
  const merged = [...byId.values()];
  const requestedIds = new Set(
    state.queue?.requested.map((request) => request.repo_id),
  );
  const selected = state.sealed
    ? state.selected
    : Object.fromEntries(
        merged.map((repo) => [
          repo.id,
          !state.touched.includes(repo.id) && repo.watched
            ? true
            : (state.selected[repo.id] ??
              (requestedIds.has(repo.id) ? true : repo.watched)),
        ]),
      );
  return clampOnboardingCursor({ ...state, repos: merged, selected });
}

export function mergeOnboardingQueue(
  state: OnboardingSelection,
  queue: ReviewQueue,
): OnboardingSelection {
  const requestedIds = new Set(
    queue.requested.map((request) => request.repo_id),
  );
  const selected = state.sealed
    ? state.selected
    : Object.fromEntries(
        state.repos.map((repository) => [
          repository.id,
          !state.touched.includes(repository.id) &&
          requestedIds.has(repository.id)
            ? true
            : (state.selected[repository.id] ?? repository.watched),
        ]),
      );
  return clampOnboardingCursor({ ...state, queue, selected });
}

export function setOnboardingRepoWatch(
  state: OnboardingSelection,
  change: { repoId: string; watched: boolean },
): OnboardingSelection {
  if (
    state.sealed ||
    !state.repos.some((repository) => repository.id === change.repoId)
  )
    return state;
  return {
    ...state,
    selected: { ...state.selected, [change.repoId]: change.watched },
    touched: state.touched.includes(change.repoId)
      ? state.touched
      : [...state.touched, change.repoId],
  };
}

export function toggleFocusedOnboardingRepo(
  state: OnboardingSelection,
): OnboardingSelection {
  const repo = groupOnboardingRepos(state).flatMap((group) => group.rows)[
    state.cursor
  ];
  if (repo === undefined) return state;
  return setOnboardingRepoWatch(state, {
    repoId: repo.id,
    watched: !(state.selected[repo.id] ?? repo.watched),
  });
}

export function selectAllOnboardingRepos(
  state: OnboardingSelection,
  options: { watched: boolean },
): OnboardingSelection {
  if (state.sealed) return state;
  return {
    ...state,
    selected: Object.fromEntries(
      state.repos.map((repository) => [repository.id, options.watched]),
    ),
    touched: [
      ...new Set([
        ...state.touched,
        ...state.repos.map((repository) => repository.id),
      ]),
    ],
  };
}

export function setOnboardingRepoFilter(
  state: OnboardingSelection,
  filter: string,
): OnboardingSelection {
  return clampOnboardingCursor({ ...state, filter });
}

export function moveOnboardingCursor(
  state: OnboardingSelection,
  delta: 1 | -1,
): OnboardingSelection {
  return clampOnboardingCursor({ ...state, cursor: state.cursor + delta });
}

export function buildOnboardingWatchChanges(
  state: OnboardingSelection,
): readonly WatchChange[] {
  return state.repos.flatMap((repository) => {
    const watched = state.selected[repository.id] ?? repository.watched;
    return watched === repository.watched
      ? []
      : [{ repo_id: repository.id, watched }];
  });
}

export function sealOnboardingSelection(
  state: OnboardingSelection,
): OnboardingSelection {
  return state.sealed ? state : { ...state, sealed: true };
}

/** Counts table repositories under the canonical folder, not all local locations. */
export function countOnboardingFolderMatches(
  repos: readonly RepoInfo[],
  folder: string,
): number {
  const normalize = (path: string): string =>
    path.replaceAll('\\', '/').replace(/\/+$/, '');
  const root = normalize(folder);
  if (folder.trim() === '') return 0;
  return repos.filter((repo) => {
    if (!repo.local_path) return false;
    const path = normalize(repo.local_path);
    return path === root || path.startsWith(`${root}/`);
  }).length;
}
