import {
  matchesKeyboardEvent,
  normalizeHotkey,
  parseHotkey,
  validateHotkey,
} from '@tanstack/hotkeys';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({ router: { navigate: vi.fn() } }));

import { NAVIGATION_STYLES } from '../../types/settings';
import type { OverlayKind } from '../overlays/OverlayContext';
import {
  ACTION_CATALOG,
  getEffectiveBindings,
  isBindingEnabled,
  type RouteBase,
} from './keybindings';
import { KEYMAPS, SEQUENCE_TIMEOUT_MS } from './keymaps';

const PLATFORMS = ['mac', 'windows', 'linux'] as const;
const ROUTES: readonly RouteBase[] = [
  'list',
  'detail',
  'diff',
  'settings',
  'login',
  'onboarding',
];
const OVERLAYS: readonly OverlayKind[] = [
  'command',
  'help',
  'search',
  'filter',
  'repoPicker',
  'addRepo',
  'threads',
];
const ONBOARDING_CONTEXTS = [
  {
    routeBase: 'onboarding' as const,
    activeOverlay: null,
    sandboxPane: null,
    onboardingStep: 'sync' as const,
    isScanning: false,
  },
  {
    routeBase: 'onboarding' as const,
    activeOverlay: null,
    sandboxPane: null,
    onboardingStep: 'sync' as const,
    isScanning: true,
  },
  ...(['list', 'detail', 'diff'] as const).flatMap((sandboxPane) =>
    [false, true].map((isPracticeComplete) => ({
      routeBase: 'onboarding' as const,
      activeOverlay: null,
      sandboxPane,
      onboardingStep: 'practice' as const,
      isPracticeComplete,
    })),
  ),
  ...[false, true].map((hasListEscapeWork) => ({
    routeBase: 'list' as const,
    activeOverlay: null,
    sandboxPane: null,
    isFirstRunRailVisible: true,
    hasListEscapeWork,
  })),
];
const CONTEXTS = [
  ...ROUTES.map((routeBase) => ({
    routeBase,
    activeOverlay: null,
    sandboxPane: null,
  })),
  ...OVERLAYS.map((activeOverlay) => ({
    routeBase: 'list' as const,
    activeOverlay,
    sandboxPane: null,
  })),
  ...ONBOARDING_CONTEXTS,
  ...(['idle', 'waiting'] as const).map((screen) => ({
    routeBase: 'login' as const,
    activeOverlay: null,
    sandboxPane: null,
    screen,
    isSpawned: true,
    isConnected: true,
  })),
  {
    routeBase: 'connecting' as const,
    activeOverlay: null,
    sandboxPane: null,
    screen: 'crashed' as const,
    isSpawned: true,
    isConnected: true,
  },
];
/** Pairs that intentionally share a key in one context. */
const ALLOWED_SHARED = new Set(['filter.close|global.escape']);

const steps = (keys: string | readonly string[]): readonly string[] =>
  typeof keys === 'string' ? [keys] : keys;

function assertUnambiguousKeys(
  byKey: ReadonlyMap<string, readonly string[]>,
  contextLabel: string,
): void {
  for (const [key, ids] of byKey) {
    const pair = [...ids].sort().join('|');
    if (ids.length > 1 && !ALLOWED_SHARED.has(pair)) {
      expect.fail(`${contextLabel}: "${key}" → ${pair}`);
    }
    for (const other of byKey.keys()) {
      if (other !== key && other.startsWith(`${key} `)) {
        expect.fail(`${contextLabel}: "${key}" is a prefix of "${other}"`);
      }
    }
  }
}

describe.each(NAVIGATION_STYLES)('%s keymap', (style) => {
  it('only maps actions that exist in the catalog', () => {
    const known = new Set(
      ACTION_CATALOG.map((definition) => definition.actionId),
    );
    expect(Object.keys(KEYMAPS[style]).filter((id) => !known.has(id))).toEqual(
      [],
    );
  });

  it('uses only valid hotkey strings', () => {
    for (const keys of Object.values(KEYMAPS[style])) {
      for (const step of steps(keys)) {
        expect(validateHotkey(step).errors, step).toEqual([]);
      }
    }
  });

  it.each(PLATFORMS)(
    'has no ambiguous keys in any context on %s',
    (platform) => {
      const effectiveBindings = getEffectiveBindings(style, {});
      const continueKeys = effectiveBindings.find(
        (binding) => binding.actionId === 'onboarding.continue',
      )?.keys;
      for (const context of CONTEXTS) {
        const bindings = effectiveBindings.filter((binding) =>
          isBindingEnabled(binding, { ...context, continueKeys }),
        );
        const byKey = new Map<string, string[]>();
        for (const binding of bindings) {
          const key = steps(binding.keys)
            .map((step) => normalizeHotkey(step, platform))
            .join(' ');
          byKey.set(key, [...(byKey.get(key) ?? []), binding.actionId]);
        }
        assertUnambiguousKeys(
          byKey,
          `${style}/${platform} ${JSON.stringify(context)}`,
        );
      }
    },
  );
});

