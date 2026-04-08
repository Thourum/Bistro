import React, { useRef, useEffect } from 'react';
import { Input } from '@renderer/components/ui/input';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

export function SearchBar() {
  const query = useStore(appStore, (s: UIState) => s.searchQuery);
  const mode = useStore(appStore, (s: UIState) => s.mode);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (mode === 'search' && inputRef.current) {
      inputRef.current.focus();
    }
  }, [mode]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    appStore.setState((s) => ({ ...s, searchQuery: value }));
  };

  const handleFocus = () => {
    appStore.setState((s) => ({ ...s, mode: 'search' }));
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      appStore.setState((s) => ({ ...s, mode: 'list', searchQuery: '' }));
      inputRef.current?.blur();
    } else if (e.key === 'Enter') {
      // Apply search filter — exit search mode but keep query
      appStore.setState((s) => ({ ...s, mode: 'list' }));
      inputRef.current?.blur();
    }
  };

  return (
    <Input
      ref={inputRef}
      type="text"
      placeholder="Search PRs... (/ to focus)"
      value={query}
      onChange={handleChange}
      onFocus={handleFocus}
      onKeyDown={handleKeyDown}
      className="flex-1 px-3 py-2 bg-surface-container-lowest border border-outline-variant"
    />
  );
}
