// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react';
import React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useAppHotkeys } from '../../src/renderer/hooks/useAppHotkeys';
import { OverlayProvider, useOverlays } from '../../src/renderer/overlays/OverlayContext';
import { appStore } from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import { initialUIState } from '../../src/renderer/store/ui';

// Mock the router so list.open can call router.navigate without a RouterProvider.
const mockRouterNavigate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('../../src/renderer/router', () => ({
  router: {
    navigate: mockRouterNavigate,
    state: { location: { pathname: '/prs' } },
  },
  prDetailRoute: {},
}));

// Mock @tanstack/react-router so useRouterState works without RouterProvider.
const mockPathname = { current: '/prs' };
vi.mock('@tanstack/react-router', () => ({
  useRouterState: vi.fn(() => ({ location: { pathname: mockPathname.current } })),
  useNavigate: vi.fn(() => vi.fn()),
}));

// Integration tests: real KeyboardEvents dispatched through the DOM, handled
// by TanStack Hotkeys (which listens on `document` by default), asserting
// appStore transitions and overlay context state.

function fireKey(
  key: string,
  opts: KeyboardEventInit = {},
  target: EventTarget = document.body,
): void {
  target.dispatchEvent(
    new KeyboardEvent('keydown', {
      key,
      bubbles: true,
      cancelable: true,
      ...opts,
    }),
  );
}

/** Wrapper that provides OverlayProvider context. */
function wrapper({ children }: { children: React.ReactNode }) {
  return React.createElement(OverlayProvider, null, children);
}

/** Renders useAppHotkeys + useOverlays so we can assert overlay state. */
function renderWithOverlays() {
  return renderHook(
    () => {
      useAppHotkeys();
      return useOverlays();
    },
    { wrapper },
  );
}

describe('useAppHotkeys', () => {
  beforeEach(() => {
    mockPathname.current = '/prs';
    appStore.setState(() => ({ ...initialUIState }));
    settingsStore.setState((s) => ({ ...s, keybindings: {} }));
    document.body.innerHTML = '';
  });

  it('mounts without throwing', () => {
    expect(() => renderWithOverlays()).not.toThrow();
  });

  it('moves selection down on "j" in list mode', () => {
    renderWithOverlays();
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(1);
  });

  it('does not move selection below zero on "k" from index 0', () => {
    renderWithOverlays();
    act(() => {
      fireKey('k', { code: 'KeyK' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('Enter on list route navigates to the PR by id', () => {
    // Seed a visible PR so list.open can navigate by id.
    mockRouterNavigate.mockClear();
    appStore.setState((s) => ({ ...s, visiblePRIds: ['pr-1'] }));
    renderWithOverlays();
    act(() => {
      fireKey('Enter', { code: 'Enter' });
    });
    // Navigation is router-owned; verify router.navigate was called.
    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs/$prId', params: { prId: 'pr-1' } }),
    );
  });

  it('opens search overlay on "/"', () => {
    const { result } = renderWithOverlays();
    act(() => {
      fireKey('/', { code: 'Slash' });
    });
    expect(result.current.activeOverlay).toBe('search');
  });

  it('toggles the filter overlay on "f"', () => {
    const { result } = renderWithOverlays();
    act(() => {
      fireKey('f', { code: 'KeyF' });
    });
    expect(result.current.activeOverlay).toBe('filter');
    act(() => {
      fireKey('f', { code: 'KeyF' });
    });
    expect(result.current.activeOverlay).toBeNull();
  });

  it('opens help overlay on "?" (Shift+/ physically)', () => {
    const { result } = renderWithOverlays();
    act(() => {
      fireKey('?', { code: 'Slash', shiftKey: true });
    });
    expect(result.current.activeOverlay).toBe('help');
    expect(result.current.helpSourceRoute).toBe('list');
  });

  it('opens command overlay on ":" (Shift+; physically)', () => {
    const { result } = renderWithOverlays();
    act(() => {
      fireKey(':', { code: 'Semicolon', shiftKey: true });
    });
    expect(result.current.activeOverlay).toBe('command');
  });

  it('moves selection with the arrow keys', () => {
    renderWithOverlays();
    act(() => {
      fireKey('ArrowDown', { code: 'ArrowDown' });
    });
    expect(appStore.state.selectedPRIndex).toBe(1);
    act(() => {
      fireKey('ArrowUp', { code: 'ArrowUp' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('Escape on detail route calls router.navigate to /prs', () => {
    mockPathname.current = '/prs/some-pr';
    mockRouterNavigate.mockClear();
    renderWithOverlays();
    act(() => {
      fireKey('Escape', { code: 'Escape' });
    });
    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs' }),
    );
  });

  it('closes overlay on Escape when command overlay is open', () => {
    const { result } = renderWithOverlays();
    // Open command overlay first
    act(() => { result.current.openCommand(); });
    expect(result.current.activeOverlay).toBe('command');
    act(() => {
      fireKey('Escape', { code: 'Escape' });
    });
    expect(result.current.activeOverlay).toBeNull();
  });

  it('runs the "g g" sequence to jump to the top', () => {
    appStore.setState((s) => ({ ...s, selectedPRIndex: 6 }));
    renderWithOverlays();
    act(() => {
      fireKey('g', { code: 'KeyG' });
      fireKey('g', { code: 'KeyG' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('route-gates bindings: "j" in detail route does not move the PR selection', () => {
    // Simulate being on a detail route, not list
    mockPathname.current = '/prs/some-pr';
    renderWithOverlays();
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('suppresses bindings while typing in an input', () => {
    renderWithOverlays();
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => {
      fireKey('j', { code: 'KeyJ' }, input);
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('Escape still works while typing in an input', () => {
    const { result } = renderWithOverlays();
    // Open command overlay first so Escape fires when overlay is active
    act(() => { result.current.openCommand(); });
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => {
      fireKey('Escape', { code: 'Escape' }, input);
    });
    expect(result.current.activeOverlay).toBeNull();
  });

  it('applies settings overrides: list.down remapped to "x"', () => {
    settingsStore.setState((s) => ({
      ...s,
      keybindings: { 'list.down': ['x'] },
    }));
    const { rerender } = renderWithOverlays();
    rerender();
    act(() => {
      fireKey('x', { code: 'KeyX' });
    });
    expect(appStore.state.selectedPRIndex).toBe(1);
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(1);
  });

  it('stops firing after unmount', () => {
    const { unmount } = renderWithOverlays();
    unmount();
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });
});
