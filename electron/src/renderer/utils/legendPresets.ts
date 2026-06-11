import { formatForDisplay } from '@tanstack/react-hotkeys';
import {
  type AppMode,
  isSequenceBinding,
  type KeyBinding,
} from './keybindings';

/**
 * Per-mode footer legend preset.
 *
 * Each entry is an ordered list of `actionId`s. The footer looks up the
 * effective binding for each id (respecting user overrides) and renders it as
 * `<key> <description>`.
 *
 * Keep this list short — roughly 3–5 entries per mode — so it stays readable
 * in the 32px-tall footer. Less common bindings are reachable via the `?`
 * help overlay.
 */
export const FOOTER_LEGEND_PRESETS: Record<AppMode, string[]> = {
  login: [],
  onboarding: [],
  list: ['list.down', 'list.up', 'list.open', 'list.filter', 'list.search'],
  detail: [
    'detail.nextFile',
    'detail.prevFile',
    'detail.approve',
    'detail.comment',
    'global.escape',
  ],
  diff: [
    'detail.scrollDown',
    'detail.scrollUp',
    'detail.nextFile',
    'detail.prevFile',
    'global.escape',
  ],
  search: ['global.escape'],
  filter: ['filter.open', 'filter.draft', 'filter.all', 'filter.close'],
  help: ['global.escape'],
  command: ['global.escape'],
  settings: ['global.escape'],
};

export interface FooterLegendEntry {
  actionId: string;
  /** Display label for the key combo, already prettified for the footer. */
  label: string;
  description: string;
}

/**
 * Pretty-print a binding's keys via TanStack Hotkeys' platform-aware
 * formatter. Sequences render their formatted steps joined by a space.
 */
export function formatBindingKeys(binding: KeyBinding): string {
  if (isSequenceBinding(binding)) {
    return binding.keys.map((step) => formatForDisplay(step)).join(' ');
  }
  return formatForDisplay(binding.keys);
}

/**
 * Build the ordered legend entries for the given mode using the effective
 * (user-overridden) bindings.
 */
export function buildLegendForMode(
  mode: AppMode,
  bindings: KeyBinding[],
): FooterLegendEntry[] {
  const preset = FOOTER_LEGEND_PRESETS[mode];
  if (!preset || preset.length === 0) return [];
  const byId = new Map(bindings.map((b) => [b.actionId, b]));
  const entries: FooterLegendEntry[] = [];
  for (const id of preset) {
    const binding = byId.get(id);
    if (!binding) continue;
    if (binding.keys.length === 0) continue;
    entries.push({
      actionId: id,
      label: formatBindingKeys(binding),
      description: binding.description,
    });
  }
  return entries;
}
