import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';
import { ScrollArea } from './ui/scroll-area';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';

interface ShortcutRow {
  readonly key: string;
  readonly description: string;
}

interface ShortcutSection {
  readonly title: string;
  readonly shortcuts: readonly ShortcutRow[];
}

const SHORTCUT_SECTIONS: readonly ShortcutSection[] = [
  {
    title: 'Navigation',
    shortcuts: [
      { key: 'j/k', description: 'Move up/down in list' },
      { key: 'gg', description: 'Jump to top' },
      { key: 'G', description: 'Jump to bottom' },
    ],
  },
  {
    title: 'Actions',
    shortcuts: [
      { key: 'Enter', description: 'Open selected PR' },
      { key: 'a', description: 'Approve PR' },
      { key: 'Esc', description: 'Close overlay / go back' },
    ],
  },
  {
    title: 'Search & Commands',
    shortcuts: [
      { key: '/', description: 'Focus search' },
      { key: ':', description: 'Open command palette' },
      { key: 'f', description: 'Toggle filter menu' },
      { key: '?', description: 'Show this help' },
    ],
  },
] as const;

export function HelpOverlay(): React.ReactElement | null {
  const mode = useStore(appStore, (s) => s.mode);
  const isOpen = mode === 'help';

  const handleClose = (): void => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl max-h-[80vh]">
        <DialogHeader>
          <DialogTitle>Keyboard Shortcuts</DialogTitle>
        </DialogHeader>

        <ScrollArea className="h-full pr-4">
          <div className="space-y-6">
            {SHORTCUT_SECTIONS.map((section) => (
              <section key={section.title}>
                <h3 className="font-bold text-sm text-on-surface uppercase tracking-widest mb-3">
                  {section.title}
                </h3>
                <div className="space-y-2 text-sm">
                  {section.shortcuts.map((shortcut) => (
                    <div key={shortcut.key} className="flex justify-between">
                      <span className="font-mono text-on-surface-variant">{shortcut.key}</span>
                      <span className="text-on-surface">{shortcut.description}</span>
                    </div>
                  ))}
                </div>
              </section>
            ))}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
