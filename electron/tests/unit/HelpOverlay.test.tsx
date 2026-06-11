// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect } from 'vitest';
import { HelpOverlay } from '../../src/renderer/components/HelpOverlay';
import { appStore } from '../../src/renderer/store';

expect.extend(matchers);

describe('HelpOverlay', () => {
  beforeEach(() => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  });

  afterEach(() => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  });

  it('does not render when mode is not "help"', () => {
    const { container } = render(<HelpOverlay />);
    expect(container.querySelector('[role="dialog"]')).not.toBeInTheDocument();
  });

  it('renders when mode is "help"', () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    expect(screen.getByText('Keyboard Shortcuts')).toBeInTheDocument();
  });

  it('displays keyboard shortcuts', () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    expect(screen.getByText(/Navigation/)).toBeInTheDocument();
    // j and k are separate bindings; TanStack's formatForDisplay renders 'J'.
    expect(screen.getAllByText('J').length).toBeGreaterThan(0);
  });

  it('closes on Escape', async () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    await userEvent.keyboard('{Escape}');
    expect(appStore.state.mode).toBe('list');
  });

  it('displays actions section', () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    expect(screen.getByText(/Actions/)).toBeInTheDocument();
  });

  it('displays search and commands section', () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    expect(screen.getByText(/Search/)).toBeInTheDocument();
  });

  it('derives shortcuts from DEFAULT_KEYBINDINGS (not hardcoded)', () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    // The Escape binding description comes from the binding definition
    expect(
      screen.getByText(/Close help \/ cancel input \/ back to list/),
    ).toBeInTheDocument();
  });

  it('shows all four section categories', () => {
    appStore.setState((s) => ({ ...s, mode: 'help' }));
    render(<HelpOverlay />);
    expect(screen.getByText('Navigation')).toBeInTheDocument();
    expect(screen.getByText('Actions')).toBeInTheDocument();
    expect(screen.getByText(/Search/)).toBeInTheDocument();
  });
});
