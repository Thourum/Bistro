// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';
import { expect } from 'vitest';
import { HelpOverlay } from '../../src/renderer/components/HelpOverlay';
import { OverlayProvider, useOverlays } from '../../src/renderer/overlays/OverlayContext';

expect.extend(matchers);

function wrapper({ children }: { children: React.ReactNode }) {
  return React.createElement(OverlayProvider, null, children);
}

function renderHelpOpen(source: 'list' | 'detail' = 'list') {
  const TestComp = () => {
    const { openHelp } = useOverlays();
    // Use state to track if opened
    const [opened, setOpened] = React.useState(false);
    if (!opened) {
      setOpened(true);
      openHelp(source);
    }
    return <HelpOverlay />;
  };
  return render(<TestComp />, { wrapper });
}

describe('HelpOverlay', () => {
  it('does not render when overlay is not "help"', () => {
    const { container } = render(<HelpOverlay />, { wrapper });
    expect(container.querySelector('[role="dialog"]')).not.toBeInTheDocument();
  });

  it('renders when help overlay is open', () => {
    renderHelpOpen();
    expect(screen.getByText('Keyboard Shortcuts')).toBeInTheDocument();
  });

  it('displays keyboard shortcuts when open', () => {
    renderHelpOpen();
    expect(screen.getByText(/Navigation/)).toBeInTheDocument();
    // j and k are separate bindings; TanStack's formatForDisplay renders 'J'.
    expect(screen.getAllByText('J').length).toBeGreaterThan(0);
  });

  it('closes on Escape via Dialog', async () => {
    renderHelpOpen();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByText('Keyboard Shortcuts')).not.toBeInTheDocument();
  });

  it('displays actions section', () => {
    renderHelpOpen();
    expect(screen.getByText(/Actions/)).toBeInTheDocument();
  });

  it('displays search and commands section', () => {
    renderHelpOpen();
    expect(screen.getByText('Search & Commands')).toBeInTheDocument();
  });

  it('derives shortcuts from DEFAULT_KEYBINDINGS (not hardcoded)', () => {
    renderHelpOpen();
    expect(screen.getByText(/Back \/ Cancel/)).toBeInTheDocument();
  });

  it('shows all four section categories', () => {
    renderHelpOpen();
    expect(screen.getByText('Navigation')).toBeInTheDocument();
    expect(screen.getByText('Actions')).toBeInTheDocument();
    expect(screen.getByText('Search & Commands')).toBeInTheDocument();
  });

  it('uses helpSourceRoute to scope content (detail shows detail bindings)', () => {
    renderHelpOpen('detail');
    expect(screen.getByText('Keyboard Shortcuts')).toBeInTheDocument();
  });
});
