// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: vi.fn(() => Promise.resolve({ repos: [] })),
    send: vi.fn(),
  }),
}));

import {
  PRListTable,
  visiblePRListRows,
} from '../../src/renderer/components/PRListTable';
import * as storeModule from '../../src/renderer/store';
import type { PR } from '../../src/renderer/types';

expect.extend(matchers);

const mockPR: PR = {
  id: 'github-1',
  provider: 'github',
  owner: 'acme',
  repo: 'widgets',
  number: 42,
  title: 'Fix auth bug',
  body: '',
  author: 'alice',
  state: 'open' as const,
  created_at: '2025-04-08T10:00:00Z',
  updated_at: '2025-04-08T14:00:00Z',
  merged_at: null,
  base_branch: 'main',
  head_branch: 'fix/auth',
  base_sha: 'base-sha',
  head_sha: 'head-sha',
  comment_count: 0,
  ci_status: null,
  draft: 0,
  review_status: 'pending',
  approved_by: null,
  changes_requested_by: null,
  requested_reviewers: [],
};

const mockStore = {
  state: {
    selectedPRIndex: 0,
    repoFilter: null,
    repos: [{ owner: 'acme', repo: 'widgets', watched: true }],
    reposLoading: false,
    filterState: 'open' as const,
    sortBy: 'updated' as const,
    searchQuery: '',
    collapsedRepoKeys: [] as string[],
  },
  setState: vi.fn((updater) => {
    const nextState =
      typeof updater === 'function' ? updater(mockStore.state) : updater;
    mockStore.state = nextState;
  }),
  subscribe: vi.fn(() => () => {}),
};

describe('PRListTable', () => {
  beforeEach(() => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );
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
    const btn = screen.getByRole('button', {
      name: /Open PR #42 in acme\/widgets/,
    });
    expect(btn).toBeInTheDocument();
  });

  it('groups PR rows by repository', () => {
    const otherPR = {
      ...mockPR,
      id: 'github-2',
      repo: 'api',
      number: 43,
      title: 'Add endpoint',
    };
    const laterWidgetPR = {
      ...mockPR,
      id: 'github-3',
      number: 44,
      title: 'Fix widget tests',
    };

    render(
      <PRListTable
        prs={[mockPR, otherPR, laterWidgetPR]}
        isLoading={false}
        error={null}
      />,
    );

    expect(screen.getByText('acme/widgets')).toBeInTheDocument();
    expect(screen.getByText('2 PRS')).toBeInTheDocument();
    expect(screen.getByText('acme/api')).toBeInTheDocument();
    expect(screen.getByText('1 PR')).toBeInTheDocument();
  });

  it('exposes the rendered repo-grouped order for keyboard navigation', () => {
    const otherPR = { ...mockPR, id: 'github-2', repo: 'api' };
    const laterWidgetPR = { ...mockPR, id: 'github-3', number: 44 };

    expect(
      visiblePRListRows([mockPR, otherPR, laterWidgetPR], []).map(
        (r) => r.pr?.id,
      ),
    ).toEqual(['github-1', 'github-3', 'github-2']);
  });

  it('collapses a repo group to a single header row', () => {
    const otherPR = { ...mockPR, id: 'github-2', repo: 'api' };
    const laterWidgetPR = { ...mockPR, id: 'github-3', number: 44 };

    const rows = visiblePRListRows(
      [mockPR, otherPR, laterWidgetPR],
      ['acme/widgets'],
    );
    expect(rows).toEqual([
      { pr: null, repoKey: 'acme/widgets' },
      { pr: otherPR, repoKey: 'acme/api' },
    ]);
  });

  it('hides PR rows of a collapsed group', () => {
    mockStore.state.collapsedRepoKeys = ['acme/widgets'];
    render(<PRListTable prs={[mockPR]} isLoading={false} error={null} />);
    expect(screen.queryByText('Fix auth bug')).not.toBeInTheDocument();
    expect(screen.getByText('acme/widgets')).toBeInTheDocument();
    mockStore.state.collapsedRepoKeys = [];
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
