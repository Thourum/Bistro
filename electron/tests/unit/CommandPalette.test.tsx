// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { act, render, renderHook, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CommandPalette } from '../../src/renderer/components/CommandPalette';
import {
  OverlayProvider,
  useOverlays,
} from '../../src/renderer/overlays/OverlayContext';
import { appStore } from '../../src/renderer/store';

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [], isLoading: false, error: null }),
}));
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ send: vi.fn(), request: vi.fn() }),
}));

// Mock router for navigation
vi.mock('../../src/renderer/router', () => ({
  router: { navigate: vi.fn().mockResolvedValue(undefined) },
  prDetailRoute: {},
}));
vi.mock('@tanstack/react-router', () => ({
  useNavigate: vi.fn(() => vi.fn()),
  useParams: vi.fn(() => ({})),
  useRouterState: vi.fn(() => ({ location: { pathname: '/prs' } })),
}));

expect.extend(matchers);

function wrapper({ children }: { children: React.ReactNode }) {
  return React.createElement(OverlayProvider, null, children);
}

/** Helper to open command palette via overlay context. */
function renderWithCommand() {
  const TestComp = () => {
    const overlays = useOverlays();
    React.useEffect(() => {
      overlays.openCommand();
    }, []);
    return <CommandPalette />;
  };
  return { result: render(<TestComp />, { wrapper }) };
}

describe('CommandPalette', () => {
  beforeEach(() => {
    appStore.setState((s) => ({ ...s, searchQuery: '', filterState: 'open' }));
  });

  afterEach(() => {
    appStore.setState((s) => ({ ...s, searchQuery: '', filterState: 'open' }));
  });

  it('does not render when overlay is not "command"', () => {
    const { container } = render(<CommandPalette />, { wrapper });
    expect(container.querySelector('[role="dialog"]')).not.toBeInTheDocument();
  });

  it('renders when command overlay is open', () => {
    renderWithCommand();
    expect(screen.getByPlaceholderText(/Type a command/i)).toBeInTheDocument();
  });

  it('displays core commands', () => {
    renderWithCommand();
    expect(screen.getByText(/Show keyboard shortcuts/i)).toBeInTheDocument();
    expect(screen.getByText(/Open project settings/i)).toBeInTheDocument();
    expect(screen.getByText(/Clear all filters/i)).toBeInTheDocument();
  });

  it('filters commands by search query', async () => {
    renderWithCommand();
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'shortcut');
    expect(screen.getByText(/Show keyboard shortcuts/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/Open project settings/i),
    ).not.toBeInTheDocument();
  });

  it('closes on Escape and overlay becomes null', async () => {
    const { result: hookResult } = renderHook(() => useOverlays(), { wrapper });
    act(() => hookResult.current.openCommand());
    // Render palette inside same wrapper
    const TestComp = () => {
      const overlays = useOverlays();
      React.useEffect(() => {
        overlays.openCommand();
      }, []);
      return <CommandPalette />;
    };
    render(<TestComp />, { wrapper: OverlayProvider as React.ComponentType });
    await userEvent.keyboard('{Escape}');
    // Dialog is dismissed after Escape (overlay-only close, no store side-effects)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('autofocuses the input on open', async () => {
    renderWithCommand();
    const input = screen.getByPlaceholderText(/Type a command/i);
    // Focus is applied in a microtask via setTimeout(..., 0).
    await new Promise((r) => setTimeout(r, 10));
    expect(input).toHaveFocus();
  });

  it(':clear-style command resets filters and search', async () => {
    appStore.setState((s) => ({
      ...s,
      searchQuery: 'hello',
      filterState: 'draft',
    }));
    renderWithCommand();
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'clear');
    await userEvent.keyboard('{Enter}');
    expect(appStore.state.searchQuery).toBe('');
    expect(appStore.state.filterState).toBe('open');
  });

  it('Enter on "shortcuts" opens help overlay', async () => {
    const TestComp = () => {
      const overlays = useOverlays();
      React.useEffect(() => {
        overlays.openCommand();
      }, []);
      return (
        <>
          <CommandPalette />
          <div data-testid="overlay">{overlays.activeOverlay}</div>
        </>
      );
    };
    render(<TestComp />, { wrapper });
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'shortcut');
    await userEvent.keyboard('{Enter}');
    // Should switch to help overlay
    expect(screen.getByTestId('overlay').textContent).toBe('help');
  });

  it('repository picker command opens repoPicker overlay', async () => {
    const TestComp = () => {
      const overlays = useOverlays();
      React.useEffect(() => {
        overlays.openCommand();
      }, []);
      return (
        <>
          <CommandPalette />
          <div data-testid="overlay">{overlays.activeOverlay}</div>
        </>
      );
    };
    render(<TestComp />, { wrapper });
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'repository picker');
    await userEvent.keyboard('{Enter}');
    expect(screen.getByTestId('overlay').textContent).toBe('repoPicker');
  });
});
