// @vitest-environment jsdom
import { Store } from '@tanstack/store';
import { describe, expect, it, vi } from 'vitest';
import { COMMANDS } from '../components/commandPaletteCommands';
import type { OverlayContextValue } from '../overlays/OverlayContext';
import { router } from '../router';
import { initialUIState } from '../store/ui';
import type { KeyActionContext } from './keybindingActions';
import { isActive } from './keybindingActivation';
import { ACTION_CATALOG, getEffectiveBindings } from './keybindings';
import { enabledMenuActions, runMenuAction } from './menuActions';

vi.mock('../router', () => ({ router: { navigate: vi.fn() } }));

function actionContext(): KeyActionContext {
  return {
    store: new Store(initialUIState),
    routeBase: 'list',
    activeOverlay: null,
    overlays: {
      activeOverlay: null,
      helpSourceRoute: 'list',
      openCommand: vi.fn(),
      openHelp: vi.fn(),
      openSearch: vi.fn(),
      openFilter: vi.fn(),
      toggleFilter: vi.fn(),
      openRepoPicker: vi.fn(),
      openAddRepo: vi.fn(),
      openThreads: vi.fn(),
      openLineComment: vi.fn(),
      closeOverlay: vi.fn(),
      closeSearch: vi.fn(),
    } satisfies OverlayContextValue,
    location: { pathname: '/prs' },
  };
}

describe('menu action availability', () => {
  it('enables only actions the current route activates', () => {
    expect(
      enabledMenuActions({ routeBase: 'list', activeOverlay: null }),
    ).toEqual([
      'global.settings',
      'global.inbox',
      'global.search',
      'global.command',
      'global.help',
    ]);
    expect(
      enabledMenuActions({ routeBase: 'diff', activeOverlay: null }),
    ).toEqual(['global.inbox', 'global.command', 'global.help']);
    expect(
      enabledMenuActions({ routeBase: 'settings', activeOverlay: null }),
    ).toEqual(['global.inbox']);
  });

  it('disables every menu action while signed out or an overlay owns input', () => {
    expect(
      enabledMenuActions({ routeBase: 'login', activeOverlay: null }),
    ).toEqual([]);
    expect(
      enabledMenuActions({ routeBase: 'list', activeOverlay: 'command' }),
    ).toEqual([]);
  });

  it('rejects a stale signed-out request without running its handler', () => {
    const context = actionContext();
    expect(
      runMenuAction('global.command', { ...context, routeBase: 'login' }),
    ).toBe(false);
    expect(context.overlays.openCommand).not.toHaveBeenCalled();
    expect(runMenuAction('global.command', context)).toBe(true);
    expect(context.overlays.openCommand).toHaveBeenCalledOnce();
    expect(runMenuAction('list.down', context)).toBe(false);
    expect(context.store.state.selectedPRIndex).toBe(
      initialUIState.selectedPRIndex,
    );
  });

  it('lists Go to Inbox in the palette without assigning a default key', () => {
    const context = actionContext();
    const commands = COMMANDS.filter((command) => {
      const definition = ACTION_CATALOG.find(
        (entry) => entry.actionId === command.actionId,
      );
      return definition !== undefined && isActive(definition, context);
    });
    expect(commands).toContainEqual({
      id: 'inbox',
      name: 'Go to Inbox',
      icon: 'inbox',
      actionId: 'global.inbox',
    });
    for (const style of ['vim', 'emacs', 'standard'] as const) {
      expect(
        getEffectiveBindings(style, {}).some(
          (binding) => binding.actionId === 'global.inbox',
        ),
      ).toBe(false);
    }
    expect(runMenuAction('global.inbox', context)).toBe(true);
    expect(router.navigate).toHaveBeenCalledExactlyOnceWith({ to: '/prs' });
  });
});
