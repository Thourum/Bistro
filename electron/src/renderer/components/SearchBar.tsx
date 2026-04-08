import React from 'react';

export function SearchBar() {
  return (
    <input
      placeholder="Search PRs..."
      className="flex-1 px-3 py-2 border border-outline-variant bg-surface-container-lowest text-on-surface font-mono text-sm focus:outline-none focus:border-primary"
    />
  );
}
