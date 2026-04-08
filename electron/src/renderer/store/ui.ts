import { Store } from '@tanstack/store';

export interface UIState {
  selectedPRIndex: number;
  searchQuery: string;
  mode: 'list' | 'detail' | 'search' | 'help';
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all';
}

const initialUIState: UIState = {
  selectedPRIndex: 0,
  searchQuery: '',
  mode: 'list',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
};

export function createUIStore(): Store<UIState> {
  return new Store<UIState>(initialUIState);
}
