import type React from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { BindingDefinition } from '../utils/keyboard';
import { bindingSequence, KEYBOARD_BINDING_GROUPS } from '../utils/keyboard';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { ScrollArea } from './ui/scroll-area';

interface ShortcutRow {
  readonly key: string;
  readonly description: string;
}

interface ShortcutSection {
  readonly title: string;
  readonly shortcuts: readonly ShortcutRow[];
}

function toRow(b: BindingDefinition): ShortcutRow {
  return { key: bindingSequence(b), description: b.description };
}

function buildSections(): ShortcutSection[] {
  const navigationKeys = new Set([
    'j',
    'k',
    'gg',
    'G',
    'n',
    'p',
    'h',
    'l',
    'ctrl+d',
    'ctrl+u',
    '0',
    '$',
  ]);
  const actionKeys = new Set([
    'Enter',
    'a',
    'aa',
    'Escape',
    'c',
    'r',
    'o',
    ' ',
    'd',
    'm',
    's',
  ]);
  const searchKeys = new Set(['/', ':', 'f', '?', 'meta+/', 'ctrl+/']);

  const navBindings = KEYBOARD_BINDING_GROUPS.filter((b) =>
    navigationKeys.has(bindingSequence(b)),
  );
  const actionBindings = KEYBOARD_BINDING_GROUPS.filter((b) =>
    actionKeys.has(bindingSequence(b)),
  );
  const searchBindings = KEYBOARD_BINDING_GROUPS.filter((b) =>
    searchKeys.has(bindingSequence(b)),
  );
  const globalBindings = KEYBOARD_BINDING_GROUPS.filter(
    (b) =>
      !navigationKeys.has(bindingSequence(b)) &&
      !actionKeys.has(bindingSequence(b)) &&
      !searchKeys.has(bindingSequence(b)),
  );

  const sections: ShortcutSection[] = [
    { title: 'Navigation', shortcuts: navBindings.map(toRow) },
    { title: 'Actions', shortcuts: actionBindings.map(toRow) },
    { title: 'Search & Commands', shortcuts: searchBindings.map(toRow) },
  ];

  if (globalBindings.length > 0) {
    sections.push({ title: 'Global', shortcuts: globalBindings.map(toRow) });
  }

  return sections.filter((s) => s.shortcuts.length > 0);
}

export function HelpOverlay(): React.ReactElement | null {
  const mode = useStore(appStore, (s) => s.mode);
  const isOpen = mode === 'help';
  const sections = buildSections();

  const handleClose = (): void => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl max-h-[80vh]">
        <DialogHeader>
          <DialogTitle className="font-mono">Keyboard Shortcuts</DialogTitle>
        </DialogHeader>

        <ScrollArea className="h-full pr-4">
          <div className="space-y-6">
            {sections.map((section) => (
              <section key={section.title}>
                <h3 className="font-bold text-sm text-on-surface uppercase tracking-widest mb-3">
                  {section.title}
                </h3>
                <div className="space-y-2 text-sm">
                  {section.shortcuts.map((shortcut) => (
                    <div key={shortcut.key} className="flex justify-between">
                      <kbd className="font-mono text-on-surface-variant bg-surface-container px-2 py-0.5 rounded text-xs min-w-[3rem] text-center">
                        {shortcut.key}
                      </kbd>
                      <span className="text-on-surface">
                        {shortcut.description}
                      </span>
                    </div>
                  ))}
                </div>
              </section>
            ))}

            <div className="text-xs text-on-surface-variant text-center mt-4 pt-4 border-t border-outline-variant">
              Press{' '}
              <kbd className="font-mono bg-surface-container px-1 rounded">
                ?
              </kbd>{' '}
              or{' '}
              <kbd className="font-mono bg-surface-container px-1 rounded">
                Esc
              </kbd>{' '}
              to close
            </div>
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
