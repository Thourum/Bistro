// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { useAppHotkeys } from '../../src/renderer/hooks/useAppHotkeys';
import { appStore } from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import { initialUIState } from '../../src/renderer/store/ui';

// Integration tests: real KeyboardEvents dispatched through the DOM, handled
// by TanStack Hotkeys (which listens on `document` by default), asserting
// appStore transitions.

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

describe('useAppHotkeys', () => {
  beforeEach(() => {
    appStore.setState(() => ({ ...initialUIState, mode: 'list' as const }));
    settingsStore.setState((s) => ({ ...s, keybindings: {} }));
    document.body.innerHTML = '';
  });

  it('mounts without throwing', () => {
    expect(() => renderHook(() => useAppHotkeys())).not.toThrow();
  });

  it('moves selection down on "j" in list mode', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(1);
  });

  it('does not move selection below zero on "k" from index 0', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('k', { code: 'KeyK' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('opens detail mode on Enter', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('Enter', { code: 'Enter' });
    });
    expect(appStore.state.mode).toBe('detail');
  });

  it('switches to search mode on "/"', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('/', { code: 'Slash' });
    });
    expect(appStore.state.mode).toBe('search');
  });

  it('toggles the filter menu on "f"', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('f', { code: 'KeyF' });
    });
    expect(appStore.state.filterMenuOpen).toBe(true);
    act(() => {
      fireKey('f', { code: 'KeyF' });
    });
    expect(appStore.state.filterMenuOpen).toBe(false);
  });

  it('opens help on "?" (Shift+/ physically)', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('?', { code: 'Slash', shiftKey: true });
    });
    expect(appStore.state.mode).toBe('help');
    expect(appStore.state.helpSourceMode).toBe('list');
  });

  it('opens command mode on ":" (Shift+; physically)', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey(':', { code: 'Semicolon', shiftKey: true });
    });
    expect(appStore.state.mode).toBe('command');
  });

  it('moves selection with the arrow keys', () => {
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('ArrowDown', { code: 'ArrowDown' });
    });
    expect(appStore.state.selectedPRIndex).toBe(1);
    act(() => {
      fireKey('ArrowUp', { code: 'ArrowUp' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('returns from detail mode to list on Escape', () => {
    appStore.setState((s) => ({ ...s, mode: 'detail' as const }));
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('Escape', { code: 'Escape' });
    });
    expect(appStore.state.mode).toBe('list');
  });

  it('runs the "g g" sequence to jump to the top', () => {
    appStore.setState((s) => ({ ...s, selectedPRIndex: 6 }));
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('g', { code: 'KeyG' });
      fireKey('g', { code: 'KeyG' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('mode-gates bindings: "j" in detail mode does not move the PR selection', () => {
    appStore.setState((s) => ({ ...s, mode: 'detail' as const }));
    renderHook(() => useAppHotkeys());
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('suppresses bindings while typing in an input', () => {
    renderHook(() => useAppHotkeys());
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => {
      fireKey('j', { code: 'KeyJ' }, input);
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('Escape still works while typing in an input', () => {
    appStore.setState((s) => ({ ...s, mode: 'search' as const }));
    renderHook(() => useAppHotkeys());
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => {
      fireKey('Escape', { code: 'Escape' }, input);
    });
    expect(appStore.state.mode).toBe('list');
  });

  it('applies settings overrides: list.down remapped to "x"', () => {
    settingsStore.setState((s) => ({
      ...s,
      keybindings: { 'list.down': ['x'] },
    }));
    const { rerender } = renderHook(() => useAppHotkeys());
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
    const { unmount } = renderHook(() => useAppHotkeys());
    unmount();
    act(() => {
      fireKey('j', { code: 'KeyJ' });
    });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });
});
