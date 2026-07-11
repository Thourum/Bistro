// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useEffect } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { OverlayProvider, useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { CommandPalette } from './CommandPalette';

const navigateMock = vi.fn();
const requestMock = vi.fn();
const sendMock = vi.fn();

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
}));

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: sendMock,
  }),
}));

vi.mock('../hooks/useVisiblePRs', () => ({
  useVisiblePRs: () => ({ prs: [] }),
}));

function OpenCommandOnMount(): null {
  const { openCommand } = useOverlays();

  useEffect(() => {
    openCommand();
  }, [openCommand]);

  return null;
}

function ActiveOverlayProbe(): React.ReactElement {
  const { activeOverlay } = useOverlays();
  return <div data-testid="active-overlay">{activeOverlay ?? 'none'}</div>;
}

function renderOpenPalette(): void {
  render(
    <OverlayProvider>
      <OpenCommandOnMount />
      <ActiveOverlayProbe />
      <CommandPalette />
    </OverlayProvider>,
  );
}

describe('CommandPalette add repository command', () => {
  beforeEach(() => {
    navigateMock.mockReset();
    requestMock.mockReset();
    sendMock.mockReset();
    appStore.setState(() => initialUIState);
  });

  afterEach(() => {
    appStore.setState(() => initialUIState);
  });

  it('opens the add repository overlay when selected', async () => {
    const user = userEvent.setup();
    renderOpenPalette();

    await user.type(screen.getByRole('combobox'), 'add repository');
    await user.click(
      screen.getByRole('option', { name: /add repository by url/i }),
    );

    expect(screen.getByTestId('active-overlay')).toHaveTextContent('addRepo');
    expect(navigateMock).not.toHaveBeenCalled();
  });
});
