import React from 'react';
import { FooterCenterSlot } from '../components/FooterSlot';
import { PRListTable } from '../components/PRListTable';
import { useStore } from '../hooks/useStore';
import { useVisiblePRs } from '../hooks/useVisiblePRs';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { countTriageLanes } from '../utils/triage';

const FILTER_TITLES: Record<UIState['filterState'], string> = {
  needs_action: 'Needs Your Action',
  open: 'Open Pull Requests',
  draft: 'Draft Pull Requests',
  all: 'All Pull Requests',
};

export interface PRListRouteSearch {
  filter: UIState['filterState'];
  sort: UIState['sortBy'];
  q: string;
}

/**
 * PR inbox — matches `pr_inbox_minimal_light/code.html`.
 *
 * The legend shortcuts are rendered by the global `AppFooter` (binding-derived),
 * and the filter/sort status is projected into the footer's center region via
 * `FooterCenterSlot`.
 *
 * @param routeSearch  When rendered by the /prs/ route, the route-authoritative
 *                     filter/sort/q values are passed here. When omitted (tests,
 *                     legacy callers), the component falls back to the store.
 */
export function PRListPage({
  routeSearch,
}: {
  routeSearch?: PRListRouteSearch;
}): React.ReactElement {
  const storeFilter = useStore(appStore, (s: UIState) => s.filterState);
  const storeSortBy = useStore(appStore, (s: UIState) => s.sortBy);
  const storeSearchQuery = useStore(appStore, (s: UIState) => s.searchQuery);

  const filterState = routeSearch?.filter ?? storeFilter;
  const sortBy = routeSearch?.sort ?? storeSortBy;
  const q = routeSearch?.q ?? storeSearchQuery;

  const username = useStore(
    appStore,
    (s: UIState) => (s.connectedProviders ?? [])[0]?.username ?? '',
  );

  const { prs, isLoading, error } = useVisiblePRs({
    filter: filterState,
    sort: sortBy,
    q,
  });

  // Sync visible PR ids to store so keybinding actions (list.open, list.bottom)
  // can navigate by id without a React hook context.
  // useLayoutEffect ensures ids are committed BEFORE any synchronous user input
  // (e.g. filter-then-immediate-Enter) reads the list.
  React.useLayoutEffect(() => {
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: prs.map((p) => p.id),
    }));
  }, [prs]);

  // Counters always reflect the full open lane, independent of the filter.
  const { data: openPRs = [] } = useWebSocketPRList('open');
  const triage = countTriageLanes(openPRs, username);

  return (
    <main
      data-testid="pr-list-page"
      className="flex-1 overflow-y-auto bg-surface-container-lowest text-on-surface relative"
    >
      <FooterCenterSlot>
        <span className="text-primary font-bold">
          {`FILTER:${filterState.toUpperCase()} // SORT:${sortBy.toUpperCase()}`}
        </span>
      </FooterCenterSlot>

      <div className="max-w-5xl mx-auto px-8 pt-12 pb-20">
        <header className="mb-12">
          <div className="flex items-center space-x-2 text-outline font-medium tracking-tight text-sm">
            <span className="hover:text-primary cursor-pointer transition-colors">
              bistro
            </span>
            <span className="text-outline-variant">/</span>
            <span className="text-on-surface font-bold">inbox</span>
          </div>
          <div className="mt-4 flex items-baseline justify-between border-b border-outline-variant/30 pb-2">
            <h1 className="text-xs font-mono uppercase tracking-[0.2em] text-outline">
              {FILTER_TITLES[filterState]}
            </h1>
            <span className="text-xs font-mono text-outline-variant">
              <span className="text-primary font-bold">
                NEEDS ACTION: {triage.needsAction}
              </span>
              {' · '}WAITING: {triage.waitingAuthor}
              {' · '}FYI: {triage.fyi}
              {' · '}TOTAL: {prs.length}
            </span>
          </div>
        </header>

        <PRListTable prs={prs} isLoading={isLoading} error={error} />
      </div>

      {/* Floating filter / sort card */}
      <div className="fixed top-8 right-8 flex flex-col items-end gap-2 z-20 pointer-events-none">
        <div className="bg-surface-container-lowest border border-outline-variant/30 p-4 shadow-ambient pointer-events-auto">
          <div className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-8">
              <span className="font-mono text-[10px] text-outline">
                FILTER:
              </span>
              <span className="font-mono text-[10px] text-on-surface font-bold">
                IS:{filterState.toUpperCase()}
              </span>
            </div>
            <div className="flex items-center justify-between gap-8">
              <span className="font-mono text-[10px] text-outline">SORT:</span>
              <span className="font-mono text-[10px] text-on-surface">
                {sortBy === 'updated' ? 'UPDATED_FIRST' : 'CREATED_FIRST'}
              </span>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}
