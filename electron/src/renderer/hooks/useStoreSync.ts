import { useEffect } from 'react';
import { useStore } from './useStore';
import type { AppStore } from '../store';
import type { UIState } from '../store/ui';
import { createLogger } from '../utils/logger';

const log = createLogger('StoreSync');

export function useStoreSync(store: AppStore) {
  const selectedIndex = useStore(store, (s: UIState) => s.selectedPRIndex);

  // Sync to localStorage for persistence
  useEffect(() => {
    const state = store.state;
    try {
      localStorage.setItem('bistro-ui-state', JSON.stringify({
        selectedPRIndex: state.selectedPRIndex,
        filterState: state.filterState,
        sortBy: state.sortBy,
      }));
    } catch (e) {
      log.warn({ err: e }, 'failed to save state to localStorage');
    }
  }, [selectedIndex]);

  // Load from localStorage on mount
  useEffect(() => {
    try {
      const saved = localStorage.getItem('bistro-ui-state');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed.selectedPRIndex !== undefined) {
          store.setState((s: UIState) => ({ ...s, selectedPRIndex: parsed.selectedPRIndex }));
        }
        if (parsed.filterState !== undefined) {
          store.setState((s: UIState) => ({ ...s, filterState: parsed.filterState }));
        }
      }
    } catch (e) {
      log.warn({ err: e }, 'failed to load state from localStorage');
    }
  }, []);
}
