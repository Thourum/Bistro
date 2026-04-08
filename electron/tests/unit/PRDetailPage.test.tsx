// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { expect, vi } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { PRDetailPage } from '../../src/renderer/pages/PRDetailPage';
import * as storeModule from '../../src/renderer/store';

expect.extend(matchers);

const mockStore = {
  state: { selectedPRIndex: 0, filterState: 'open' },
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
    expect(screen.getByRole('heading', { name: /PR Details/i })).toBeInTheDocument();
  });

  it('renders within MainLayout', () => {
    const { container } = render(<PRDetailPage />);
    expect(container.querySelector('[data-testid="main-layout"]')).toBeInTheDocument();
  });

  it('displays diff viewer', () => {
    render(<PRDetailPage />);
    expect(screen.getByText(/No changes/i)).toBeInTheDocument();
  });
});
