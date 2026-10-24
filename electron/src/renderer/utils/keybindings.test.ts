import { Store } from '@tanstack/store';
import { assert, describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import type { OverlayContextValue } from '../overlays/OverlayContext';
import { initialUIState } from '../store/ui';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import {
  hotkeyOptions,
  isBindingEnabled,
  isEscape,
} from './keybindingActivation';
import {
  ACTION_CATALOG,
  getEffectiveBindings,
  isActive,
  type KeyBinding,
} from './keybindings';
import type { SandboxPane } from './onboardingSandbox';

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

describe('hotkeyOptions', () => {
  it('suppresses cancel while typing and preserves Escape-only event handling', () => {
    const cancel = { actionId: 'global.cancel', keys: 'Control+G' };
    expect(hotkeyOptions(cancel)).toEqual({ ignoreInputs: true });
    expect(isEscape(cancel)).toBe(false);
    expect(hotkeyOptions(binding('global.escape'))).toEqual({
      ignoreInputs: false,
      preventDefault: false,
    });
    expect(hotkeyOptions(binding('list.down'))).toEqual({ ignoreInputs: true });
    expect(hotkeyOptions(binding('onboarding.continue'))).toEqual({
      ignoreInputs: true,
      preventDefault: false,
    });
  });
});

describe('isBindingEnabled on the onboarding route', () => {
  const onboarding = (sandboxPane: SandboxPane | null) => ({
    routeBase: 'onboarding' as const,
    activeOverlay: null,
    sandboxPane,
  });

  it('activates sandbox actions by the sandbox pane, not the real route', () => {
    expect(isBindingEnabled(binding('list.down'), onboarding('list'))).toBe(
      true,
    );
    expect(isBindingEnabled(binding('list.down'), onboarding('detail'))).toBe(
      false,
    );
    expect(isBindingEnabled(binding('diff.nextFile'), onboarding('diff'))).toBe(
      true,
    );
    expect(isBindingEnabled(binding('list.mute'), onboarding('list'))).toBe(
      false,
    );
    expect(isBindingEnabled(binding('list.down'), onboarding(null))).toBe(
      false,
    );
  });

  it('only lets Space continue from the sandbox list pane', () => {
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding('list')),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding('detail')),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding('diff')),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding(null)),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('detail.viewDiff'), onboarding('detail')),
    ).toBe(true);
  });

  it('keeps overlay precedence and other routes unchanged', () => {
    expect(
      isBindingEnabled(binding('list.down'), {
        ...onboarding('list'),
        activeOverlay: 'command',
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('global.escape'), {
        ...onboarding('detail'),
        activeOverlay: 'command',
      }),
    ).toBe(true);
    for (const routeBase of [
      'list',
      'detail',
      'diff',
      'settings',
      'login',
      null,
    ] as const) {
      const context = { routeBase, activeOverlay: null };
      // First-run actions are additionally gated on the derived screen.
      const routeBindings = getEffectiveBindings('vim', {}).filter(
        ({ actionId }) =>
          !actionId.startsWith('login.') && !actionId.startsWith('connecting.'),
      );
      for (const configuredBinding of routeBindings) {
        expect(
          isBindingEnabled(configuredBinding, {
            ...context,
            sandboxPane: 'diff',
          }),
        ).toBe(isActive(configuredBinding, context));
      }
    }
  });

  it('enables style cycling only while the sandbox step is mounted', () => {
    for (const actionId of ['onboarding.nextStyle', 'onboarding.prevStyle']) {
      const styleBinding = binding(actionId);
      expect(isBindingEnabled(styleBinding, onboarding(null))).toBe(false);
      expect(isBindingEnabled(styleBinding, onboarding('list'))).toBe(true);
      expect(isBindingEnabled(styleBinding, onboarding('detail'))).toBe(true);
    }
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
