// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PRListTable } from '../../src/renderer/components/PRListTable';
import * as storeModule from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import type { PR } from '../../src/renderer/types';

expect.extend(matchers);

const mockPR: PR = {
  id: 'github-1',
  provider_id: 'github:default',
  owner: 'acme',
  repo: 'widgets',
  number: 42,
  title: 'Fix auth bug',
  author: 'alice',
  state: 'open' as const,
  review_status: 'pending',
  created_at: '2025-04-08T10:00:00Z',
  updated_at: '2025-04-08T14:00:00Z',
  base_branch: 'main',
  head_branch: 'fix/auth',
  additions: 10,
  deletions: 2,
  changed_files: 3,
  body: '',
};

const mockStore = {
  state: {
    selectedPRIndex: 0,
    repoFilter: null,
    repos: [{ owner: 'acme', repo: 'widgets', watched: true }],
    filterState: 'open' as const,
    sortBy: 'updated' as const,
    searchQuery: '',
  },
  subscribe: vi.fn(() => () => {}),
};

describe('PRListTable', () => {
  beforeEach(() => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );

    // Ensure flat view.
    settingsStore.setState((s) => ({
      ...s,
      prList: { ...s.prList, view: 'flat' },
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the PR title and metadata', () => {
    render(<PRListTable prs={[mockPR]} isLoading={false} error={null} />);
    expect(screen.getByText('Fix auth bug')).toBeInTheDocument();
    expect(screen.getByText(/#42/)).toBeInTheDocument();
    expect(screen.getByText('@alice')).toBeInTheDocument();
  });

  it('exposes the CI status via aria-label', () => {
    render(<PRListTable prs={[mockPR]} isLoading={false} error={null} />);
    expect(screen.getByLabelText(/CI /i)).toBeInTheDocument();
  });

  it('renders the row as a button for keyboard access', () => {
    render(<PRListTable prs={[mockPR]} isLoading={false} error={null} />);
    const btn = screen.getByRole('button', { name: /Fix auth bug/ });
    expect(btn).toBeInTheDocument();
  });

  it('shows loading state', () => {
    render(<PRListTable prs={[]} isLoading={true} error={null} />);
    expect(screen.getByText(/Loading PRs/)).toBeInTheDocument();
  });

  it('shows error state', () => {
    render(
      <PRListTable prs={[]} isLoading={false} error={new Error('oops')} />,
    );
    expect(screen.getByText(/Failed to load PRs: oops/)).toBeInTheDocument();
  });

  it('handles empty PR list with no watched repos', () => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue({
      ...mockStore,
      state: { ...mockStore.state, repos: [] },
    } as unknown as typeof storeModule.appStore);

    render(<PRListTable prs={[]} isLoading={false} error={null} />);
    expect(screen.getByText('No repositories watched')).toBeInTheDocument();
  });
});
