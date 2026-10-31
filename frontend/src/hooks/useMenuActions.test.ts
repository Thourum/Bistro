// @vitest-environment jsdom
import { okAsync } from '@thourum/fault';
import { act, createElement, StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import type { OverlayContextValue } from '../overlays/OverlayContext';
import { reportMenuActions } from '../services/menu';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import type { KeyActionContext } from '../utils/keybindingActions';
import type { BindingActivationContext } from '../utils/keybindingActivation';
import { useMenuActions } from './useMenuActions';

vi.mock('../router', () => ({ router: { navigate: vi.fn() } }));
vi.mock('../services/menu', () => ({ reportMenuActions: vi.fn() }));

afterEach(() => {
  appStore.setState(() => initialUIState);
  vi.unstubAllGlobals();
});

it('clears requests before ordered dispatch and never replays them on effect remount', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.mocked(reportMenuActions).mockImplementation(() => okAsync(undefined));
  const dispatched: string[] = [];
  const overlays: OverlayContextValue = {
    activeOverlay: null,
    helpSourceRoute: 'list',
    openCommand: () => {
      expect(appStore.state.menuActionRequests).toEqual([]);
      dispatched.push('command');
    },
    openSearch: () => dispatched.push('search'),
    openHelp: vi.fn(),
    openFilter: vi.fn(),
    toggleFilter: vi.fn(),
    openRepoPicker: vi.fn(),
    openAddRepo: vi.fn(),
    openThreads: vi.fn(),
    openLineComment: vi.fn(),
    closeOverlay: vi.fn(),
    closeSearch: vi.fn(),
  };
  const activation: BindingActivationContext = {
    routeBase: 'list',
    activeOverlay: null,
    sandboxPane: null,
  };
  const makeCtx = (): KeyActionContext => ({
    store: appStore,
    routeBase: activation.routeBase,
    activeOverlay: activation.activeOverlay,
    overlays,
    location: { pathname: '/prs' },
  });
  function MenuHost(): null {
    useMenuActions(activation, makeCtx);
    return null;
  }
  appStore.setState((state) => ({
    ...state,
    menuActionRequests: [
      { actionId: 'global.command', sequence: 1 },
      { actionId: 'global.search', sequence: 2 },
    ],
  }));
  const root = createRoot(document.createElement('div'));
  await act(async () =>
    root.render(createElement(StrictMode, null, createElement(MenuHost))),
  );
  expect(dispatched).toEqual(['command', 'search']);
  await act(async () => root.unmount());
});
