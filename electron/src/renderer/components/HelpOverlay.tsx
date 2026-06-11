import type React from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { KeyBinding } from '../utils/keybindings';
import { getEffectiveBindings } from '../utils/keybindings';
import { formatBindingKeys } from '../utils/legendPresets';
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

function toRow(b: KeyBinding): ShortcutRow {
  return { key: formatBindingKeys(b), description: b.description };
}

// Sections are keyed by stable actionIds so user-remapped keys stay grouped.
const NAVIGATION_IDS = new Set([
  'list.down',
  'list.up',
  'list.top',
  'list.bottom',
  'list.pageDown',
  'list.pageUp',
  'list.jumpTo1',
  'detail.nextFile',
  'detail.prevFile',
  'detail.scrollLeft',
  'detail.scrollDown',
  'detail.scrollUp',
  'detail.scrollRight',
  'detail.pageDown',
  'detail.pageUp',
  'detail.scrollTop',
  'detail.scrollBottom',
]);

const ACTION_IDS = new Set([
  'list.open',
  'list.openBrowser',
  'list.preview',
  'list.draft',
  'list.mute',
  'list.star',
  'detail.viewDiff',
  'detail.approve',
  'detail.comment',
  'detail.reply',
  'global.escape',
  'filter.needsAction',
  'filter.open',
  'filter.draft',
  'filter.all',
  'filter.close',
]);

const SEARCH_IDS = new Set([
  'list.search',
  'list.filter',
  'filter.toggle',
  'global.command',
  'global.help',
  'global.search.mac',
  'global.search.win',
]);

function buildSections(bindings: KeyBinding[]): ShortcutSection[] {
  const visible = bindings.filter((b) => b.hidden !== true);

  const navBindings = visible.filter((b) => NAVIGATION_IDS.has(b.actionId));
  const actionBindings = visible.filter((b) => ACTION_IDS.has(b.actionId));
  const searchBindings = visible.filter((b) => SEARCH_IDS.has(b.actionId));
  const globalBindings = visible.filter(
    (b) =>
      !NAVIGATION_IDS.has(b.actionId) &&
      !ACTION_IDS.has(b.actionId) &&
      !SEARCH_IDS.has(b.actionId),
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
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const isOpen = mode === 'help';
  const sections = buildSections(getEffectiveBindings(overrides));

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
