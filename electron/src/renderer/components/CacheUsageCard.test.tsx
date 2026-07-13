// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { CacheUsageCard } from './CacheUsageCard';
import ToastContainer from './ToastContainer';

const requestMock = vi.fn();
const sendMock = vi.fn();

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: sendMock,
  }),
}));

const statsResponse = {
  ok: true,
  total_bytes: 1000,
  active_bytes: 250,
  stale_bytes: 750,
  repos: [
    {
      repo_id: 'github:acme/api',
      size_bytes: 250,
      last_used_at: '2026-07-10T12:00:00Z',
      stale: false,
    },
    {
      repo_id: 'github:acme/old',
      size_bytes: 750,
      last_used_at: '2026-05-01T12:00:00Z',
      stale: true,
    },
  ],
};

function renderCard(): void {
  render(
    <>
      <CacheUsageCard />
      <ToastContainer />
    </>,
  );
}

describe('CacheUsageCard', () => {
  beforeEach(() => {
    requestMock.mockReset();
    sendMock.mockReset();
    appStore.setState(() => initialUIState);
    vi.spyOn(window, 'confirm').mockReturnValue(true);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    appStore.setState(() => initialUIState);
  });

  it('renders active and stale cache segments proportionally', async () => {
    requestMock.mockResolvedValueOnce(statsResponse);

    renderCard();

    expect(await screen.findByText('1000 B')).toBeInTheDocument();
    expect(screen.getByTestId('cache-active-segment')).toHaveStyle({
      width: '25%',
    });
    expect(screen.getByTestId('cache-stale-segment')).toHaveStyle({
      width: '75%',
    });
  });

  it('sends PRUNE_CACHE and refetches stats', async () => {
    const user = userEvent.setup();
    requestMock
      .mockResolvedValueOnce(statsResponse)
      .mockResolvedValueOnce({
        ok: true,
        removed: ['github:acme/old'],
        failed: [],
      })
      .mockResolvedValueOnce({ ...statsResponse, stale_bytes: 0 });

    renderCard();

    await screen.findByText('1000 B');
    await user.click(screen.getByRole('button', { name: /^prune$/i }));

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(3));
    expect(requestMock).toHaveBeenNthCalledWith(2, 'PRUNE_CACHE', {});
    expect(requestMock).toHaveBeenNthCalledWith(3, 'GET_CACHE_STATS', {});
  });

  it('requires confirmation before clearing all cached repos', async () => {
    const user = userEvent.setup();
    vi.mocked(window.confirm).mockReturnValue(false);
    requestMock.mockResolvedValueOnce(statsResponse);

    renderCard();

    await screen.findByText('1000 B');
    await user.click(
      screen.getByRole('button', { name: /clear all cached repos/i }),
    );

    expect(window.confirm).toHaveBeenCalledWith('Clear all cached repos?');
    expect(requestMock).toHaveBeenCalledTimes(1);
  });

  it('sends DELETE_CACHE_REPO for a row trash action and refetches stats', async () => {
    const user = userEvent.setup();
    requestMock
      .mockResolvedValueOnce(statsResponse)
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ ...statsResponse, repos: [] });

    renderCard();

    await screen.findByRole('button', { name: /show cached repositories/i });
    await user.click(
      screen.getByRole('button', { name: /show cached repositories/i }),
    );
    await user.click(
      screen.getByRole('button', {
        name: /delete cached repo github:acme\/api/i,
      }),
    );

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(3));
    expect(requestMock).toHaveBeenNthCalledWith(2, 'DELETE_CACHE_REPO', {
      repo_id: 'github:acme/api',
    });
    expect(requestMock).toHaveBeenNthCalledWith(3, 'GET_CACHE_STATS', {});
  });

  it('shows a generic toast when a delete_failed response is returned', async () => {
    const user = userEvent.setup();
    requestMock.mockResolvedValueOnce(statsResponse).mockResolvedValueOnce({
      ok: false,
      repo_id: 'github:acme/api',
      operation: 'delete_cache_repo',
      code: 'delete_failed',
      message: 'permission details should stay in logs',
    });

    renderCard();

    await screen.findByRole('button', { name: /show cached repositories/i });
    await user.click(
      screen.getByRole('button', { name: /show cached repositories/i }),
    );
    await user.click(
      screen.getByRole('button', {
        name: /delete cached repo github:acme\/api/i,
      }),
    );

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Something went wrong, check the logs',
    );
    expect(screen.queryByText(/permission details/i)).not.toBeInTheDocument();
  });

  it('shows a generic toast when PRUNE_CACHE returns failed deletions', async () => {
    const user = userEvent.setup();
    requestMock
      .mockResolvedValueOnce(statsResponse)
      .mockResolvedValueOnce({
        ok: true,
        removed: [],
        failed: [
          {
            repo_id: 'github:acme/old',
            code: 'delete_failed',
            message: 'permission details should stay in logs',
          },
        ],
      })
      .mockResolvedValueOnce(statsResponse);

    renderCard();

    await screen.findByText('1000 B');
    await user.click(screen.getByRole('button', { name: /^prune$/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Something went wrong, check the logs',
    );
    expect(screen.queryByText(/permission details/i)).not.toBeInTheDocument();
    expect(requestMock).toHaveBeenCalledTimes(2);
  });

  it('shows a generic toast when CLEAR_CACHE returns failed deletions', async () => {
    const user = userEvent.setup();
    requestMock
      .mockResolvedValueOnce(statsResponse)
      .mockResolvedValueOnce({
        ok: true,
        removed: [],
        failed: [
          {
            repo_id: 'github:acme/api',
            code: 'delete_failed',
            message: 'permission details should stay in logs',
          },
        ],
        orphans_removed: 0,
      })
      .mockResolvedValueOnce(statsResponse);

    renderCard();

    await screen.findByText('1000 B');
    await user.click(
      screen.getByRole('button', { name: /clear all cached repos/i }),
    );

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Something went wrong, check the logs',
    );
    expect(screen.queryByText(/permission details/i)).not.toBeInTheDocument();
    expect(requestMock).toHaveBeenCalledTimes(2);
  });
});
