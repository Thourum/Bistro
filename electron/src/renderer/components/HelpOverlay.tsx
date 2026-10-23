import { detectPlatform } from '@tanstack/react-hotkeys';
import type React from 'react';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useOverlays } from '../overlays/OverlayContext';
import type { KeyBinding, RouteBase } from '../utils/keybindings';
import { formatBindingKeys } from '../utils/legendPresets';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { Kbd } from './ui/kbd';
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
  // useSymbols: false renders 'Cmd+/' style labels instead of '⌘ /' symbols.
  return {
    key: formatBindingKeys(b, { useSymbols: false }),
    description: b.description,
  };
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
  'list.toggleGroup',
  'list.draft',
  'list.mute',
  'list.star',
  'detail.viewDiff',
  'detail.approve',
  'detail.comment',
  'detail.requestChanges',
  'detail.reply',
  'global.escape',
  'filter.needsAction',
  'filter.touchesYou',
  'filter.open',
  'filter.draft',
  'filter.all',
  'filter.close',
]);

// Diff-review keys get their own section: on the diff route they are the
// primary vocabulary, not incidental globals.
const DIFF_IDS = new Set([
  'diff.nextFile',
  'diff.prevFile',
  'diff.nextComment',
  'diff.prevComment',
  'diff.sinceLastReview',
  'diff.toggleStyle',
  'diff.toggleWhitespace',
  'diff.toggleCollapse',
  'diff.markViewed',
  'diff.openFileBrowser',
  'detail.inlineReply',
]);

const SEARCH_IDS = new Set([
  'list.search',
  'list.filter',
  'filter.toggle',
  'global.command',
  'global.help',
  'global.search',
  'global.settings',
]);

function buildSections(
  bindings: KeyBinding[],
  sourceRoute: RouteBase,
): ShortcutSection[] {
  // Cmd+Q is the macOS quit key; Alt+F4 is the Windows/Linux one.
  const platformQuitId =
    detectPlatform() === 'mac' ? 'global.quit.win' : 'global.quit.mac';
  // Only show route-active bindings usable from where the user opened the
  // overlay. Skip bindings that are exclusively overlay-only (they have
  // overlays but no routes) — these are for the filter/command menus.
  const visible = bindings.filter(
    (b) =>
      b.hidden !== true &&
      b.actionId !== platformQuitId &&
      // Overlay-only bindings (have overlays but no routes) are excluded.
      !(
        b.activation.overlays !== undefined && b.activation.routes === undefined
      ) &&
      // Must be active on the current source route (or unrestricted).
      (b.activation.routes === undefined ||
        b.activation.routes.includes(sourceRoute)),
  );

  const navBindings = visible.filter((b) => NAVIGATION_IDS.has(b.actionId));
  const diffBindings = visible.filter((b) => DIFF_IDS.has(b.actionId));
  const actionBindings = visible.filter((b) => ACTION_IDS.has(b.actionId));
  const searchBindings = visible.filter((b) => SEARCH_IDS.has(b.actionId));
  const globalBindings = visible.filter(
    (b) =>
      !NAVIGATION_IDS.has(b.actionId) &&
      !DIFF_IDS.has(b.actionId) &&
      !ACTION_IDS.has(b.actionId) &&
      !SEARCH_IDS.has(b.actionId),
  );

  const sections: ShortcutSection[] = [
    { title: 'Navigation', shortcuts: navBindings.map(toRow) },
    { title: 'Diff review', shortcuts: diffBindings.map(toRow) },
    { title: 'Actions', shortcuts: actionBindings.map(toRow) },
    { title: 'Search & Commands', shortcuts: searchBindings.map(toRow) },
  ];

  if (globalBindings.length > 0) {
    sections.push({ title: 'Global', shortcuts: globalBindings.map(toRow) });
  }

  return sections.filter((s) => s.shortcuts.length > 0);
}

export function HelpOverlay(): React.ReactElement | null {
  const { activeOverlay, helpSourceRoute, closeOverlay } = useOverlays();
  const isOpen = activeOverlay === 'help';
  const sections = buildSections(useEffectiveBindings(), helpSourceRoute);

  const handleClose = (): void => {
    closeOverlay();
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      {/* Grid rows: header takes its height, body gets the rest and scrolls. */}
      <DialogContent className="max-w-2xl max-h-[80vh] grid-rows-[auto_minmax(0,1fr)] overflow-hidden">
        <DialogHeader>
          <DialogTitle>Keyboard Shortcuts</DialogTitle>
        </DialogHeader>

        <ScrollArea className="min-h-0 overflow-y-auto pr-4">
          <div className="space-y-6">
            {sections.map((section) => (
              <section key={section.title}>
                <h3 className="mb-3 font-mono text-[11px] font-bold uppercase tracking-widest text-outline">
                  {section.title}
                </h3>
                <div className="space-y-2 text-sm">
                  {section.shortcuts.map((shortcut) => (
                    <div key={shortcut.key} className="flex justify-between">
                      <Kbd>{shortcut.key}</Kbd>
                      <span className="text-on-surface">
                        {shortcut.description}
                      </span>
                    </div>
                  ))}
                </div>
              </section>
            ))}

            <div className="mt-4 border-t border-outline-variant pt-4 text-center text-xs text-on-surface-variant">
              Press <Kbd>Esc</Kbd> to close
            </div>
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
