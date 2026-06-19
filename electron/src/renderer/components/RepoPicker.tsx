import type React from 'react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { RepoInfo } from '../types';

interface RepoPickerProps {
  onClose: () => void;
}

export function RepoPicker({ onClose }: RepoPickerProps): React.ReactElement {
  const repos = useStore(appStore, (s: UIState) => s.repos);
  const reposLoading = useStore(appStore, (s: UIState) => s.reposLoading);
  const { send } = useWebSocket();

  const [searchQuery, setSearchQuery] = useState('');
  const [focusedIndex, setFocusedIndex] = useState(0);
  const [isSearching, setIsSearching] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Fetch repo list on mount
  useEffect(() => {
    appStore.setState((s) => ({ ...s, reposLoading: true }));
    send('REQUEST_REPO_LIST', {});
  }, [send]);

  useEffect(() => {
    setTimeout(() => containerRef.current?.focus(), 0);
  }, []);

  const sortedRepos = [...repos].sort((a, b) => {
    if (a.watched && !b.watched) return -1;
    if (!a.watched && b.watched) return 1;
    return `${a.owner}/${a.repo}`.localeCompare(`${b.owner}/${b.repo}`);
  });

  const filteredRepos = searchQuery
    ? sortedRepos.filter((r) =>
        `${r.owner}/${r.repo}`
          .toLowerCase()
          .includes(searchQuery.toLowerCase()),
      )
    : sortedRepos;

  // Clamp focusedIndex when list changes
  useEffect(() => {
    setFocusedIndex((i) => Math.min(i, Math.max(0, filteredRepos.length - 1)));
  }, [filteredRepos.length]);

  const toggleWatch = useCallback(
    (repo: RepoInfo): void => {
      send('TOGGLE_WATCH_REPO', { repo_id: repo.id, watched: !repo.watched });
    },
    [send],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent): void => {
      if (e.key === 'Escape') {
        if (isSearching || searchQuery) {
          setSearchQuery('');
          setIsSearching(false);
          containerRef.current?.focus();
        } else {
          onClose();
        }
        return;
      }

      if (isSearching) return;

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setFocusedIndex((i) => Math.min(i + 1, filteredRepos.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setFocusedIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        const repo = filteredRepos[focusedIndex];
        if (repo) {
          toggleWatch(repo);
        }
      } else if (e.key === '/') {
        e.preventDefault();
        setIsSearching(true);
        setTimeout(() => searchInputRef.current?.focus(), 0);
      }
    },
    [
      isSearching,
      searchQuery,
      filteredRepos,
      focusedIndex,
      toggleWatch,
      onClose,
    ],
  );

  if (reposLoading && repos.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-on-surface-variant text-sm">
        Loading repositories...
      </div>
    );
  }

  if (!reposLoading && repos.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-32 text-on-surface-variant text-sm gap-2">
        <p>No repositories found.</p>
        <p className="text-xs">Make sure you are connected to a provider.</p>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      role="application"
      aria-label="Repository picker"
      className="outline-none"
      tabIndex={-1}
      onKeyDown={handleKeyDown}
    >
      {isSearching ? (
        <div className="mb-2">
          <input
            ref={searchInputRef}
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.stopPropagation();
                setSearchQuery('');
                setIsSearching(false);
                containerRef.current?.focus();
              }
            }}
            placeholder="Filter repos..."
            className="w-full px-3 py-1.5 text-sm bg-surface-container border border-outline-variant text-on-surface placeholder:text-on-surface-variant font-mono outline-none"
          />
        </div>
      ) : (
        <p className="text-xs text-on-surface-variant mb-2 font-mono">
          ↑↓ navigate · Space/Enter toggle · / search · Esc close
        </p>
      )}

      <div className="divide-y divide-outline-variant max-h-48 overflow-y-auto">
        {filteredRepos.map((repo, idx) => {
          const label = `${repo.owner}/${repo.repo}`;
          const isFocused = idx === focusedIndex;
          return (
            <button
              type="button"
              key={repo.id}
              onClick={() => {
                setFocusedIndex(idx);
                toggleWatch(repo);
              }}
              className={`w-full flex items-center justify-between px-3 py-2 cursor-pointer transition-colors text-left ${
                isFocused
                  ? 'bg-surface-container border-l-2 border-l-primary'
                  : 'hover:bg-surface-container'
              }`}
              tabIndex={-1}
            >
              <span
                className={
                  repo.watched
                    ? 'font-bold text-primary text-sm font-mono'
                    : 'text-on-surface-variant text-sm font-mono'
                }
              >
                {label}
              </span>
              <span className="text-xs text-on-surface-variant ml-2">
                {repo.watched ? 'watching' : ''}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
