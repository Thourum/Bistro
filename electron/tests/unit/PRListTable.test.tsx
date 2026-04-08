// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { expect, vi } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { PRListTable } from '../../src/renderer/components/PRListTable';
import * as prListModule from '../../src/renderer/queries/usePRList';
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
  state: { selectedPRIndex: 0 },
  subscribe: vi.fn(() => () => {}),
};

describe('PRListTable', () => {
  beforeEach(() => {
    vi.spyOn(prListModule, 'usePRList').mockReturnValue({
      data: [mockPR],
      isLoading: false,
      error: null,
    } as ReturnType<typeof prListModule.usePRList>);

    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders PR table header', () => {
    render(<PRListTable />);
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('Title')).toBeInTheDocument();
    expect(screen.getByText('Updated')).toBeInTheDocument();
    expect(screen.getByText('Author')).toBeInTheDocument();
  });

  it('renders PR rows', () => {
    render(<PRListTable />);
    expect(screen.getByText('Fix auth bug')).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('renders state badge', () => {
    render(<PRListTable />);
    expect(screen.getByText('open')).toBeInTheDocument();
  });

  it('renders author in monospace', () => {
    render(<PRListTable />);
    const authorCell = screen.getByText('alice').closest('td');
    expect(authorCell).toHaveClass('font-mono');
  });

  it('renders updated date in monospace', () => {
    render(<PRListTable />);
    const dateCell = screen.getByText(/4\/8\/2025/);
    expect(dateCell).toHaveClass('font-mono');
  });

  it('highlights selected row with border', () => {
    const { container } = render(<PRListTable />);
    const rows = container.querySelectorAll('tbody tr');
    expect(rows[0]).toHaveClass('border-l-primary');
  });

  it('handles empty PR list gracefully', () => {
    vi.spyOn(prListModule, 'usePRList').mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as ReturnType<typeof prListModule.usePRList>);

    const { container } = render(<PRListTable />);
    const tbody = container.querySelector('tbody');
    expect(tbody?.children.length).toBe(0);
  });
});
