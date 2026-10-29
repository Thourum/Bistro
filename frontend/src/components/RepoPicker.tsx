import { Command } from 'cmdk';
import type React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useStore } from '../hooks/useStore';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { RepoInfo } from '../types';

interface RepoPickerProps {
  onClose: () => void;
}

/**
 * RepoPicker — cmdk-powered repository chooser.
 *
 * cmdk owns keyboard navigation, selected-item styling hooks, and keeping the
 * active item scrolled into view. We keep filtering external so watched and
 * unwatched repositories can stay in distinct groups with a separator.
 */
export function RepoPicker({ onClose }: RepoPickerProps): React.ReactElement {
  const repos = useStore(appStore, (s: UIState) => s.repos);
  const reposLoading = useStore(appStore, (s: UIState) => s.reposLoading);
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    appStore.setState((s) => ({ ...s, reposLoading: true }));
    wsClient.send('REQUEST_REPO_LIST', {});
  }, []);

  useEffect(() => {
    if (repos.length === 0) return;

    const timeoutId = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(timeoutId);
  }, [repos.length]);

  const filteredRepos = useMemo(() => {
    const q = query.trim().toLowerCase();
    return [...repos]
      .sort((a, b) => {
        if (a.watched && !b.watched) return -1;
        if (!a.watched && b.watched) return 1;
        return repoLabel(a).localeCompare(repoLabel(b));
      })
      .filter((repo) => q === '' || repoLabel(repo).toLowerCase().includes(q));
  }, [query, repos]);

  const watchedRepos = filteredRepos.filter((repo) => repo.watched);
  const otherRepos = filteredRepos.filter((repo) => !repo.watched);

  const toggleWatch = (repo: RepoInfo): void => {
    wsClient.send('TOGGLE_WATCH_REPO', {
      repo_id: repo.id,
      watched: !repo.watched,
    });
  };

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
    <Command
      shouldFilter={false}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          e.preventDefault();
          onClose();
        }
      }}
      className="outline-none"
    >
      <div className="flex items-center gap-3 mb-3 border border-outline-variant/40 bg-surface-container px-3 h-10">
        <span className="material-symbols-outlined text-primary shrink-0 text-[18px]">
          search
        </span>
        <Command.Input
          ref={inputRef}
          value={query}
          onValueChange={setQuery}
          placeholder="Filter repos..."
          className="flex-1 min-w-0 bg-transparent appearance-none border-0 shadow-none outline-none ring-0 text-on-surface placeholder:text-on-surface-variant text-sm font-mono focus:border-transparent focus:outline-none focus:ring-0 focus-visible:outline-none focus-visible:ring-0"
        />
        <div className="flex items-center gap-2 shrink-0">
          <kbd className="font-mono text-[10px] px-1.5 py-0.5 bg-surface border border-outline-variant text-on-surface-variant">
            ESC
          </kbd>
          <span className="font-mono text-[10px] text-outline uppercase tracking-widest">
            close
          </span>
        </div>
      </div>

      <p className="text-xs text-on-surface-variant mb-2 font-mono">
        ↑↓ navigate · Enter toggle watch · Type to filter
      </p>

      <Command.List className="max-h-80 overflow-y-auto py-1">
        <Command.Empty className="px-3 py-6 text-center text-sm text-outline font-mono tracking-wide">
          No repositories match
        </Command.Empty>

        {watchedRepos.length > 0 && (
          <RepoGroup heading="Watched repositories">
            {watchedRepos.map((repo) => (
              <RepoItem key={repo.id} repo={repo} onSelect={toggleWatch} />
            ))}
          </RepoGroup>
        )}

        {watchedRepos.length > 0 && otherRepos.length > 0 && (
          <Command.Separator
            alwaysRender
            className="my-2 h-px bg-outline-variant/40"
          />
        )}

        {otherRepos.length > 0 && (
          <RepoGroup heading="Other repositories">
            {otherRepos.map((repo) => (
              <RepoItem key={repo.id} repo={repo} onSelect={toggleWatch} />
            ))}
          </RepoGroup>
        )}
      </Command.List>
    </Command>
  );
}

function RepoGroup({
  heading,
  children,
}: {
  heading: string;
  children: React.ReactNode;
}) {
  return (
    <Command.Group
      heading={heading}
      className="[&_[cmdk-group-heading]]:font-mono [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:text-outline [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-widest [&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-items]]:space-y-1"
    >
      {children}
    </Command.Group>
  );
}

function RepoItem({
  repo,
  onSelect,
}: {
  repo: RepoInfo;
  onSelect: (repo: RepoInfo) => void;
}) {
  const label = repoLabel(repo);
  return (
    <Command.Item
      value={`repo-${repo.id}`}
      onSelect={() => onSelect(repo)}
      className="group w-full flex items-center justify-between px-3 py-2 cursor-pointer transition-colors text-left border-l-2 border-l-transparent hover:bg-surface-container data-[selected=true]:bg-surface-container-highest data-[selected=true]:border-l-primary"
    >
      <span
        className={`text-sm font-mono truncate transition-colors ${
          repo.watched ? 'font-bold text-secondary' : 'text-on-surface-variant'
        } group-data-[selected=true]:font-semibold group-data-[selected=true]:text-primary`}
      >
        {label}
      </span>
      <span
        className={`text-xs ml-3 shrink-0 font-mono transition-colors ${
          repo.watched ? 'text-primary' : 'text-outline'
        } group-data-[selected=true]:text-primary`}
      >
        {repo.watched ? 'watching' : 'watch'}
      </span>
    </Command.Item>
  );
}

function repoLabel(repo: RepoInfo): string {
  return `${repo.owner}/${repo.repo}`;
}
