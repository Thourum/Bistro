import { useMemo } from 'react';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { PR } from '../types';
import { useStore } from './useStore';
import { useWebSocketPRList } from './useWebSocketPRList';

/**
 * Optional overrides that let callers feed filter/sort/q/repoFilter from route
 * search params (Slice 1+) instead of the store. Any omitted field falls back
 * to the current store value — a one-line change per field when migrating.
 */
export interface VisiblePRsOverride {
  filter?: UIState['filterState'];
  sort?: UIState['sortBy'];
  q?: string;
  repoFilter?: string | null;
}

export interface UseVisiblePRsResult {
  /** Canonical ordered + filtered list — the single source of truth for all
   *  consumers (table rows, keyboard cursor, Enter, Shift+G, command palette). */
  prs: PR[];
  isLoading: boolean;
  error: Error | null;
}

/**
 * Single canonical source for the visible PR list.
 *
 * Composition order: data-source → repoFilter → searchQuery → sort.
 * All consumers MUST derive their row index from this list so that
 * `selectedPRIndex` always addresses the same row everywhere.
 *
 * @param overrides  Optional route-param values (Slice 1+). Omit in pre-routing
 *                   code — the hook reads the store automatically.
 *
 * @example
 *   // Current (pre-routing) — reads everything from the store:
 *   const { prs, isLoading, error } = useVisiblePRs();
 *
 *   // Slice 1+ (route search params override):
 *   const { filter, sort, q } = useSearch({ from: '/prs' });
 *   const { prs } = useVisiblePRs({ filter, sort, q });
 *
 * Caller helper: `prs[clamp(selectedPRIndex, 0, prs.length - 1)]` resolves the
 * PR under the keyboard cursor without any separate list derivation.
 *
 * TODO(slice-2): CommandPalette still builds its own list — adopt useVisiblePRs there.
 */
export function useVisiblePRs(
  overrides?: VisiblePRsOverride,
): UseVisiblePRsResult {
  const storeFilter = useStore(appStore, (s: UIState) => s.filterState);
  const storeSort = useStore(appStore, (s: UIState) => s.sortBy);
  const storeQ = useStore(appStore, (s: UIState) => s.searchQuery);
  const storeRepoFilter = useStore(appStore, (s: UIState) => s.repoFilter);

  const filter = overrides?.filter ?? storeFilter;
  const sort = overrides?.sort ?? storeSort;
  const q = overrides?.q ?? storeQ;
  // Explicit `null` in overrides means "no filter"; undefined means "use store".
  const repoFilter =
    overrides !== undefined && 'repoFilter' in overrides
      ? (overrides.repoFilter ?? null)
      : storeRepoFilter;

  const { data: rawPRs = [], isLoading, error } = useWebSocketPRList(filter);

  const prs = useMemo(() => {
    let result: PR[] = rawPRs;

    // 1. repoFilter: restrict to a single owner/repo slug.
    if (repoFilter) {
      result = result.filter((pr) => `${pr.owner}/${pr.repo}` === repoFilter);
    }

    // 2. searchQuery: case-insensitive substring match on title/owner/repo.
    if (q) {
      const lower = q.toLowerCase();
      result = result.filter(
        (pr) =>
          pr.title.toLowerCase().includes(lower) ||
          pr.repo.toLowerCase().includes(lower) ||
          pr.owner.toLowerCase().includes(lower),
      );
    }

    // 3. sort: backend returns updated_at DESC (matches 'updated'); re-sort
    //    only when the user prefers created_at order.
    if (sort === 'created') {
      result = [...result].sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      );
    }

    return result;
  }, [rawPRs, repoFilter, q, sort]);

  return { prs, isLoading, error: error as Error | null };
}
