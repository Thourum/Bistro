// usePrsSearchStoreBridge
//
// Bidirectional sync between /prs/ route search params and UIState fields.
//
// Direction 1 (route → store): reacts to route-search changes (deep-link on
//   mount AND later route changes, e.g. browser back/forward). Route wins.
// Direction 2 (store → route): after hydration, any store write (FilterMenu,
//   keybindings) is reflected in the URL via navigate({replace:true}).
//
// Echo-loop prevention:
//   - hydratedRef: direction 2 skips until direction 1 has run at least once.
//   - applyingRouteToStoreRef: direction 2 skips while direction 1 is writing
//     the store, so a route→store write cannot bounce back as store→route.
//   - Equality guards in both directions: no-op when values already match.

import { useNavigate } from '@tanstack/react-router';
import { useEffect, useRef } from 'react';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { useStore } from './useStore';

export interface PrsSearch {
  filter: UIState['filterState'];
  sort: UIState['sortBy'];
  q: string;
}

export function usePrsSearchStoreBridge(search: PrsSearch): void {
  const navigate = useNavigate({ from: '/prs/' });
  const hydratedRef = useRef(false);
  const applyingRouteToStoreRef = useRef(false);

  const filterState = useStore(appStore, (s: UIState) => s.filterState);
  const sortBy = useStore(appStore, (s: UIState) => s.sortBy);
  const searchQuery = useStore(appStore, (s: UIState) => s.searchQuery);

  // Direction 1: route → store (on mount and on any later route-search change).
  useEffect(() => {
    const differs =
      appStore.state.filterState !== search.filter ||
      appStore.state.sortBy !== search.sort ||
      appStore.state.searchQuery !== search.q;

    if (differs) {
      applyingRouteToStoreRef.current = true;
      appStore.setState((s) => ({
        ...s,
        filterState: search.filter,
        sortBy: search.sort,
        searchQuery: search.q,
      }));
      // Release the guard after this flush so direction 2 doesn't echo it back.
      queueMicrotask(() => {
        applyingRouteToStoreRef.current = false;
        hydratedRef.current = true;
      });
    } else {
      hydratedRef.current = true;
    }
  }, [search.filter, search.sort, search.q]);

  // Direction 2: store → route (after hydration; skipped while applying route→store).
  useEffect(() => {
    if (!hydratedRef.current) return;
    if (applyingRouteToStoreRef.current) return;

    if (
      filterState === search.filter &&
      sortBy === search.sort &&
      searchQuery === search.q
    ) {
      return;
    }

    void navigate({
      replace: true,
      search: { filter: filterState, sort: sortBy, q: searchQuery },
    });
  }, [
    filterState,
    sortBy,
    searchQuery,
    navigate,
    search.filter,
    search.sort,
    search.q,
  ]);
}
