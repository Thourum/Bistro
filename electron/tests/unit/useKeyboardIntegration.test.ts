// @vitest-environment jsdom
import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { useKeyboardIntegration } from '../../src/renderer/hooks/useKeyboardIntegration';
import { appStore } from '../../src/renderer/store';

// We test useKeyboardIntegration by firing real keyboard events on window and
// asserting that appStore state mutates as expected.  This is the correct
// integration approach because useKeyboard (and therefore
// useKeyboardIntegration) works via window.addEventListener('keydown').

function fireKey(key: string, opts: KeyboardEventInit = {}): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...opts }));
}

describe('useKeyboardIntegration', () => {
  beforeEach(() => {
    // Reset store to a known baseline before each test.
    appStore.setState(() => ({
      selectedPRIndex: 0,
      selectedFileIndex: 0,
      searchQuery: '',
      mode: 'list' as const,
      paneLayout: 'split' as const,
      sortBy: 'updated' as const,
      filterState: 'open' as const,
      confirmingAction: null,
      filterMenuOpen: false,
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('mounts without throwing', () => {
    expect(() => renderHook(() => useKeyboardIntegration())).not.toThrow();
  });

  it('moves selection down on "j" in list mode', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('j'); });
    expect(appStore.state.selectedPRIndex).toBe(1);
  });

  it('does not move selection below zero on "k" from index 0', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('k'); });
    expect(appStore.state.selectedPRIndex).toBe(0);
  });

  it('moves selection up on "k" when index > 0', () => {
    appStore.setState((s) => ({ ...s, selectedPRIndex: 3 }));
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('k'); });
    expect(appStore.state.selectedPRIndex).toBe(2);
  });

  it('opens detail mode on Enter', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('Enter'); });
    expect(appStore.state.mode).toBe('detail');
  });

  it('shows help overlay on "?"', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('?'); });
    expect(appStore.state.mode).toBe('help');
  });

  it('switches to search mode on "/"', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('/'); });
    expect(appStore.state.mode).toBe('search');
  });

  it('toggles filter menu on "f"', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('f'); });
    expect(appStore.state.filterMenuOpen).toBe(true);
    act(() => { fireKey('f'); });
    expect(appStore.state.filterMenuOpen).toBe(false);
  });

  it('sets confirmingAction to "command" on ":"', () => {
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey(':'); });
    expect(appStore.state.confirmingAction).toBe('command');
  });

  it('returns from detail mode to list on Escape', () => {
    appStore.setState((s) => ({ ...s, mode: 'detail' }));
    renderHook(() => useKeyboardIntegration());
    act(() => { fireKey('Escape'); });
    expect(appStore.state.mode).toBe('list');
  });

  it('removes event listener on unmount (no memory leak)', () => {
    const addSpy = vi.spyOn(window, 'addEventListener');
    const removeSpy = vi.spyOn(window, 'removeEventListener');

    const { unmount } = renderHook(() => useKeyboardIntegration());
    expect(addSpy).toHaveBeenCalledWith('keydown', expect.any(Function));

    unmount();
    expect(removeSpy).toHaveBeenCalledWith('keydown', expect.any(Function));
  });
});
