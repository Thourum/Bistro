import { describe, expect, it } from 'vitest';
import type { RepoInfo } from '../types';
import {
  buildOnboardingWatchChanges,
  countOnboardingFolderMatches,
  createOnboardingSelection,
  groupOnboardingRepos,
  mergeOnboardingQueue,
  mergeOnboardingRepos,
  moveOnboardingCursor,
  sealOnboardingSelection,
  selectAllOnboardingRepos,
  setOnboardingRepoFilter,
  setOnboardingRepoWatch,
  toggleFocusedOnboardingRepo,
} from './onboardingRepos';

const repo = (name: string, watched = false): RepoInfo => ({
  id: `github:acme/${name}`,
  provider: 'github',
  owner: 'acme',
  repo: name,
  watched,
});
const queue = {
  requested: [{ repo_id: 'github:acme/api', count: 3 }],
  recently_reviewed: ['github:acme/web'],
};

describe('onboarding repository choices', () => {
  it('counts only table paths within the selected folder boundary', () => {
    const repos = [
      { ...repo('api'), local_path: '/dev/api' },
      { ...repo('web'), local_path: '/developer/web' },
      { ...repo('other'), local_path: null },
    ];
    expect(countOnboardingFolderMatches(repos, '/dev/')).toBe(1);
    expect(countOnboardingFolderMatches(repos, '/')).toBe(2);
    expect(countOnboardingFolderMatches(repos, '')).toBe(0);
    expect(
      countOnboardingFolderMatches(
        [{ ...repo('api'), local_path: 'C:\\dev\\api' }],
        'C:\\dev',
      ),
    ).toBe(1);
  });

  it('defaults requests on without overwriting explicit choices or existing watch flags', () => {
    let state = mergeOnboardingRepos(createOnboardingSelection(), [
      repo('api'),
      repo('web', true),
      repo('other'),
    ]);
    state = setOnboardingRepoWatch(state, {
      repoId: 'github:acme/api',
      watched: false,
    });
    const before = state;
    state = mergeOnboardingQueue(state, queue);
    expect(state.selected).toEqual({
      'github:acme/api': false,
      'github:acme/web': true,
      'github:acme/other': false,
    });
    expect(before.queue).toBeNull();
    expect(buildOnboardingWatchChanges(state)).toEqual([]);
    const fresh = mergeOnboardingQueue(
      mergeOnboardingRepos(createOnboardingSelection(), [repo('api')]),
      queue,
    );
    expect(buildOnboardingWatchChanges(fresh)).toEqual([
      { repo_id: 'github:acme/api', watched: true },
    ]);
  });

  it('handles queue-before-cache, grouping, filtering and cursor clamps', () => {
    let state = mergeOnboardingQueue(createOnboardingSelection(), queue);
    state = mergeOnboardingRepos(state, [
      repo('other'),
      repo('web'),
      repo('api'),
    ]);
    expect(
      groupOnboardingRepos(state).map((g) => [
        g.key,
        g.rows.map((r) => r.repo),
      ]),
    ).toEqual([
      ['requested', ['api']],
      ['recent', ['web']],
      ['other', ['other']],
    ]);
    state = setOnboardingRepoFilter(state, 'ACME/WEB');
    expect(
      groupOnboardingRepos(state)
        .flatMap((g) => g.rows)
        .map((r) => r.repo),
    ).toEqual(['web']);
    state = moveOnboardingCursor(state, 1);
    expect(state.cursor).toBe(0);
    state = toggleFocusedOnboardingRepo(state);
    expect(state.selected['github:acme/web']).toBe(true);
    state = setOnboardingRepoFilter(state, 'no match');
    expect(toggleFocusedOnboardingRepo(state)).toEqual(state);
    expect(state.cursor).toBe(0);
  });

  it('All and None cover the complete table, including filtered-out repos and late defaults', () => {
    let state = mergeOnboardingRepos(createOnboardingSelection(), [
      repo('api'),
      repo('web'),
    ]);
    state = setOnboardingRepoFilter(state, 'api');
    state = selectAllOnboardingRepos(state, { watched: true });
    expect(buildOnboardingWatchChanges(state)).toHaveLength(2);
    state = selectAllOnboardingRepos(state, { watched: false });
    state = mergeOnboardingQueue(state, queue);
    expect(Object.values(state.selected)).toEqual([false, false]);
    expect(buildOnboardingWatchChanges(state)).toEqual([]);
    state = mergeOnboardingRepos(state, [repo('late', true)]);
    expect(state.selected['github:acme/late']).toBe(true);
  });

  it('preserves clone metadata and stops adding automatic choices after Continue', () => {
    const initial = mergeOnboardingRepos(createOnboardingSelection(), [
      repo('api'),
    ]);
    const sealed = sealOnboardingSelection(initial);
    const refreshed = mergeOnboardingRepos(
      mergeOnboardingQueue(sealed, queue),
      [
        { ...repo('api'), source: 'local', local_path: '/projects/api' },
        repo('web'),
      ],
    );
    expect(refreshed.selected['github:acme/api']).toBe(false);
    expect(refreshed.repos[0]?.local_path).toBe('/projects/api');
    expect(buildOnboardingWatchChanges(refreshed)).toEqual([]);
    expect(
      setOnboardingRepoWatch(refreshed, {
        repoId: 'github:acme/api',
        watched: true,
      }),
    ).toBe(refreshed);
    expect(selectAllOnboardingRepos(refreshed, { watched: true })).toBe(
      refreshed,
    );
  });

  it('orders requests by count then name, excludes them from recent and preserves unfiltered totals', () => {
    let state = mergeOnboardingRepos(createOnboardingSelection(), [
      repo('z'),
      repo('a'),
      repo('web'),
      repo('other'),
    ]);
    state = mergeOnboardingQueue(state, {
      requested: [
        { repo_id: 'github:acme/z', count: 2 },
        { repo_id: 'github:acme/a', count: 2 },
      ],
      recently_reviewed: [
        'github:acme/z',
        'github:acme/web',
        'github:acme/other',
      ],
    });
    expect(
      groupOnboardingRepos(state).map((g) => g.rows.map((r) => r.repo)),
    ).toEqual([
      ['a', 'z'],
      ['web', 'other'],
    ]);
    state = moveOnboardingCursor(
      moveOnboardingCursor(moveOnboardingCursor(state, 1), 1),
      1,
    );
    expect(state.cursor).toBe(3);
    state = setOnboardingRepoFilter(state, 'a');
    const groups = groupOnboardingRepos(state);
    expect(groups[0]?.total).toBe(2);
    state = setOnboardingRepoFilter(state, '/web');
    expect(state.cursor).toBe(0);
    expect(groupOnboardingRepos(state)).toMatchObject([
      { key: 'recent', total: 2 },
    ]);
  });

  it('deduplicates updates and compares choices against the latest watched flags', () => {
    const before = mergeOnboardingRepos(createOnboardingSelection(), [
      repo('api'),
    ]);
    const requested = mergeOnboardingQueue(before, queue);
    const refreshed = mergeOnboardingRepos(requested, [
      repo('api', true),
      repo('api', true),
    ]);
    expect(refreshed.repos).toHaveLength(1);
    expect(before.repos[0]?.watched).toBe(false);
    expect(buildOnboardingWatchChanges(refreshed)).toEqual([]);
    const newlyWatched = mergeOnboardingRepos(before, [repo('api', true)]);
    expect(newlyWatched.selected['github:acme/api']).toBe(true);
    const explicitOff = setOnboardingRepoWatch(before, {
      repoId: 'github:acme/api',
      watched: false,
    });
    expect(
      mergeOnboardingRepos(explicitOff, [repo('api', true)]).selected[
        'github:acme/api'
      ],
    ).toBe(false);
  });
});
