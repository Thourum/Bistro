import type React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { useWebSocketPRList as usePRList } from '../hooks/useWebSocketPRList';
import { appStore } from '../store';
import { initialUIState, type UIState } from '../store/ui';
import type { PR } from '../types';

type Icon = string;

interface Command {
  readonly id: string;
  readonly name: string;
  readonly description?: string;
  readonly icon: Icon;
  readonly shortcut?: readonly string[];
  readonly dev?: boolean;
}

const isDev = import.meta.env.DEV;

const COMMANDS: readonly Command[] = [
  {
    id: 'help',
    name: 'Show keyboard shortcuts',
    icon: 'keyboard',
    shortcut: ['?'],
  },
  {
    id: 'settings',
    name: 'Open project settings',
    icon: 'settings',
    shortcut: ['⌘', ','],
  },
  {
    id: 'clear',
    name: 'Clear all filters and search',
    icon: 'filter_alt_off',
  },
  { id: 'focus-list', name: 'Focus PR list', icon: 'list_alt' },
  { id: 'focus-detail', name: 'Focus PR detail', icon: 'article' },
  { id: 'about', name: 'About BISTRO', icon: 'info' },
  {
    id: 'reset-store',
    name: 'Reset store to initial state',
    icon: 'restart_alt',
    description: 'DEV ONLY',
    dev: true,
  },
];

/**
 * Command Palette — matches `command_palette/code.html`.
 *
 * Overlay: `fixed inset-0` with `bg-surface/80 backdrop-blur-[20px]`; the box
 * sits ~153px from the top in a `max-w-2xl` container. Results are grouped
 * into _Recently opened PRs_ and _Available commands_; Arrow keys move
 * between groups; Enter selects; Escape closes.
 */
