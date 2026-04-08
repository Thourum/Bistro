import { Store } from '@tanstack/store';

export interface UIState {
  selectedPRIndex: number;
  selectedFileIndex: number;
  searchQuery: string;
  mode: 'list' | 'detail' | 'search' | 'help';
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all';
  confirmingAction: string | null;
  filterMenuOpen: boolean;
}

const initialUIState: UIState = {
  selectedPRIndex: 0,
  selectedFileIndex: 0,
  searchQuery: '',
  mode: 'list',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
  confirmingAction: null,
  filterMenuOpen: false,
};

export function createUIStore(): Store<UIState> {
  return new Store<UIState>(initialUIState);
}
