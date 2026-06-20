import type React from 'react';
import { useStore } from '../hooks/useStore';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

type FilterValue = UIState['filterState'];

const FILTER_OPTIONS: { label: string; value: FilterValue; hint: string }[] = [
  { label: 'Needs Action', value: 'needs_action', hint: 'n' },
  { label: 'Open', value: 'open', hint: 'o' },
  { label: 'Draft', value: 'draft', hint: 'd' },
  { label: 'All', value: 'all', hint: 'a' },
];

function FilterPanel(): React.ReactElement {
  const filterState = useStore(appStore, (s) => s.filterState);
  const { closeOverlay } = useOverlays();

  const handleSelect = (value: FilterValue): void => {
    appStore.setState((s) => ({ ...s, filterState: value }));
    closeOverlay();
  };

  return (
    <div
      className="absolute top-full right-0 mt-1 z-50 w-48 border border-outline-variant bg-surface-container rounded shadow-lg"
      role="menu"
    >
      <div className="px-3 py-2 border-b border-outline-variant">
        <span className="text-xs font-bold text-on-surface-variant uppercase tracking-widest">
          Filters
        </span>
      </div>
      {FILTER_OPTIONS.map(({ label, value, hint }) => (
        <div
          key={value}
          onClick={() => handleSelect(value)}
          className={`flex items-center justify-between px-3 py-2 cursor-pointer transition-colors hover:bg-surface-container-low ${
            filterState === value
              ? 'bg-surface-container-low font-semibold'
              : ''
          }`}
          role="menuitem"
          tabIndex={0}
          onKeyDown={(e) => e.key === 'Enter' && handleSelect(value)}
        >
          <span className="text-sm text-on-surface">{label}</span>
          <kbd className="text-[10px] text-on-surface-variant font-mono bg-surface-container px-1 rounded">
            {hint}
          </kbd>
        </div>
      ))}
      <div className="px-3 py-2 border-t border-outline-variant text-[10px] text-on-surface-variant">
        Press <kbd className="font-mono">f</kbd> or{' '}
        <kbd className="font-mono">Esc</kbd> to close
      </div>
    </div>
  );
}

export function FilterMenu(): React.ReactElement {
  const { activeOverlay, toggleFilter } = useOverlays();
  const isOpen = activeOverlay === 'filter';

  return (
    <div className="relative">
      <button
        type="button"
        onClick={toggleFilter}
        className="inline-flex items-center gap-2 px-3 py-1.5 border border-outline-variant rounded text-sm hover:bg-surface-container transition-colors"
        aria-label="Filter"
      >
        <span>Filter</span>
        <span className="text-[10px] text-on-surface-variant font-mono">f</span>
      </button>
      {isOpen && <FilterPanel />}
    </div>
  );
}