export function CommandPalette(): React.ReactElement | null {
  const mode = useStore(appStore, (s: UIState) => s.mode);
  const filterState = useStore(appStore, (s: UIState) => s.filterState);
  const isOpen = mode === 'command';
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const { send } = useWebSocket();
  const prefetchedRef = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const { data: prs = [] } = usePRList(filterState);
  const recentPRs = useMemo(() => prs.slice(0, 5), [prs]);

  const filteredPRs = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === '') return recentPRs;
    return recentPRs.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.author.toLowerCase().includes(q) ||
        String(p.number).includes(q),
    );
  }, [recentPRs, query]);

  const filteredCommands = useMemo(() => {
    const q = query.trim().toLowerCase();
    return COMMANDS.filter((cmd) => {
      if (cmd.dev === true && !isDev) return false;
      if (q === '') return true;
      return (
        cmd.name.toLowerCase().includes(q) ||
        cmd.description?.toLowerCase().includes(q) === true
      );
    });
  }, [query]);

  // Linearize items so arrow keys walk across groups.
  const items = useMemo(
    () => [
      ...filteredPRs.map((pr) => ({ kind: 'pr' as const, pr })),
      ...filteredCommands.map((cmd) => ({ kind: 'command' as const, cmd })),
    ],
    [filteredPRs, filteredCommands],
  );

  useEffect(() => {
    setSelectedIndex(0);
  }, []);

  useEffect(() => {
    if (selectedIndex >= items.length) {
      setSelectedIndex(Math.max(0, items.length - 1));
    }
  }, [items, selectedIndex]);

  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [isOpen]);

  useEffect(() => {
    if (query.toLowerCase().startsWith('repo') && !prefetchedRef.current) {
      prefetchedRef.current = true;
      appStore.setState((s) => ({ ...s, reposLoading: true }));
      const requestId = send('REQUEST_REPO_LIST', {});
      if (requestId === undefined) {
        appStore.setState((s) => ({ ...s, reposLoading: false }));
        prefetchedRef.current = false;
      }
    }
  }, [query, send]);

  const handleClose = (): void => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
    setQuery('');
    prefetchedRef.current = false;
  };

  const handleExecutePR = (pr: PR): void => {
    const idx = prs.findIndex((p) => p.id === pr.id);
    appStore.setState((s) => ({
      ...s,
      mode: 'detail',
      selectedPRIndex: idx >= 0 ? idx : 0,
    }));
    setQuery('');
  };

  const handleExecuteCommand = (command: Command): void => {
    switch (command.id) {
      case 'settings':
        appStore.setState((s) => ({ ...s, mode: 'settings' }));
        setQuery('');
        return;
      case 'help':
        appStore.setState((s) => ({ ...s, mode: 'help' }));
        setQuery('');
        return;
      case 'focus-list':
        appStore.setState((s) => ({ ...s, mode: 'list' }));
        setQuery('');
        return;
      case 'focus-detail':
        appStore.setState((s) => ({ ...s, mode: 'detail' }));
        setQuery('');
        return;
      case 'clear':
        appStore.setState((s) => ({
          ...s,
          mode: 'list',
          searchQuery: '',
          filterState: 'open',
          filterMenuOpen: false,
        }));
        setQuery('');
        return;
      case 'reset-store':
        appStore.setState(() => ({ ...initialUIState, mode: 'list' }));
        setQuery('');
        return;
      default:
        handleClose();
    }
  };

  const handleExecute = (idx: number): void => {
    const item = items[idx];
    if (item === undefined) return;
    if (item.kind === 'pr') handleExecutePR(item.pr);
    else handleExecuteCommand(item.cmd);
  };

  const handleInputKeyDown = (
    e: React.KeyboardEvent<HTMLInputElement>,
  ): void => {
    if (e.key === 'Escape') {
      e.preventDefault();
      handleClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((i) => Math.min(i + 1, items.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      handleExecute(selectedIndex);
    }
  };

  if (!isOpen) return null;

  // Track a running index matching the linearised `items` array so the
  // selection highlight aligns across groups.
  let cursor = -1;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
      className="fixed inset-0 z-40 bg-surface/80 backdrop-blur-[20px] flex items-start justify-center pt-[153px]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) handleClose();
      }}
    >
      <div className="w-full max-w-2xl mx-4 bg-surface-container-lowest border border-primary shadow-[0_10px_30px_rgba(25,28,29,0.04)] flex flex-col overflow-hidden">
        {/* Search input header */}
        <div className="flex items-center px-4 h-14 border-b border-outline-variant/20">
          <span className="material-symbols-outlined text-primary mr-3">
            search
          </span>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleInputKeyDown}
            placeholder="Type a command or search…"
            className="flex-1 bg-transparent border-none text-on-surface placeholder:text-outline text-lg outline-none focus:ring-0"
          />
          <div className="flex items-center gap-2 shrink-0">
            <kbd className="font-mono text-[10px] px-1.5 py-0.5 bg-surface-container border border-outline-variant text-on-surface-variant">
              ESC
            </kbd>
            <span className="font-mono text-[10px] text-outline uppercase tracking-widest">
              to close
            </span>
          </div>
        </div>

        {/* Results */}
        <div className="max-h-[614px] overflow-y-auto py-2">
          {filteredPRs.length > 0 && (
            <GroupHeader title="Recently opened PRs" />
          )}
          {filteredPRs.map((pr) => {
            cursor += 1;
            const idx = cursor;
            const isActive = idx === selectedIndex;
            return (
              <PRResultRow
                key={pr.id}
                pr={pr}
                active={isActive}
                onSelect={() => handleExecute(idx)}
                onHover={() => setSelectedIndex(idx)}
              />
            );
          })}

          {filteredCommands.length > 0 && (
            <GroupHeader title="Available commands" spaced />
          )}
          {filteredCommands.map((cmd) => {
            cursor += 1;
            const idx = cursor;
            const isActive = idx === selectedIndex;
            return (
              <CommandRow
                key={cmd.id}
                command={cmd}
                active={isActive}
                onSelect={() => handleExecute(idx)}
                onHover={() => setSelectedIndex(idx)}
              />
            );
          })}

          {items.length === 0 && (
            <div className="px-6 py-6 text-center text-sm text-outline font-mono tracking-wide">
              No matches
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-4 h-8 bg-surface-container-low border-t border-outline-variant/20 flex items-center justify-between">
          <div className="flex gap-4">
            <div className="flex items-center gap-1">
              <span className="material-symbols-outlined text-[14px] text-outline">
                keyboard_arrow_up
              </span>
              <span className="material-symbols-outlined text-[14px] text-outline">
                keyboard_arrow_down
              </span>
              <span className="font-mono text-[9px] text-outline uppercase tracking-tight">
                Navigate
              </span>
            </div>
            <div className="flex items-center gap-1">
              <kbd className="font-mono text-[9px] px-1 bg-surface border border-outline-variant text-outline">
                ↩
              </kbd>
              <span className="font-mono text-[9px] text-outline uppercase tracking-tight">
                Select
              </span>
            </div>
          </div>
          <div className="font-mono text-[9px] text-outline uppercase tracking-widest">
            {'BISTRO // V2.4.1'}
          </div>
        </div>
      </div>
    </div>
  );
}

