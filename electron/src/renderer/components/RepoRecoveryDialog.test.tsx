// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RepoInfo } from '../types';
import { RepoRecoveryDialog } from './RepoRecoveryDialog';

const requestMock = vi.fn();
const closeMock = vi.fn();
const pickFolderMock = vi.fn();

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: vi.fn(),
  }),
}));

const degradedRepo: RepoInfo = {
  id: 'github:acme/api',
  provider: 'github',
  owner: 'acme',
  repo: 'api',
  watched: true,
  source: 'local',
  location_status: 'missing',
  location_error_code: 'path_missing',
  location_error: 'folder not found: /Users/sam/dev/api',
};

function installElectronMock(): void {
  Object.defineProperty(window, 'electron', {
    configurable: true,
    value: { pickFolder: pickFolderMock },
  });
}

function renderDialog(repo: RepoInfo = degradedRepo): void {
  render(<RepoRecoveryDialog repo={repo} onClose={closeMock} />);
}

describe('RepoRecoveryDialog', () => {
  beforeEach(() => {
    requestMock.mockReset();
    closeMock.mockReset();
    pickFolderMock.mockReset();
    installElectronMock();
  });

  it('shows the degraded repo message', () => {
    renderDialog();

    expect(
      screen.getByText(
        'Repo acme/api could not sync: folder not found: /Users/sam/dev/api',
      ),
    ).toBeInTheDocument();
  });

  it('chooses a new path with RECOVER_REPO_PATH', async () => {
    const user = userEvent.setup();
    pickFolderMock.mockResolvedValueOnce('/Users/sam/dev/api');
    requestMock.mockResolvedValueOnce({ ok: true });
    renderDialog();

    await user.click(screen.getByRole('button', { name: /choose new path/i }));

    await waitFor(() => {
      expect(requestMock).toHaveBeenCalledWith('RECOVER_REPO_PATH', {
        repo_id: 'github:acme/api',
        path: '/Users/sam/dev/api',
      });
    });
    expect(closeMock).toHaveBeenCalledTimes(1);
  });

  it('uses location_error_code, not message text, for remote mismatch state', async () => {
    const user = userEvent.setup();
    pickFolderMock.mockResolvedValueOnce('/Users/sam/dev/api');
    requestMock.mockResolvedValueOnce({
      ok: false,
      repo_id: 'github:acme/api',
      operation: 'recover_repo_path',
      code: 'remote_mismatch',
      message: 'path disappeared instead',
    });
    renderDialog({
      ...degradedRepo,
      location_error_code: 'remote_mismatch',
      location_error: 'folder disappeared instead',
    });

    expect(
      screen.getByText(/the selected path belongs to another remote/i),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /choose new path/i }));

    expect(
      await screen.findByText(/that folder belongs to a different repository/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('path disappeared instead'),
    ).not.toBeInTheDocument();
    expect(closeMock).not.toHaveBeenCalled();
  });

  it('moves the repo to Bistro cache with RECOVER_REPO_TO_CACHE', async () => {
    const user = userEvent.setup();
    requestMock.mockResolvedValueOnce({ ok: true });
    renderDialog();

    await user.click(
      screen.getByRole('button', { name: /move to bistro cache/i }),
    );

    expect(
      screen.getByText(/bistro will clone this repository/i),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole('button', { name: /confirm move to cache/i }),
    );

    await waitFor(() => {
      expect(requestMock).toHaveBeenCalledWith('RECOVER_REPO_TO_CACHE', {
        repo_id: 'github:acme/api',
      });
    });
    expect(closeMock).toHaveBeenCalledTimes(1);
  });

  it('preflights removal and confirms REMOVE_REPO with counts', async () => {
    const user = userEvent.setup();
    requestMock
      .mockResolvedValueOnce({
        ok: true,
        counts: { prs: 3, comments: 9, threads: 2, patchsets: 4, actions: 1 },
      })
      .mockResolvedValueOnce({ ok: true, deleted: {} });
    renderDialog();

    await user.click(
      screen.getByRole('button', { name: /remove repo and pr history/i }),
    );

    await waitFor(() => {
      expect(requestMock).toHaveBeenNthCalledWith(1, 'REMOVE_REPO_PREFLIGHT', {
        repo_id: 'github:acme/api',
      });
    });
    expect(
      screen.getByText(/This deletes 3 PRs, 9 comments/i),
    ).toBeInTheDocument();
    expect(requestMock).toHaveBeenCalledTimes(1);

    await user.click(
      screen.getByRole('button', { name: /confirm remove repo/i }),
    );

    expect(requestMock).toHaveBeenNthCalledWith(2, 'REMOVE_REPO', {
      repo_id: 'github:acme/api',
    });
    await waitFor(() => expect(closeMock).toHaveBeenCalledTimes(1));
  });
});
