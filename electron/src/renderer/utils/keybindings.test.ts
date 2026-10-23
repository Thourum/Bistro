import { Store } from '@tanstack/store';
import { assert, describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import type { OverlayContextValue } from '../overlays/OverlayContext';
import { initialUIState } from '../store/ui';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import {
  ACTION_CATALOG,
  getEffectiveBindings,
  isActive,
  type KeyBinding,
} from './keybindings';

function binding(actionId: string): KeyBinding {
  const found = getEffectiveBindings('vim', {}).find(
    (candidate) => candidate.actionId === actionId,
  );
  assert(found !== undefined, `Expected binding ${actionId}`);
  return found;
}

describe('getEffectiveBindings', () => {
  it('lets a user override beat the style table and an empty override unbind', () => {
    const bindings = getEffectiveBindings('vim', {
      'list.down': ['x'],
      'list.mute': [],
      'list.top': ['g', 'h'],
    });
    const keysOf = (id: string) =>
      bindings.find((b) => b.actionId === id)?.keys;
    expect(keysOf('list.down')).toBe('x');
    expect(keysOf('list.top')).toEqual(['g', 'h']);
    expect(keysOf('list.mute')).toBeUndefined();
    expect(keysOf('list.up')).toBe('k');
  });
});

describe('inbox keybindings', () => {
  it('resolves an action for every configured binding', () => {
    for (const configuredBinding of ACTION_CATALOG) {
      expect(
        getBindingAction(configuredBinding.actionId),
        configuredBinding.actionId,
      ).toBeTypeOf('function');
    }
  });

  it('defines lane jumps and hides repetitive help entries', () => {
    for (let lane = 1; lane <= 6; lane += 1) {
      const laneBinding = binding(`inbox.lane${lane}`);
      expect(laneBinding.keys).toEqual(['g', String(lane)]);
      expect(laneBinding.hidden).toBe(lane !== 1);
    }
  });

  it('keeps list cursor navigation clamped to visible PRs', () => {
    const store = new Store({
      ...initialUIState,
      selectedPRIndex: 2,
      visiblePRIds: ['1', '2', '3'],
    });
    const context: KeyActionContext = {
      store,
      routeBase: 'list',
      activeOverlay: null,
      overlays: {} as OverlayContextValue,
      location: { pathname: '/prs' },
    };

    getBindingAction('list.down')?.(context);
    expect(store.state.selectedPRIndex).toBe(2);
    getBindingAction('list.up')?.(context);
    expect(store.state.selectedPRIndex).toBe(1);
    getBindingAction('list.pageDown')?.(context);
    expect(store.state.selectedPRIndex).toBe(2);
  });

  it('keeps Shift-R route-scoped between inbox and detail actions', () => {
    const inboxMarkRead = binding('inbox.markLaneRead');
    const detailRequestChanges = binding('detail.requestChanges');
    const withoutOverlay = { activeOverlay: null } as const;

    expect(
      isActive(inboxMarkRead, { ...withoutOverlay, routeBase: 'list' }),
    ).toBe(true);
    expect(
      isActive(inboxMarkRead, { ...withoutOverlay, routeBase: 'detail' }),
    ).toBe(false);
    expect(
      isActive(detailRequestChanges, {
        ...withoutOverlay,
        routeBase: 'detail',
      }),
    ).toBe(true);
    expect(
      isActive(detailRequestChanges, { ...withoutOverlay, routeBase: 'list' }),
    ).toBe(false);
  });
});
