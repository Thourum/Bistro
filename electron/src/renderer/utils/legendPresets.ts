import {
  type FormatDisplayOptions,
  formatForDisplay,
} from '@tanstack/react-hotkeys';
import type { OverlayKind } from '../overlays/OverlayContext';
import type { KeyBinding, RouteBase } from './keybindings';

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
const FOOTER_LEGEND_PRESETS_BY_ROUTE = {
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
    'diff.nextFile',
    'diff.prevFile',
    'diff.markViewed',
    'diff.toggleStyle',
    'global.escape',
  ],
  settings: ['global.escape'],
  // Login and onboarding footers show no legend (empty chrome).
  login: [],
  onboarding: [],
  connecting: [],
} as const satisfies Record<RouteBase, readonly string[]>;

/** Per-overlay footer legend preset. */
const FOOTER_LEGEND_PRESETS_BY_OVERLAY = {
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
    'filter.touchesYou',
    'filter.open',
    'filter.draft',
    'filter.all',
    'filter.close',
  ],
  help: ['global.escape'],
  command: ['global.escape'],
  repoPicker: ['global.escape'],
  addRepo: ['global.escape'],
  tour: ['global.escape'],
} as const satisfies Record<OverlayKind, readonly string[]>;

export interface FooterLegendEntry {
  actionId: string;
  /** Display label for the key combo, already prettified for the footer. */
  label: string;
  description: string;
}

/**
 * US-layout character each shifted punctuation key produces. Shift is
 * dropped in the label because the character already implies it.
 */
const SHIFTED_PUNCTUATION_CHARS: Readonly<Record<string, string>> = {
  '`': '~',
  '1': '!',
  '2': '@',
  '3': '#',
  '4': '$',
  '5': '%',
  '6': '^',
  '7': '&',
  '8': '*',
  '9': '(',
  '0': ')',
  '-': '_',
  '=': '+',
  '[': '{',
  ']': '}',
  '\\': '|',
  ';': ':',
  "'": '"',
  ',': '<',
  '.': '>',
  '/': '?',
};

/** Rewrites `Shift+/` as `?`; other hotkeys pass through unchanged. */
function toShiftedCharacterHotkey(hotkey: string): string {
  const parts = hotkey.split('+');
  const key = parts[parts.length - 1] ?? '';
  const modifiers = parts.slice(0, -1);
  const shiftedChar = SHIFTED_PUNCTUATION_CHARS[key];
  const hasShift = modifiers.some((m) => m.toLowerCase() === 'shift');
  if (shiftedChar === undefined || !hasShift) return hotkey;
  return [
    ...modifiers.filter((m) => m.toLowerCase() !== 'shift'),
    shiftedChar,
  ].join('+');
}

/**
 * Pretty-print a binding's keys via TanStack Hotkeys' platform-aware
 * formatter. Sequences render their formatted steps joined by a space.
 */
export function formatBindingKeys(
  binding: KeyBinding,
  options?: FormatDisplayOptions & { parts?: false },
): string {
  if (typeof binding.keys !== 'string') {
    return binding.keys
      .map((step) => formatForDisplay(toShiftedCharacterHotkey(step), options))
      .join(' ');
  }
  return formatForDisplay(toShiftedCharacterHotkey(binding.keys), options);
}

/** Keys of the first of `actionIds` that `bindings` binds, or undefined. */
export function findActionKeys(
  bindings: readonly KeyBinding[],
  actionIds: readonly string[],
): string | undefined {
  const binding = actionIds
    .map((id) => bindings.find((candidate) => candidate.actionId === id))
    .find((found) => found !== undefined && found.keys.length > 0);
  return binding === undefined ? undefined : formatBindingKeys(binding);
}

/** Keys of the first of `actionIds` that `bindings` binds, or `—` if none. */
export function formatActionKeys(
  bindings: readonly KeyBinding[],
  actionIds: readonly string[],
): string {
  return findActionKeys(bindings, actionIds) ?? '—';
}

/**
 * Build the ordered legend entries for the given route/overlay context
 * using the effective (user-overridden) bindings.
 */
export function buildLegendForRouteBase(
  routeBase: RouteBase | null,
  activeOverlay: OverlayKind | null,
  bindings: readonly KeyBinding[],
): FooterLegendEntry[] {
  const preset =
    activeOverlay !== null
      ? FOOTER_LEGEND_PRESETS_BY_OVERLAY[activeOverlay]
      : routeBase !== null
        ? FOOTER_LEGEND_PRESETS_BY_ROUTE[routeBase]
        : [];
  if (preset.length === 0) return [];

  const byId = new Map(bindings.map((binding) => [binding.actionId, binding]));
  return preset.flatMap((actionId) => {
    const binding = byId.get(actionId);
    return binding === undefined || binding.keys.length === 0
      ? []
      : [
          {
            actionId,
            label: formatBindingKeys(binding),
            description: binding.description,
          },
        ];
  });
}
