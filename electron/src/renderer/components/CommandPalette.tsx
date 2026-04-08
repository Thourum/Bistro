import React, { useState, useMemo } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';
import { Input } from './ui/input';
import { ScrollArea } from './ui/scroll-area';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';

interface Command {
  readonly id: string;
  readonly name: string;
  readonly description?: string;
}

const COMMANDS: readonly Command[] = [
  { id: 'help', name: 'help', description: 'Show keyboard shortcuts' },
  { id: 'settings', name: 'settings', description: 'Open settings' },
  { id: 'clear', name: 'clear', description: 'Clear cache' },
  { id: 'focus-list', name: 'focus list', description: 'Focus PR list' },
  { id: 'focus-detail', name: 'focus detail', description: 'Focus PR detail' },
] as const;

export function CommandPalette(): React.ReactElement | null {
  const mode = useStore(appStore, (s) => s.mode);
  const isOpen = mode === 'command';
  const [query, setQuery] = useState('');

  const filteredCommands = useMemo(
    () =>
      COMMANDS.filter(
        (cmd) =>
          cmd.name.includes(query.toLowerCase()) ||
          cmd.description?.toLowerCase().includes(query.toLowerCase()),
      ),
    [query],
  );

  const handleClose = (): void => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
    setQuery('');
  };

  const handleExecute = (command: Command): void => {
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
      default:
        // TODO: Implement remaining commands (clear cache, etc.)
        handleClose();
    }
  };

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === 'Escape') {
      handleClose();
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="w-full max-w-md">
        <DialogHeader>
          <DialogTitle>Command Palette</DialogTitle>
        </DialogHeader>

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
            {filteredCommands.map((cmd) => (
              <div
                key={cmd.id}
                onClick={() => handleExecute(cmd)}
                className="px-4 py-3 hover:bg-surface-container-low cursor-pointer transition-colors"
                role="button"
                tabIndex={0}
                onKeyDown={(e) => e.key === 'Enter' && handleExecute(cmd)}
              >
                <div className="font-mono text-sm text-on-surface">{cmd.name}</div>
                {cmd.description !== undefined && (
                  <div className="text-[11px] text-on-surface-variant mt-1">
                    {cmd.description}
                  </div>
                )}
              </div>
            ))}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