describe('style resolution', () => {
  it('resolves emacs movement, prefix actions and cancel aliases', () => {
    const bindings = getEffectiveBindings('emacs', {});
    const keysOf = (id: string) =>
      bindings.find((binding) => binding.actionId === id)?.keys;
    expect(keysOf('list.down')).toBe('Control+N');
    expect(keysOf('global.command')).toBe('Alt+X');
    expect(keysOf('detail.dismissNits')).toEqual([
      'Control+C',
      'Control+D',
      'n',
    ]);
    expect(keysOf('global.cancel')).toBe('Control+G');
    expect(keysOf('detail.undoDismissAlt')).toEqual(['Control+X', 'u']);
    expect(SEQUENCE_TIMEOUT_MS.emacs).toBe(1500);
  });

  it('resolves standard desktop movement without vim movement or emacs aliases', () => {
    const bindings = getEffectiveBindings('standard', {});
    const keysOf = (id: string) =>
      bindings.find((binding) => binding.actionId === id)?.keys;
    expect(keysOf('list.downArrow')).toBe('ArrowDown');
    expect(keysOf('list.top')).toBe('Home');
    expect(keysOf('diff.nextFile')).toBe('Control+Tab');
    expect(keysOf('detail.approve')).toBe('a');
    expect(keysOf('detail.undoDismiss')).toBe('Mod+Z');
    for (const id of [
      'list.down',
      'list.up',
      'detail.scrollLeft',
      'detail.scrollRight',
      'detail.scrollUp',
      'detail.scrollDown',
      'global.cancel',
      'detail.undoDismissAlt',
    ]) {
      expect(keysOf(id), id).toBeUndefined();
    }
  });

  it('keeps aliases hidden and active in the same contexts as their originals', () => {
    for (const [aliasId, originalId] of [
      ['global.cancel', 'global.escape'],
      ['detail.undoDismissAlt', 'detail.undoDismiss'],
    ]) {
      const alias = ACTION_CATALOG.find(
        (binding) => binding.actionId === aliasId,
      );
      const original = ACTION_CATALOG.find(
        (binding) => binding.actionId === originalId,
      );
      expect(alias, aliasId).toBeDefined();
      expect(alias?.hidden, aliasId).toBe(true);
      expect(alias?.description, aliasId).toBe(original?.description);
      expect(alias?.activation, aliasId).toEqual(original?.activation);
    }
    for (const style of ['vim', 'standard'] as const) {
      const actionIds = getEffectiveBindings(style, {}).map(
        (binding) => binding.actionId,
      );
      expect(actionIds).not.toContain('global.cancel');
      expect(actionIds).not.toContain('detail.undoDismissAlt');
    }
  });
});

describe('emacs Meta bindings on macOS', () => {
  // Option rewrites event.key (Alt+X → "≈"); TanStack must fall back to event.code.
  const CODE_BY_KEY: Readonly<Record<string, string>> = {
    ',': 'Comma',
    '.': 'Period',
    '/': 'Slash',
  };
  const codeFor = (key: string): string =>
    CODE_BY_KEY[key] ??
    (/^\d$/.test(key) ? `Digit${key}` : `Key${key.toUpperCase()}`);

  it('matches every Alt+ step from its physical key', () => {
    for (const keys of Object.values(KEYMAPS.emacs)) {
      for (const step of steps(keys).filter((key) => key.includes('Alt+'))) {
        const parts = step.split('+');
        const key = parts[parts.length - 1] ?? '';
        // Function keys do not produce Option-transformed printable characters.
        const isFunctionKey = /^F\d+$/.test(key);
        const event = {
          type: 'keydown',
          key: isFunctionKey ? key : '∆',
          code: isFunctionKey ? key : codeFor(key),
          altKey: true,
          shiftKey: parts.includes('Shift'),
          ctrlKey: parts.includes('Control'),
          metaKey: false,
          isComposing: false,
        } as unknown as KeyboardEvent;
        expect(
          matchesKeyboardEvent(event, parseHotkey(step, 'mac'), 'mac'),
          step,
        ).toBe(true);
      }
    }
  });
});
