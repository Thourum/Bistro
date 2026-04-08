import React, { useState } from 'react';
import { Input } from '@renderer/components/ui/input';
import { appStore } from '../store';

export function SearchBar() {
  const [query, setQuery] = useState('');

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setQuery(value);
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
