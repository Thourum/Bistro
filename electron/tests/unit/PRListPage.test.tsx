// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FooterSlotProvider } from '../../src/renderer/components/FooterSlot';
import * as prListModule from '../../src/renderer/hooks/useWebSocketPRList';
import { PRListPage } from '../../src/renderer/pages/PRListPage';
import * as storeModule from '../../src/renderer/store';

expect.extend(matchers);

const mockPR = {
  id: 'github-1',
  provider_id: 'github:default',
  owner: 'acme',
  repo: 'widgets',
  number: 1,
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
    filterState: 'open' as const,
    sortBy: 'updated' as const,
    searchQuery: '',
    repoFilter: null,
    repos: [{ owner: 'acme', repo: 'widgets', watched: true }],
    connectedProviders: [],
    visiblePRIds: [],
  },
  subscribe: vi.fn(() => () => {}),
  setState: vi.fn(),
};

describe('PRListPage', () => {
  beforeEach(() => {
    vi.spyOn(prListModule, 'useWebSocketPRList').mockReturnValue({
      data: [mockPR],
      isLoading: false,
      error: null,
    } as ReturnType<typeof prListModule.useWebSocketPRList>);

    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the PR title from the redesigned list', () => {
    render(
      <FooterSlotProvider>
        <PRListPage />
      </FooterSlotProvider>,
    );
    expect(screen.getByText('Fix auth bug')).toBeInTheDocument();
  });

  it('shows the inbox breadcrumb and filter heading', () => {
    render(
      <FooterSlotProvider>
        <PRListPage />
      </FooterSlotProvider>,
    );
    expect(screen.getByText('bistro')).toBeInTheDocument();
    expect(screen.getByText('inbox')).toBeInTheDocument();
    expect(screen.getByText('Open Pull Requests')).toBeInTheDocument();
  });

  it('renders the floating filter/sort card', () => {
    render(
      <FooterSlotProvider>
        <PRListPage />
      </FooterSlotProvider>,
    );
    expect(screen.getByText(/IS:OPEN/)).toBeInTheDocument();
    expect(screen.getByText(/UPDATED_FIRST/)).toBeInTheDocument();
  });

  it('accepts routeSearch prop and uses route-authoritative values', () => {
    render(
      <FooterSlotProvider>
        <PRListPage
          routeSearch={{ filter: 'draft', sort: 'created', q: '' }}
        />
      </FooterSlotProvider>,
    );
    expect(screen.getByText('Draft Pull Requests')).toBeInTheDocument();
    expect(screen.getByText(/IS:DRAFT/)).toBeInTheDocument();
    expect(screen.getByText(/CREATED_FIRST/)).toBeInTheDocument();
  });

  it('has pr-list-page testid', () => {
    render(
      <FooterSlotProvider>
        <PRListPage />
      </FooterSlotProvider>,
    );
    expect(screen.getByTestId('pr-list-page')).toBeInTheDocument();
  });
});