function GroupHeader({ title, spaced }: { title: string; spaced?: boolean }) {
  return (
    <div className={`px-4 py-2 ${spaced === true ? 'mt-4' : ''}`}>
      <h3 className="font-mono text-[10px] text-outline uppercase tracking-widest">
        {title}
      </h3>
    </div>
  );
}

interface RowProps {
  active: boolean;
  onSelect: () => void;
  onHover: () => void;
}

function PRResultRow({ pr, ...rest }: { pr: PR } & RowProps) {
  const relAgo = relativeTime(pr.updated_at ?? pr.created_at);
  return (
    <Row {...rest}>
      <div className="flex items-center gap-3 min-w-0">
        <span className="material-symbols-outlined text-primary shrink-0">
          rebase
        </span>
        <div className="flex flex-col min-w-0">
          <span className="text-sm font-medium text-on-surface truncate">
            {pr.title}
          </span>
          <span className="font-mono text-[10px] text-outline truncate">
            {pr.owner}/{pr.repo}#{pr.number} • {relAgo} by @{pr.author}
          </span>
        </div>
      </div>
      <div className="hidden group-hover:flex items-center gap-2 shrink-0">
        <span className="font-mono text-[10px] text-outline uppercase">
          Select
        </span>
        <kbd className="font-mono text-[10px] px-1.5 py-0.5 bg-surface border border-outline-variant text-on-surface-variant">
          ENTER
        </kbd>
      </div>
    </Row>
  );
}

function CommandRow({ command, ...rest }: { command: Command } & RowProps) {
  return (
    <Row {...rest}>
      <div className="flex items-center gap-3 min-w-0">
        <span className="material-symbols-outlined text-on-surface-variant shrink-0">
          {command.icon}
        </span>
        <div className="flex flex-col min-w-0">
          <span className="text-sm font-medium text-on-surface truncate">
            {command.name}
          </span>
          {command.description !== undefined && (
            <span className="font-mono text-[10px] text-outline truncate">
              {command.description}
            </span>
          )}
        </div>
      </div>
      {command.shortcut !== undefined && (
        <div className="flex items-center gap-1 opacity-60 shrink-0">
          {command.shortcut.map((k) => (
            <kbd
              key={`${command.id}-${k}`}
              className="font-mono text-[10px] px-1.5 py-0.5 bg-surface border border-outline-variant text-on-surface-variant"
            >
              {k}
            </kbd>
          ))}
        </div>
      )}
    </Row>
  );
}

function Row({
  active,
  onSelect,
  onHover,
  children,
}: RowProps & { children: React.ReactNode }) {
  return (
    <button
      type="button"
      onMouseDown={(e) => {
        e.preventDefault();
        onSelect();
      }}
      onMouseMove={onHover}
      className={`group w-full flex items-center justify-between px-4 py-3 text-left cursor-pointer transition-colors ${
        active
          ? 'bg-surface-container-highest border-l-2 border-primary'
          : 'border-l-2 border-transparent hover:bg-surface-container-highest'
      }`}
    >
      {children}
    </button>
  );
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour}h ago`;
  const day = Math.floor(hour / 24);
  return `${day}d ago`;
}
