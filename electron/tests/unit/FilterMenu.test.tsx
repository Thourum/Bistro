// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { FilterMenu } from '../../src/renderer/components/FilterMenu';
import {
  OverlayProvider,
  useOverlays,
} from '../../src/renderer/overlays/OverlayContext';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';

expect.extend(matchers);

function wrapper({ children }: { children: React.ReactNode }) {
  return React.createElement(OverlayProvider, null, children);
}

/** Renders FilterMenu + overlay state indicator in one tree */
function renderFilterMenu(initialOpen = false) {
  let overlayRef: ReturnType<typeof useOverlays> | null = null;
  const TestComp = () => {
    overlayRef = useOverlays();
    if (initialOpen && overlayRef.activeOverlay !== 'filter') {
      overlayRef.openFilter();
    }
    return <FilterMenu />;
  };
  const result = render(<TestComp />, { wrapper });
  return { ...result, getOverlays: () => overlayRef! };
}

describe('FilterMenu', () => {
  beforeEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
  });

  afterEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
  });

  it('renders filter button', () => {
    renderFilterMenu();
    expect(screen.getByRole('button', { name: /Filter/i })).toBeInTheDocument();
  });

  it('shows keyboard hint for filter shortcut', () => {
    renderFilterMenu();
    expect(screen.getByText('f')).toBeInTheDocument();
  });

  it('does not show filter panel when filter overlay is not active', () => {
    renderFilterMenu();
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('shows filter panel when filter overlay is active', () => {
    // Use state-based open via the button click
    renderFilterMenu();
    const btn = screen.getByRole('button', { name: /Filter/i });
    // Click to open
    return userEvent.click(btn).then(() => {
      expect(screen.getByRole('menu')).toBeInTheDocument();
      expect(screen.getByText('Open')).toBeInTheDocument();
      expect(screen.getByText('Draft')).toBeInTheDocument();
      expect(screen.getByText('All')).toBeInTheDocument();
    });
  });

  it('displays filter options keyboard hints when open', async () => {
    renderFilterMenu();
    const btn = screen.getByRole('button', { name: /Filter/i });
    await userEvent.click(btn);
    expect(screen.getByText('o')).toBeInTheDocument();
    expect(screen.getByText('d')).toBeInTheDocument();
    expect(screen.getByText('a')).toBeInTheDocument();
  });

  it('changes filter state and closes overlay when option is clicked', async () => {
    renderFilterMenu();
    const btn = screen.getByRole('button', { name: /Filter/i });
    await userEvent.click(btn);
    expect(screen.getByRole('menu')).toBeInTheDocument();
    const draftOption = screen.getByText('Draft');
    await userEvent.click(draftOption);
    expect(appStore.state.filterState).toBe('draft');
    // Overlay should be closed after selection
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('toggles filter overlay open/closed when button is clicked', async () => {
    renderFilterMenu();
    const btn = screen.getByRole('button', { name: /Filter/i });
    await userEvent.click(btn);
    // Panel should be visible
    expect(screen.getByRole('menu')).toBeInTheDocument();
    await userEvent.click(btn);
    // Panel should be hidden
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
});
