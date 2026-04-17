import { FooterCenterSlot } from '../components/FooterSlot';
import { PRListTable } from '../components/PRListTable';
import { useStore } from '../hooks/useStore';
import { useSyncEvents } from '../hooks/useSyncEvents';
import { useWebSocketPRList } from '../hooks/useWebSocketPRList';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

/**
 * PR inbox — matches `pr_inbox_minimal_light/code.html`.
 *
 * The legend shortcuts are rendered by the global `AppFooter` (binding-derived),
 * and the filter/sort status is projected into the footer's center region via
 * `FooterCenterSlot`.
 */
export function PRListPage() {
  useSyncEvents();

  const filterState = useStore(appStore, (s: UIState) => s.filterState);
  const sortBy = useStore(appStore, (s: UIState) => s.sortBy);
  const { data: prs = [] } = useWebSocketPRList(filterState);

  return (
    <main className="flex-1 overflow-y-auto bg-surface-container-lowest text-on-surface relative">
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
              {filterState === 'open'
                ? 'Open Pull Requests'
                : filterState === 'draft'
                  ? 'Draft Pull Requests'
                  : 'All Pull Requests'}
            </h1>
            <span className="text-xs font-mono text-outline-variant">
              TOTAL: {prs.length}
            </span>
          </div>
        </header>

        <PRListTable />
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
        <div className="bg-primary text-on-primary px-3 py-1 font-mono text-[10px] font-bold pointer-events-auto">
          CMD + K
        </div>
      </div>
    </main>
  );
}
