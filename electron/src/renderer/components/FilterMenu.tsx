import React from 'react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@renderer/components/ui/dropdown-menu';
import { Button } from '@renderer/components/ui/button';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

type FilterValue = UIState['filterState'];

const FILTER_OPTIONS: { label: string; value: FilterValue; hint: string }[] = [
  { label: 'Open', value: 'open', hint: 'o' },
  { label: 'Draft', value: 'draft', hint: 'd' },
  { label: 'All', value: 'all', hint: 'a' },
];

export function FilterMenu() {
  const filterState = useStore(appStore, (s) => s.filterState);

  const handleSelect = (value: FilterValue) => {
    appStore.setState((s) => ({ ...s, filterState: value }));
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" className="gap-2">
          <span>Filter</span>
          <span className="text-[10px] text-on-surface-variant font-mono">f</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40 rounded-none">
        {FILTER_OPTIONS.map(({ label, value, hint }) => (
          <DropdownMenuItem
            key={value}
            onSelect={() => handleSelect(value)}
            className={filterState === value ? 'font-semibold' : ''}
          >
            <span className="flex items-center gap-2 w-full">
              <span>{label}</span>
              <span className="text-[10px] text-on-surface-variant font-mono ml-auto">{hint}</span>
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
