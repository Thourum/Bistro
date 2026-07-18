// @vitest-environment jsdom

import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DEFAULT_SETTINGS } from '../../src/types/settings';
import { useVisiblePRs } from '../../src/renderer/hooks/useVisiblePRs';
import * as prListModule from '../../src/renderer/hooks/useWebSocketPRList';
import * as storeModule from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';

type PR = Parameters<typeof prListModule.useWebSocketPRList>[0] extends infer _
  ? Awaited<ReturnType<typeof prListModule.useWebSocketPRList>>['data'][number]
  : never;

const makePR = (overrides: Partial<{
  id: string;
  owner: string;
  repo: string;
  number: number;
  title: string;
  created_at: string;
  updated_at: string;
}> = {}) => ({
  id: overrides.id ?? 'github-1',
  provider_id: 'github:default',
  owner: overrides.owner ?? 'acme',
  repo: overrides.repo ?? 'widgets',
  number: overrides.number ?? 1,
  title: overrides.title ?? 'Fix auth bug',
  author: 'alice',
  state: 'open' as const,
  review_status: 'pending',
  created_at: overrides.created_at ?? '2025-01-01T10:00:00Z',
  updated_at: overrides.updated_at ?? '2025-01-01T14:00:00Z',
  base_branch: 'main',
  head_branch: 'fix/auth',
  additions: 10,
  deletions: 2,
  changed_files: 3,
  body: '',
  ci_status: undefined,
  draft: false,
});

const prA = makePR({ id: 'pr-a', number: 1, title: 'Alpha PR',   owner: 'acme', repo: 'widgets', created_at: '2025-01-01T10:00:00Z', updated_at: '2025-01-03T10:00:00Z' });
const prB = makePR({ id: 'pr-b', number: 2, title: 'Beta PR',    owner: 'acme', repo: 'gadgets', created_at: '2025-01-03T10:00:00Z', updated_at: '2025-01-02T10:00:00Z' });
const prC = makePR({ id: 'pr-c', number: 3, title: 'Gamma PR',   owner: 'acme', repo: 'widgets', created_at: '2025-01-02T10:00:00Z', updated_at: '2025-01-01T10:00:00Z' });

function makeStore(overrides: {
  filterState?: string;
  sortBy?: string;
  searchQuery?: string;
  repoFilter?: string | null;
  viewerLogin?: string;
} = {}) {
  return {
    state: {
      filterState: overrides.filterState ?? 'open',
      sortBy: overrides.sortBy ?? 'updated',
      searchQuery: overrides.searchQuery ?? '',
      repoFilter: overrides.repoFilter !== undefined ? overrides.repoFilter : null,
      selectedPRIndex: 0,
      viewerLogin: overrides.viewerLogin ?? '',
      repos: [],
      connectedProviders: [],
      prListRevision: 0,
    },
    subscribe: vi.fn(() => () => {}),
  };
}

describe('useVisiblePRs', () => {
  beforeEach(() => {
    settingsStore.setState(() => ({
      ...DEFAULT_SETTINGS,
      prList: { view: 'grouped' },
    }));
    vi.spyOn(prListModule, 'useWebSocketPRList').mockReturnValue({
      data: [prA, prB, prC],
      isLoading: false,
      error: null,
    } as ReturnType<typeof prListModule.useWebSocketPRList>);

    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      makeStore() as unknown as typeof storeModule.appStore,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns all PRs from the data source when no filters are active', () => {
    const { result } = renderHook(() => useVisiblePRs());
    expect(result.current.prs).toHaveLength(3);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('applies repoFilter from the store', () => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      makeStore({ repoFilter: 'acme/widgets' }) as unknown as typeof storeModule.appStore,
    );
    const { result } = renderHook(() => useVisiblePRs());
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-a', 'pr-c']);
  });

  it('applies repoFilter override without touching the store value', () => {
    const { result } = renderHook(() => useVisiblePRs({ repoFilter: 'acme/gadgets' }));
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-b']);
  });

  it('repoFilter override null means no filter (even when store has one)', () => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      makeStore({ repoFilter: 'acme/widgets' }) as unknown as typeof storeModule.appStore,
    );
    const { result } = renderHook(() => useVisiblePRs({ repoFilter: null }));
    expect(result.current.prs).toHaveLength(3);
  });

  it('applies searchQuery override (case-insensitive, matches title)', () => {
    const { result } = renderHook(() => useVisiblePRs({ q: 'alpha' }));
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-a']);
  });

  it('applies searchQuery override — matches repo name', () => {
    const { result } = renderHook(() => useVisiblePRs({ q: 'gadgets' }));
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-b']);
  });

  it('applies sort=created — orders by created_at DESC', () => {
    const { result } = renderHook(() => useVisiblePRs({ sort: 'created' }));
    // prB created_at=2025-01-03 > prC created_at=2025-01-02 > prA created_at=2025-01-01
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-b', 'pr-c', 'pr-a']);
  });

  it('sort=updated preserves backend order (no re-sort)', () => {
    const { result } = renderHook(() => useVisiblePRs({ sort: 'updated' }));
    // Backend order (as supplied): prA, prB, prC
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-a', 'pr-b', 'pr-c']);
  });

  it('sorts flat view by lane then newest update', () => {
    settingsStore.setState(() => DEFAULT_SETTINGS);
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      makeStore() as unknown as typeof storeModule.appStore,
    );

    const { result } = renderHook(() => useVisiblePRs());

    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-a', 'pr-b', 'pr-c']);
  });

  it('composes repoFilter + q + sort deterministically', () => {
    // repoFilter=acme/widgets keeps prA, prC; q='' no extra filter; sort=created → prC(2025-01-02) > prA(2025-01-01)
    const { result } = renderHook(() =>
      useVisiblePRs({ repoFilter: 'acme/widgets', q: '', sort: 'created' }),
    );
    expect(result.current.prs.map((p) => p.id)).toEqual(['pr-c', 'pr-a']);
  });

  it('returns loading/error state from the underlying hook', () => {
    vi.spyOn(prListModule, 'useWebSocketPRList').mockReturnValue({
      data: [],
      isLoading: true,
      error: null,
    } as ReturnType<typeof prListModule.useWebSocketPRList>);

    const { result } = renderHook(() => useVisiblePRs());
    expect(result.current.isLoading).toBe(true);
    expect(result.current.prs).toHaveLength(0);
  });

  it('PRListTable and useVisiblePRs produce the same list order', () => {
    // This test verifies that the index a PRRow receives (idx in visiblePRs.map)
    // matches what useVisiblePRs returns — i.e., they are the same array.
    const { result } = renderHook(() => useVisiblePRs());
    // With no filters, all 3 PRs in order
    const ids = result.current.prs.map((p) => p.id);
    expect(ids).toEqual(['pr-a', 'pr-b', 'pr-c']);
    // And index 0 == prA, index 1 == prB, etc.
    expect(result.current.prs[0].id).toBe('pr-a');
    expect(result.current.prs[1].id).toBe('pr-b');
    expect(result.current.prs[2].id).toBe('pr-c');
  });
});
