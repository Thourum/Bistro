// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { type AppSettings, DEFAULT_SETTINGS } from '../../types/settings';
import ToastContainer from '../components/ToastContainer';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import {
  expertiseSettingsPayloadFromInputs,
  SettingsPage,
} from './SettingsPage';

const requestMock = vi.fn();
const sendMock = vi.fn();

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: sendMock,
  }),
}));

vi.mock('../components/CacheUsageCard', () => ({
  CacheUsageCard: () => <div data-testid="cache-usage-card" />,
}));

function mergedSettings(patch: Partial<AppSettings>): AppSettings {
  return {
    ...settingsStore.state,
    ...patch,
    appearance: {
      ...settingsStore.state.appearance,
      ...patch.appearance,
    },
    sync: {
      ...settingsStore.state.sync,
      ...patch.sync,
    },
    prList: {
      ...settingsStore.state.prList,
      ...patch.prList,
    },
    privacy: {
      ...settingsStore.state.privacy,
      ...patch.privacy,
    },
    repositories: {
      ...settingsStore.state.repositories,
      ...patch.repositories,
    },
    keybindings: {
      ...settingsStore.state.keybindings,
      ...patch.keybindings,
    },
  };
}

function installElectronMock(): void {
  window.electron = {
    wsPort: 8080,
    db: vi.fn(),
    fs: vi.fn(),
    go: vi.fn(),
    onGoEvent: vi.fn(),
    send: vi.fn(),
    syncAction: vi.fn(),
    on: vi.fn(() => vi.fn()),
    openExternal: vi.fn(),
    quitApp: vi.fn(),
    pickFolder: vi.fn(),
    settings: {
      get: vi.fn(),
      set: vi.fn(async (patch: Partial<AppSettings>) => {
        const next = mergedSettings(patch);
        return { ok: next };
      }),
      reset: vi.fn(),
      openFile: vi.fn(),
      path: vi.fn(),
    },
  };
}

function setDevFolderPath(devFolderPath: string | null): void {
  settingsStore.setState((settings) => ({
    ...settings,
    repositories: {
      ...settings.repositories,
      devFolderPath,
    },
  }));
}

function renderSettingsPage(): void {
  render(
    <>
      <SettingsPage />
      <ToastContainer />
    </>,
  );
}

describe('expertise settings helpers', () => {
  it('preserves empty per-repo excludes so clearing a repo is persisted', () => {
    expect(
      expertiseSettingsPayloadFromInputs('alice@example.com', 50, {
        'github:acme/api': '',
      }),
    ).toEqual({
      alias_emails: ['alice@example.com'],
      min_surviving_lines: 50,
      excludes: { 'github:acme/api': [] },
    });
  });
});

describe('SettingsPage RepositoriesSection', () => {
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

  it('renders the configured dev-folder path and an empty state when unset', () => {
    setDevFolderPath('/Users/sam/dev');

    renderSettingsPage();

    const repositories = screen
      .getByRole('heading', { name: /repositories/i })
      .closest('section');
    expect(repositories).not.toBeNull();
    expect(
      within(repositories as HTMLElement).getByText('/Users/sam/dev'),
    ).toBeInTheDocument();
    expect(
      within(repositories as HTMLElement).queryByText('Not set'),
    ).not.toBeInTheDocument();

    cleanup();
    setDevFolderPath(null);
    renderSettingsPage();

    const emptyRepositories = screen
      .getByRole('heading', { name: /repositories/i })
      .closest('section');
    expect(emptyRepositories).not.toBeNull();
    expect(
      within(emptyRepositories as HTMLElement).getByText('Not set'),
    ).toBeInTheDocument();
  });

  it('validates, persists, and scans a picked dev folder', async () => {
    const user = userEvent.setup();
    vi.mocked(window.electron.pickFolder).mockResolvedValue('/Users/sam/dev');
    requestMock
      .mockResolvedValueOnce({ ok: true, canonical_path: '/private/var/dev' })
      .mockResolvedValueOnce({
        ok: true,
        discovered: 3,
        healthy: 2,
        degraded: 1,
        skipped: 0,
        partial: false,
      });

    renderSettingsPage();

    await user.click(screen.getByRole('button', { name: /change/i }));

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
    await waitFor(() => {
      expect(requestMock).toHaveBeenNthCalledWith(2, 'SCAN_DEV_FOLDER', {
        path: '/private/var/dev',
        reason: 'settings',
      });
    });
    expect(sendMock).not.toHaveBeenCalled();
    expect(settingsStore.state.repositories.devFolderPath).toBe(
      '/private/var/dev',
    );
    expect(
      screen.getByText('3 discovered, 2 healthy, 1 degraded, 0 skipped.'),
    ).toBeInTheDocument();
  });

  it('shows a validation error and persists nothing when dev-folder validation fails', async () => {
    const user = userEvent.setup();
    vi.mocked(window.electron.pickFolder).mockResolvedValue('/Users/sam');
    requestMock.mockResolvedValueOnce({
      ok: false,
      repo_id: '',
      operation: 'validate',
      code: 'invalid_path',
      message: 'Choose a projects folder',
    });

    renderSettingsPage();

    await user.click(screen.getByRole('button', { name: /change/i }));

    await waitFor(() => {
      expect(screen.getAllByRole('alert')).toHaveLength(2);
    });
    expect(screen.getAllByRole('alert')[0]).toHaveTextContent(
      'Choose a projects folder',
    );
    expect(requestMock).toHaveBeenCalledWith('VALIDATE_DEV_FOLDER', {
      path: '/Users/sam',
    });
    expect(window.electron.settings.set).not.toHaveBeenCalled();
    expect(requestMock).toHaveBeenCalledTimes(1);
    expect(settingsStore.state.repositories.devFolderPath).toBeNull();
  });

  it('clears the configured dev-folder path and resets scan status', async () => {
    const user = userEvent.setup();
    setDevFolderPath('/Users/sam/dev');
    requestMock
      .mockResolvedValueOnce({ ok: true, canonical_path: '/private/var/dev' })
      .mockResolvedValueOnce({
        ok: true,
        discovered: 1,
        healthy: 1,
        degraded: 0,
        skipped: 0,
        partial: false,
      });

    renderSettingsPage();

    await user.click(screen.getByRole('button', { name: /clear/i }));

    await waitFor(() => {
      expect(window.electron.settings.set).toHaveBeenCalledWith({
        repositories: { devFolderPath: null },
      });
    });
    expect(settingsStore.state.repositories.devFolderPath).toBeNull();
    expect(screen.getByText('No scan has run yet.')).toBeInTheDocument();
  });

  it('renders scan status with a partial-scan notice', async () => {
    const user = userEvent.setup();
    vi.mocked(window.electron.pickFolder).mockResolvedValue('/Users/sam/dev');
    requestMock
      .mockResolvedValueOnce({ ok: true, canonical_path: '/private/var/dev' })
      .mockResolvedValueOnce({
        ok: true,
        discovered: 4,
        healthy: 2,
        degraded: 1,
        skipped: 1,
        partial: true,
      });

    renderSettingsPage();

    await user.click(screen.getByRole('button', { name: /change/i }));

    expect(
      await screen.findByText(
        '4 discovered, 2 healthy, 1 degraded, 1 skipped. Partial scan; check the logs.',
      ),
    ).toBeInTheDocument();
  });
});
