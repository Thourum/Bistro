import type React from 'react';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { findActionKeys } from '../utils/legendPresets';
import { Button } from './ui/button';
import { Kbd } from './ui/kbd';

type FilterValue = UIState['filterState'];

const FILTER_OPTIONS: { label: string; value: FilterValue; hint: string }[] = [
  { label: 'Needs Action', value: 'needs_action', hint: 'n' },
  { label: 'Touches Your Code', value: 'touches_you', hint: 'y' },
  { label: 'Open', value: 'open', hint: 'o' },
  { label: 'Draft', value: 'draft', hint: 'd' },
  { label: 'All', value: 'all', hint: 'a' },
];

function FilterPanel(): React.ReactElement {
  const filterState = useStore(appStore, (s) => s.filterState);
  const { closeOverlay } = useOverlays();
  const toggleKeys = findActionKeys(useEffectiveBindings(), ['filter.toggle']);

  const handleSelect = (value: FilterValue): void => {
    appStore.setState((s) => ({ ...s, filterState: value }));
    closeOverlay();
  };

  return (
    <div
      className="absolute right-0 top-full z-50 mt-1 w-48 border border-outline-variant bg-surface-container shadow-ambient"
      role="menu"
    >
      <div className="border-b border-outline-variant px-3 py-2">
        <span className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline">
          Filters
        </span>
      </div>
      {FILTER_OPTIONS.map(({ label, value, hint }) => (
        <div
          key={value}
          onClick={() => handleSelect(value)}
          className={`flex cursor-crosshair items-center justify-between border-l-2 px-3 py-2 transition-colors duration-150 hover:bg-surface-container-low ${
            filterState === value
              ? 'border-primary bg-surface-container-low'
              : 'border-transparent'
          }`}
          role="menuitem"
          tabIndex={0}
          onKeyDown={(e) => e.key === 'Enter' && handleSelect(value)}
        >
          <span className="text-sm text-on-surface">{label}</span>
          <Kbd>{hint}</Kbd>
        </div>
      ))}
      <div className="border-t border-outline-variant px-3 py-2 font-mono text-[10px] text-outline">
        Press{' '}
        {toggleKeys !== undefined && (
          <>
            <Kbd>{toggleKeys}</Kbd> or{' '}
          </>
        )}
        <Kbd>Esc</Kbd> to close
      </div>
    </div>
  );
}

export function FilterMenu(): React.ReactElement {
  const { activeOverlay, toggleFilter } = useOverlays();
  const isOpen = activeOverlay === 'filter';
  const filterKeys = findActionKeys(useEffectiveBindings(), ['list.filter']);

  return (
    <div className="relative">
      <Button
        variant="outline"
        size="sm"
        onClick={toggleFilter}
        aria-label="Filter"
      >
        <span className="normal-case">Filter</span>
        {filterKeys !== undefined && (
          <Kbd className="border-0 bg-transparent px-0 text-outline-variant">
            {filterKeys}
          </Kbd>
        )}
      </Button>
      {isOpen && <FilterPanel />}
    </div>
  );
}
