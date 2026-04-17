import type { AppMode, BindingDefinition } from './keyboard';

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

/** Pretty-print a binding's keys into a compact footer label (e.g. `ctrl+d` → `^D`). */
function formatKeys(keys: string[]): string {
  if (keys.length === 0) return '';
  if (keys.length > 1 && keys.every((k) => k.length === 1)) {
    // Multi-tap sequence, e.g. ['g','g'] → `GG`, ['a','a'] → `AA`.
    return keys.map((k) => k.toUpperCase()).join('');
  }
  // Single key (or modifier combo).
  const key = keys[0];
  if (!key) return '';
  return key
    .replace(/meta\+/gi, '⌘')
    .replace(/ctrl\+/gi, '^')
    .replace(/shift\+/gi, '⇧')
    .replace(/alt\+/gi, '⌥')
    .replace(/^ $/, 'SPACE')
    .replace(/^Enter$/i, 'ENTER')
    .replace(/^Escape$/i, 'ESC')
    .toUpperCase();
}

/**
 * Build the ordered legend entries for the given mode using the effective
 * (user-overridden) bindings.
 */
export function buildLegendForMode(
  mode: AppMode,
  bindings: BindingDefinition[],
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
      label: formatKeys(binding.keys),
      description: binding.description,
    });
  }
  return entries;
}
