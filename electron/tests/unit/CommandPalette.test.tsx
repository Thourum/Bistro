// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CommandPalette } from '../../src/renderer/components/CommandPalette';
import { appStore } from '../../src/renderer/store';

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [], isLoading: false, error: null }),
}));
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ send: vi.fn(), request: vi.fn() }),
}));

expect.extend(matchers);

describe('CommandPalette', () => {
  beforeEach(() => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  });

  afterEach(() => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  });

  it('does not render when mode is not "command"', () => {
    const { container } = render(<CommandPalette />);
    expect(container.querySelector('[role="dialog"]')).not.toBeInTheDocument();
  });

  it('renders when mode is "command"', () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    expect(screen.getByPlaceholderText(/Type a command/i)).toBeInTheDocument();
  });

  it('displays core commands', () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    expect(screen.getByText(/Show keyboard shortcuts/i)).toBeInTheDocument();
    expect(screen.getByText(/Open project settings/i)).toBeInTheDocument();
    expect(screen.getByText(/Clear all filters/i)).toBeInTheDocument();
  });

  it('filters commands by search query', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'shortcut');
    expect(screen.getByText(/Show keyboard shortcuts/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/Open project settings/i),
    ).not.toBeInTheDocument();
  });

  it('closes on Escape', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    await userEvent.keyboard('{Escape}');
    expect(appStore.state.mode).toBe('list');
  });

  it('autofocuses the input on open', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(/Type a command/i);
    // Focus is applied in a microtask via setTimeout(..., 0).
    await new Promise((r) => setTimeout(r, 10));
    expect(input).toHaveFocus();
  });

  it(':clear-style command resets filters and search', async () => {
    appStore.setState((s) => ({
      ...s,
      mode: 'command',
      searchQuery: 'hello',
      filterState: 'draft',
      filterMenuOpen: true,
    }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'clear');
    await userEvent.keyboard('{Enter}');
    expect(appStore.state.searchQuery).toBe('');
    expect(appStore.state.filterState).toBe('open');
    expect(appStore.state.filterMenuOpen).toBe(false);
  });

  it('Enter on "shortcuts" switches mode to help', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'shortcut');
    await userEvent.keyboard('{Enter}');
    expect(appStore.state.mode).toBe('help');
  });

  it('Enter on repository picker command launches RepoPicker mode', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(/Type a command/i);
    await userEvent.type(input, 'repository picker');
    await userEvent.keyboard('{Enter}');
    expect(appStore.state.mode).toBe('repo-picker');
  });
});
