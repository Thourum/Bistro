// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import type { PR, RepoInfo } from '../types';
import { PRListTable } from './PRListTable';

const navigateMock = vi.fn();
const requestMock = vi.fn(() => Promise.resolve({ repos: [] }));

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
}));

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: vi.fn(),
  }),
}));

const pr: PR = {
  id: 'pr-1',
  provider: 'github',
  owner: 'acme',
  repo: 'api',
  number: 42,
  title: 'Improve sync',
  body: null,
  author: 'sam',
  state: 'open',
  created_at: '2026-07-10T00:00:00Z',
  updated_at: '2026-07-10T00:00:00Z',
  merged_at: null,
  base_branch: 'main',
  head_branch: 'feature',
  base_sha: 'base',
  head_sha: 'head',
  comment_count: 0,
  ci_status: null,
  draft: 0,
  review_status: null,
  approved_by: null,
  changes_requested_by: null,
  requested_reviewers: [],
  touches_you: false,
  hydrated: 1,
};

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

describe('PRListTable degraded repositories', () => {
  beforeEach(() => {
    navigateMock.mockReset();
    requestMock.mockReset();
    requestMock.mockResolvedValue({ repos: [] });
    appStore.setState(() => ({
      ...initialUIState,
      repos: [degradedRepo],
    }));
  });

  it('requests repos so direct PR list loads can render degraded repo rows', () => {
    appStore.setState((state) => ({
      ...state,
      repos: [],
    }));

    render(<PRListTable prs={[pr]} isLoading={false} error={null} />);

    expect(requestMock).toHaveBeenCalledWith('REQUEST_REPO_LIST', {});
  });

  it('grays out degraded repo rows and opens recovery actions', async () => {
    const user = userEvent.setup();
    render(<PRListTable prs={[pr]} isLoading={false} error={null} />);

    expect(
      screen.getByText('folder not found: /Users/sam/dev/api'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /open PR #42 in acme\/api/i })
        .parentElement,
    ).toHaveClass('opacity-60');

    await user.click(
      screen.getByRole('button', { name: /recover acme\/api/i }),
    );

    expect(
      screen.getByRole('dialog', { name: /recover repository/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Repo acme\/api could not sync/i),
    ).toBeInTheDocument();
  });

  it('keeps the degraded row button navigation label aligned with its action', async () => {
    const user = userEvent.setup();
    render(<PRListTable prs={[pr]} isLoading={false} error={null} />);

    expect(
      screen.queryByRole('button', { name: /open recovery for acme\/api/i }),
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: /open PR #42 in acme\/api/i }),
    );

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-1' },
    });
    expect(
      screen.queryByRole('dialog', { name: /recover repository/i }),
    ).not.toBeInTheDocument();
  });
});
