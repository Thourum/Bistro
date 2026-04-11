// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { expect, vi } from 'vitest';
import { PRDetailPage } from '../../src/renderer/pages/PRDetailPage';
import * as storeModule from '../../src/renderer/store';

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [], isLoading: false, error: null }),
}));

expect.extend(matchers);

const mockStore = {
  state: {
    selectedPRIndex: 0,
    filterState: 'open',
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

  it('renders PR detail page', () => {
    render(<PRDetailPage />);
    expect(
      screen.getByRole('heading', { name: /PR Details/i }),
    ).toBeInTheDocument();
  });

  it('renders within MainLayout', () => {
    const { container } = render(<PRDetailPage />);
    expect(
      container.querySelector('[data-testid="main-layout"]'),
    ).toBeInTheDocument();
  });

  it('displays diff viewer', () => {
    render(<PRDetailPage />);
    expect(screen.getByText(/No changes/i)).toBeInTheDocument();
  });

  it('renders ActionFeedback when pendingAction is set', () => {
    const storeWithPending = {
      ...mockStore,
      state: {
        ...mockStore.state,
        pendingAction: { type: 'approve', prId: 'pr-1', startedAt: Date.now() },
      },
    };
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      storeWithPending as unknown as typeof storeModule.appStore,
    );
    render(<PRDetailPage />);
    expect(screen.getByText(/approving/i)).toBeInTheDocument();
  });
});
