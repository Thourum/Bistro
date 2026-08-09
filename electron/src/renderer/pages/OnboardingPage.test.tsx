// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { type AppSettings, DEFAULT_SETTINGS } from '../../types/settings';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { OnboardingPage } from './OnboardingPage';

const requestMock = vi.fn();
const sendMock = vi.fn();

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: sendMock,
  }),
}));

function installElectronMock(): void {
  window.electron = {
    wsPort: 8080,
    getWsPort: vi.fn(async () => 8080),
    db: vi.fn(),
    fs: vi.fn(),
    go: vi.fn(),
    onGoEvent: vi.fn(),
    send: vi.fn(),
    syncAction: vi.fn(),
    on: vi.fn(() => vi.fn()),
    openExternal: vi.fn(),
    openInEditor: vi.fn(),
    quitApp: vi.fn(),
    pickFolder: vi.fn(),
    settings: {
      get: vi.fn(),
      set: vi.fn(async (patch: Partial<AppSettings>) => {
        const next = {
          ...settingsStore.state,
          ...patch,
          appearance: {
            ...settingsStore.state.appearance,
            ...patch.appearance,
          },
          repositories: {
            ...settingsStore.state.repositories,
            ...patch.repositories,
          },
        };
        return { ok: next };
      }),
      reset: vi.fn(),
      openFile: vi.fn(),
      path: vi.fn(),
    },
  };
}

describe('OnboardingPage dev-folder step', () => {
  beforeEach(() => {
    requestMock.mockReset();
    sendMock.mockReset();
    installElectronMock();
    settingsStore.setState(() => DEFAULT_SETTINGS);
    appStore.setState(() => initialUIState);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    settingsStore.setState(() => DEFAULT_SETTINGS);
    appStore.setState(() => initialUIState);
  });

  it('leaves dev-folder unset and completes onboarding when skipped', async () => {
    const user = userEvent.setup();
    render(<OnboardingPage />);

    await user.click(screen.getByRole('button', { name: /skip/i }));

    expect(settingsStore.state.repositories.devFolderPath).toBeNull();
    expect(settingsStore.state.appearance.onboardingComplete).toBe(true);
    expect(window.electron.settings.set).toHaveBeenCalledWith({
      appearance: {
        ...DEFAULT_SETTINGS.appearance,
        onboardingComplete: true,
      },
    });
    expect(requestMock).not.toHaveBeenCalled();
    expect(sendMock).not.toHaveBeenCalled();
  });

  it('persists the canonical dev-folder path and shows the scan tally after validation succeeds', async () => {
    const user = userEvent.setup();
    vi.mocked(window.electron.pickFolder).mockResolvedValue('/Users/sam/dev');
    sendMock.mockReturnValue('req-validation-only');
    requestMock
      .mockResolvedValueOnce({ ok: true, canonical_path: '/private/var/dev' })
      .mockResolvedValueOnce({
        ok: true,
        discovered: 2,
        healthy: 2,
        degraded: 0,
        skipped: 0,
        partial: false,
      });

    render(<OnboardingPage />);

    await user.click(screen.getByRole('button', { name: /choose folder/i }));

    await waitFor(() => {
      expect(requestMock).toHaveBeenNthCalledWith(1, 'VALIDATE_DEV_FOLDER', {
        path: '/Users/sam/dev',
      });
    });
    await waitFor(() => {
      expect(window.electron.settings.set).toHaveBeenCalledWith({
        repositories: { devFolderPath: '/private/var/dev' },
      });
    });
    expect(requestMock).toHaveBeenNthCalledWith(2, 'SCAN_DEV_FOLDER', {
      path: '/private/var/dev',
      reason: 'settings-change',
    });
    expect(sendMock).not.toHaveBeenCalled();
    expect(settingsStore.state.repositories.devFolderPath).toBe(
      '/private/var/dev',
    );
    expect(screen.getByText(/2 repositories discovered/i)).toBeInTheDocument();
  });

  it('shows a validation error and persists nothing when validation fails', async () => {
    const user = userEvent.setup();
    vi.mocked(window.electron.pickFolder).mockResolvedValue('/Users/sam');
    sendMock.mockReturnValue('req-validation-only');
    requestMock.mockResolvedValueOnce({
      ok: false,
      repo_id: '',
      operation: 'validate',
      code: 'invalid_path',
      message: 'Choose a projects folder',
    });

    render(<OnboardingPage />);

    await user.click(screen.getByRole('button', { name: /choose folder/i }));

    expect(
      await screen.findByText('Choose a projects folder'),
    ).toBeInTheDocument();
    expect(requestMock).toHaveBeenCalledWith('VALIDATE_DEV_FOLDER', {
      path: '/Users/sam',
    });
    expect(sendMock).not.toHaveBeenCalled();
    expect(settingsStore.state.repositories.devFolderPath).toBeNull();
    expect(window.electron.settings.set).not.toHaveBeenCalled();
  });

  it('shows a settings error and does not scan when persisting the dev-folder path fails', async () => {
    const user = userEvent.setup();
    vi.mocked(window.electron.pickFolder).mockResolvedValue('/Users/sam/dev');
    vi.mocked(window.electron.settings.set).mockResolvedValue({
      err: 'Unable to write settings',
    });
    requestMock.mockResolvedValueOnce({
      ok: true,
      canonical_path: '/private/var/dev',
    });

    render(<OnboardingPage />);

    await user.click(screen.getByRole('button', { name: /choose folder/i }));

    expect(
      await screen.findByText('Unable to write settings'),
    ).toBeInTheDocument();
    expect(window.electron.settings.set).toHaveBeenCalledWith({
      repositories: { devFolderPath: '/private/var/dev' },
    });
    expect(requestMock).toHaveBeenCalledTimes(1);
    expect(requestMock).toHaveBeenCalledWith('VALIDATE_DEV_FOLDER', {
      path: '/Users/sam/dev',
    });
    expect(sendMock).not.toHaveBeenCalled();
    expect(settingsStore.state.repositories.devFolderPath).toBeNull();
    expect(
      screen.queryByText(/repositories discovered/i),
    ).not.toBeInTheDocument();
  });
});
