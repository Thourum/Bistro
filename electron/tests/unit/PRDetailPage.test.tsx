// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PRDetailPage } from '../../src/renderer/pages/PRDetailPage';
import * as storeModule from '../../src/renderer/store';

const mockPR = {
  id: 'github-1',
  provider_id: 'github:default',
  owner: 'acme',
  repo: 'widgets',
  number: 42,
  title: 'Ship new feature',
  author: 'alice',
  state: 'open' as const,
  review_status: 'pending',
  created_at: '2025-04-08T10:00:00Z',
  updated_at: '2025-04-08T14:00:00Z',
  base_branch: 'main',
  head_branch: 'feat/new',
  additions: 10,
  deletions: 2,
  changed_files: 3,
  body: 'Adds new widget',
};

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [mockPR], isLoading: false, error: null }),
}));

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ request: vi.fn().mockResolvedValue({}) }),
}));

vi.mock('../../src/renderer/hooks/useCIChecks', () => ({
  useCIChecks: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock('../../src/renderer/hooks/useComments', () => ({
  useComments: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock('../../src/renderer/hooks/useDiffFiles', () => ({
  useDiffFiles: () => ({ data: [], isLoading: false, error: null }),
}));

expect.extend(matchers);

const mockStore = {
  state: {
    selectedPRIndex: 0,
    filterState: 'open' as const,
    confirmingAction: null,
    pendingAction: null,
    actionResult: null,
  },
  subscribe: vi.fn(() => () => {}),
};

describe('PRDetailPage', () => {
  beforeEach(() => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the PR title', () => {
    render(<PRDetailPage />);
    expect(screen.getByText('Ship new feature')).toBeInTheDocument();
  });

  it('shows the SPACE-to-diff footer hint', () => {
    render(<PRDetailPage />);
    expect(screen.getByText(/view diff/i)).toBeInTheDocument();
  });

  it('shows not-found state when no PR is selected', () => {
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue({
      ...mockStore,
      state: { ...mockStore.state, selectedPRIndex: 99 },
    } as unknown as typeof storeModule.appStore);
    render(<PRDetailPage />);
    expect(screen.getByText(/PR not found/i)).toBeInTheDocument();
  });
});
