// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
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
    expect(screen.getByText(/j\/k/)).toBeInTheDocument();
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
});
