// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, vi, beforeEach, afterEach } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { FilterMenu } from '../../src/renderer/components/FilterMenu';
import * as storeModule from '../../src/renderer/store';

expect.extend(matchers);

const mockState = { filterState: 'open' as const, filterMenuOpen: false };
let stateRef = { ...mockState };

const mockStore = {
  get state() {
    return stateRef;
  },
  subscribe: vi.fn((cb: () => void) => {
    void cb;
    return () => {};
  }),
  setState: vi.fn((updater: (s: typeof stateRef) => typeof stateRef) => {
    stateRef = updater(stateRef);
  }),
};

describe('FilterMenu', () => {
  beforeEach(() => {
    stateRef = { filterState: 'open', filterMenuOpen: false };
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders filter button', () => {
    render(<FilterMenu />);
    expect(screen.getByRole('button', { name: /Filter/i })).toBeInTheDocument();
  });

  it('displays filter options in dropdown after click', async () => {
    render(<FilterMenu />);
    const button = screen.getByRole('button', { name: /Filter/i });
    await userEvent.click(button);
    expect(screen.getByText('Open')).toBeInTheDocument();
    expect(screen.getByText('Draft')).toBeInTheDocument();
    expect(screen.getByText('All')).toBeInTheDocument();
  });

  it('shows keyboard hint for filter shortcut', () => {
    render(<FilterMenu />);
    expect(screen.getByText('f')).toBeInTheDocument();
  });

  it('changes filter state when option is clicked', async () => {
    render(<FilterMenu />);
    const button = screen.getByRole('button', { name: /Filter/i });
    await userEvent.click(button);
    const draftOption = screen.getByText('Draft');
    await userEvent.click(draftOption);
    expect(mockStore.setState).toHaveBeenCalled();
  });
});
