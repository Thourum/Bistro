// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { expect, vi } from 'vitest';
import * as prListModule from '../../src/renderer/hooks/useWebSocketPRList';
import { PRListPage } from '../../src/renderer/pages/PRListPage';
import * as storeModule from '../../src/renderer/store';

expect.extend(matchers);

const mockPR = {
  id: 'github-1',
  provider_id: 'github:default',
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
  state: { selectedPRIndex: 0, filterState: 'open' },
  subscribe: vi.fn(() => () => {}),
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

  it('renders PR list page', () => {
    render(<PRListPage />);
    expect(screen.getByText('Fix auth bug')).toBeInTheDocument();
  });

  it('renders within MainLayout', () => {
    const { container } = render(<PRListPage />);
    expect(
      container.querySelector('[data-testid="main-layout"]'),
    ).toBeInTheDocument();
  });

  it('renders toolbar with search and filter', () => {
    render(<PRListPage />);
    expect(
      screen.getByPlaceholderText('Search PRs... (/ to focus)'),
    ).toBeInTheDocument();
    expect(screen.getByText('Filter')).toBeInTheDocument();
  });
});
