import type React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { RepoPicker } from './RepoPicker';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { Input } from './ui/input';
import { ScrollArea } from './ui/scroll-area';

interface Command {
  readonly id: string;
  readonly name: string;
  readonly description?: string;
  readonly dev?: boolean;
}

const isDev = import.meta.env.DEV;

const COMMANDS: readonly Command[] = [
  { id: 'help', name: 'help', description: 'Show keyboard shortcuts' },
  { id: 'settings', name: 'settings', description: 'Open settings' },
  { id: 'repos', name: 'repos', description: 'Manage watched repositories' },
  { id: 'clear', name: 'clear', description: 'Clear all filters and search' },
  { id: 'focus-list', name: 'focus list', description: 'Focus PR list' },
  { id: 'focus-detail', name: 'focus detail', description: 'Focus PR detail' },
  { id: 'about', name: 'about', description: 'Show app version and credits' },
  {
    id: 'reset-store',
    name: 'reset store',
    description: '[DEV] Reset store to initial state',
    dev: true,
  },
] as const;

export function CommandPalette(): React.ReactElement | null {
  const mode = useStore(appStore, (s) => s.mode);
  const isOpen = mode === 'command';
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [view, setView] = useState<'commands' | 'repos'>('commands');
  const { send } = useWebSocket();
  const prefetchedRef = useRef(false);

  const filteredCommands = useMemo(
    () =>
      COMMANDS.filter(
        (cmd) =>
          (!cmd.dev || isDev) &&
          (cmd.name.includes(query.toLowerCase()) ||
            cmd.description?.toLowerCase().includes(query.toLowerCase())),
      ),
    [query],
  );

  // Reset selection when query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Prefetch repo list when user starts typing "repo..."
  useEffect(() => {
    if (query.toLowerCase().startsWith('repo') && !prefetchedRef.current) {
      prefetchedRef.current = true;
      appStore.setState((s) => ({ ...s, reposLoading: true }));
      send('REQUEST_REPO_LIST', {});
    }
  }, [query, send]);

  const handleClose = (): void => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
    setQuery('');
    setView('commands');
    prefetchedRef.current = false;
  };

  const handleExecute = (command: Command): void => {
    switch (command.id) {
      case 'repos':
        setView('repos');
        setQuery('');
        return;
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
      case 'about':
        // TODO: Show about dialog with version info
        handleClose();
        return;
      default:
        handleClose();
    }
  };

  const handleInputKeyDown = (
    e: React.KeyboardEvent<HTMLInputElement>,
  ): void => {
    if (e.key === 'Escape') {
      handleClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((i) => Math.min(i + 1, filteredCommands.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter' && filteredCommands.length > 0) {
      e.preventDefault();
      handleExecute(filteredCommands[selectedIndex]!);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="w-full max-w-md">
        <DialogHeader>
          <DialogTitle>
            {view === 'repos' ? 'Watched Repositories' : 'Command Palette'}
          </DialogTitle>
        </DialogHeader>

        {view === 'repos' ? (
          <RepoPicker onClose={() => setView('commands')} />
        ) : (
          <>
            <Input
              type="text"
              placeholder=":"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleInputKeyDown}
              className="font-mono"
              autoFocus
            />

            <ScrollArea className="h-64">
              <div className="divide-y divide-outline-variant">
                {filteredCommands.map((cmd, idx) => (
                  <div
                    key={cmd.id}
                    onClick={() => handleExecute(cmd)}
                    className={`px-4 py-3 cursor-pointer transition-colors ${
                      idx === selectedIndex
                        ? 'bg-surface-container-low border-l-2 border-l-primary'
                        : 'hover:bg-surface-container-low'
                    }`}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => e.key === 'Enter' && handleExecute(cmd)}
                  >
                    <div className="font-mono text-sm text-on-surface">
                      {cmd.name}
                    </div>
                    {cmd.description !== undefined && (
                      <div className="text-[11px] text-on-surface-variant mt-1">
                        {cmd.description}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </ScrollArea>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
