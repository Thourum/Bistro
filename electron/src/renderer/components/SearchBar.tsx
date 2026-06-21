import { Input } from '@renderer/components/ui/input';
import type React from 'react';
import { useEffect, useRef } from 'react';
import { useStore } from '../hooks/useStore';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

export function SearchBar() {
  const query = useStore(appStore, (s: UIState) => s.searchQuery);
  const { activeOverlay, openSearch, closeSearch } = useOverlays();
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (activeOverlay === 'search' && inputRef.current) {
      inputRef.current.focus();
    }
  }, [activeOverlay]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    // Keep writing appStore.searchQuery — route-search bridge depends on it.
    appStore.setState((s) => ({ ...s, searchQuery: value }));
  };

  const handleFocus = () => {
    openSearch();
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      closeSearch({ clearQuery: true });
      inputRef.current?.blur();
    } else if (e.key === 'Enter') {
      // Apply search filter — exit search overlay but keep query
      closeSearch();
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
