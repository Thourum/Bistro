// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { CommandPalette } from '../../src/renderer/components/CommandPalette';
import { appStore } from '../../src/renderer/store';

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
    expect(screen.getByPlaceholderText(':')).toBeInTheDocument();
  });

  it('displays all available commands', () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    expect(screen.getByText('help')).toBeInTheDocument();
    expect(screen.getByText('settings')).toBeInTheDocument();
    expect(screen.getByText('clear')).toBeInTheDocument();
  });

  it('filters commands by search query', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(':') as HTMLInputElement;
    await userEvent.type(input, 'help');
    expect(screen.getByText('help')).toBeInTheDocument();
    expect(screen.queryByText('settings')).not.toBeInTheDocument();
  });

  it('closes on Escape', async () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    await userEvent.keyboard('{Escape}');
    expect(appStore.state.mode).toBe('list');
  });

  it('has autofocus on input', () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(':') as HTMLInputElement;
    expect(input).toHaveFocus();
  });

  it('input has monospace styling', () => {
    appStore.setState((s) => ({ ...s, mode: 'command' }));
    render(<CommandPalette />);
    const input = screen.getByPlaceholderText(':');
    expect(input).toHaveClass('font-mono');
  });
});
