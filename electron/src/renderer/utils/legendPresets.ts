import { formatForDisplay } from '@tanstack/react-hotkeys';
import type { OverlayKind } from '../overlays/OverlayContext';
import {
  isSequenceBinding,
  type KeyBinding,
  type RouteBase,
} from './keybindings';

/**
 * Per-route footer legend preset.
 *
 * Each entry is an ordered list of `actionId`s. The footer looks up the
 * effective binding for each id (respecting user overrides) and renders it as
 * `<key> <description>`.
 *
 * Keep this list short — roughly 3–5 entries per route — so it stays readable
 * in the 32px-tall footer. Less common bindings are reachable via the `?`
 * help overlay.
 */
const FOOTER_LEGEND_PRESETS_BY_ROUTE: Record<RouteBase, string[]> = {
  list: ['list.down', 'list.up', 'list.open', 'list.filter', 'list.search'],
  detail: [
    'detail.nextFile',
    'detail.prevFile',
    'detail.approve',
    'detail.comment',
    'detail.requestChanges',
    'global.escape',
  ],
  diff: [
    'detail.scrollDown',
    'detail.scrollUp',
    'detail.nextFile',
    'detail.prevFile',
    'global.escape',
  ],
  settings: ['global.escape'],
  // Login and onboarding footers show no legend (empty chrome).
  login: [],
  onboarding: [],
};

/** Per-overlay footer legend preset. */
const FOOTER_LEGEND_PRESETS_BY_OVERLAY: Record<OverlayKind, string[]> = {
  threads: [
    'threads.next',
    'threads.prev',
    'threads.resolve',
    'threads.unresolve',
    'global.escape',
  ],
  search: ['global.escape'],
  filter: [
    'filter.needsAction',
    'filter.open',
    'filter.draft',
    'filter.all',
    'filter.close',
  ],
  help: ['global.escape'],
  command: ['global.escape'],
  repoPicker: ['global.escape'],
  addRepo: ['global.escape'],
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
export function formatBindingKeys(
  binding: KeyBinding,
  options?: Parameters<typeof formatForDisplay>[1],
): string {
  if (isSequenceBinding(binding)) {
    return binding.keys
      .map((step) => formatForDisplay(step, options))
      .join(' ');
  }
  return formatForDisplay(binding.keys as string, options);
}

/**
 * Build the ordered legend entries for the given route/overlay context
 * using the effective (user-overridden) bindings.
 */
export function buildLegendForRouteBase(
  routeBase: RouteBase | null,
  activeOverlay: OverlayKind | null,
  bindings: KeyBinding[],
): FooterLegendEntry[] {
  // When an overlay is active, show overlay-specific legend.
  if (activeOverlay !== null) {
    const preset = FOOTER_LEGEND_PRESETS_BY_OVERLAY[activeOverlay];
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

  if (routeBase === null) return [];
  const preset = FOOTER_LEGEND_PRESETS_BY_ROUTE[routeBase];
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
