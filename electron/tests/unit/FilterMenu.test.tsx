// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, vi, beforeEach, afterEach } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { FilterMenu } from '../../src/renderer/components/FilterMenu';
import * as storeModule from '../../src/renderer/store';

expect.extend(matchers);

const mockState = { filterState: 'open' as const, filterMenuOpen: false, mode: 'list' as const };
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
    stateRef = { filterState: 'open', filterMenuOpen: false, mode: 'list' };
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

  it('shows keyboard hint for filter shortcut', () => {
    render(<FilterMenu />);
    expect(screen.getByText('f')).toBeInTheDocument();
  });

  it('does not show filter panel when filterMenuOpen is false', () => {
    render(<FilterMenu />);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('shows filter panel when filterMenuOpen is true', () => {
    stateRef = { filterState: 'open', filterMenuOpen: true, mode: 'filter' };
    render(<FilterMenu />);
    expect(screen.getByRole('menu')).toBeInTheDocument();
    expect(screen.getByText('Open')).toBeInTheDocument();
    expect(screen.getByText('Draft')).toBeInTheDocument();
    expect(screen.getByText('All')).toBeInTheDocument();
  });

  it('displays filter options keyboard hints when open', () => {
    stateRef = { filterState: 'open', filterMenuOpen: true, mode: 'filter' };
    render(<FilterMenu />);
    expect(screen.getByText('o')).toBeInTheDocument();
    expect(screen.getByText('d')).toBeInTheDocument();
    expect(screen.getByText('a')).toBeInTheDocument();
  });

  it('changes filter state when option is clicked', async () => {
    stateRef = { filterState: 'open', filterMenuOpen: true, mode: 'filter' };
    render(<FilterMenu />);
    const draftOption = screen.getByText('Draft');
    await userEvent.click(draftOption);
    expect(mockStore.setState).toHaveBeenCalled();
  });

  it('toggles filter menu open state when button is clicked', async () => {
    render(<FilterMenu />);
    const button = screen.getByRole('button', { name: /Filter/i });
    await userEvent.click(button);
    expect(mockStore.setState).toHaveBeenCalled();
  });
});
