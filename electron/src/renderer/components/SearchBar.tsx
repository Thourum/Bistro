import React from 'react';
import { Input } from '@renderer/components/ui/input';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

export function SearchBar() {
  const query = useStore(appStore, (s: UIState) => s.searchQuery);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    appStore.setState((s) => ({ ...s, searchQuery: value }));
  };

  const handleFocus = () => {
    appStore.setState((s) => ({ ...s, mode: 'search' }));
  };

  const handleBlur = () => {
    if (!query) {
      appStore.setState((s) => ({ ...s, mode: 'list' }));
    }
  };

  return (
    <Input
      type="text"
      placeholder="Search PRs..."
      value={query}
      onChange={handleChange}
      onFocus={handleFocus}
      onBlur={handleBlur}
      className="flex-1 px-3 py-2 bg-surface-container-lowest border border-outline-variant"
    />
  );
}
