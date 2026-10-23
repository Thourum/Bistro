import { Store } from '@tanstack/store';
import { vi } from 'vitest';
import type { OverlayContextValue } from '../../src/renderer/overlays/OverlayContext';
import type { UIState } from '../../src/renderer/store/ui';
import { initialUIState } from '../../src/renderer/store/ui';
import {
  getBindingAction,
  type KeyActionContext,
} from '../../src/renderer/utils/keybindingActions';
import type {
  KeyBinding,
  RouteBase,
} from '../../src/renderer/utils/keybindings';
import { DEFAULT_KEYBINDINGS } from '../../src/renderer/utils/keybindings';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

export function makeStore(overrides: Partial<UIState> = {}): Store<UIState> {
  return new Store<UIState>({ ...initialUIState, ...overrides });
}

/** Create a minimal KeyActionContext for testing binding actions. */
export function makeCtx(
  storeOrOverrides: Store<UIState> | Partial<UIState> = {},
  routeBase: RouteBase | null = 'list',
  overlayOverrides: Partial<OverlayContextValue> = {},
  prId = 'pr-test-id',
): KeyActionContext {
  const store =
    storeOrOverrides instanceof Store
      ? storeOrOverrides
      : makeStore(storeOrOverrides);
  const overlays: OverlayContextValue = {
    activeOverlay: null,
    helpSourceRoute: 'list',
    openCommand: vi.fn(),
    openHelp: vi.fn(),
    openSearch: vi.fn(),
    openFilter: vi.fn(),
    toggleFilter: vi.fn(),
    openRepoPicker: vi.fn(),
    openThreads: vi.fn(),
    closeOverlay: vi.fn(),
    closeSearch: vi.fn(),
    ...overlayOverrides,
  };
  return {
    store,
    routeBase,
    activeOverlay: overlays.activeOverlay,
    overlays,
    location: { pathname: '/prs' },
    prId,
  };
}

export function byId(actionId: string): KeyBinding {
  const binding = DEFAULT_KEYBINDINGS.find(
    (candidate) => candidate.actionId === actionId,
  );
  if (binding === undefined)
    throw new Error(`no binding with actionId ${actionId}`);
  return binding;
}

export function invokeAction(
  actionId: string,
  context: KeyActionContext,
): void {
  const action = getBindingAction(actionId);
  if (action === undefined)
    throw new Error(`no action for binding ${actionId}`);
  action(context);
}
