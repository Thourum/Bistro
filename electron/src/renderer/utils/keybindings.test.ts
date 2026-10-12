import { Store } from '@tanstack/store';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import type { OverlayContextValue } from '../overlays/OverlayContext';
import { initialUIState } from '../store/ui';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import { DEFAULT_KEYBINDINGS, isActive, type KeyBinding } from './keybindings';

function binding(actionId: string): KeyBinding {
  const found = DEFAULT_KEYBINDINGS.find(
    (candidate) => candidate.actionId === actionId,
  );
  if (found === undefined) {
    throw new Error(`Expected binding ${actionId}`);
  }
  return found;
}

describe('inbox keybindings', () => {
  it('resolves an action for every configured binding', () => {
    for (const configuredBinding of DEFAULT_KEYBINDINGS) {
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
