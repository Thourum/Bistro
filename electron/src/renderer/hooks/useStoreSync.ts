import { useEffect } from 'react';
import type { AppStore } from '../store';
import type { UIState } from '../store/ui';
import { createLogger } from '../utils/logger';
import { useStore } from './useStore';

const log = createLogger('StoreSync');

export function useStoreSync(store: AppStore) {
  const selectedPRIndex = useStore(store, (s: UIState) => s.selectedPRIndex);
  const filterState = useStore(store, (s: UIState) => s.filterState);
  const sortBy = useStore(store, (s: UIState) => s.sortBy);

  // Sync to localStorage for persistence
  useEffect(() => {
    try {
      localStorage.setItem(
        'bistro-ui-state',
        JSON.stringify({ selectedPRIndex, filterState, sortBy }),
      );
    } catch (e) {
      log.warn({ err: e }, 'failed to save state to localStorage');
    }
  }, [selectedPRIndex, filterState, sortBy]);

  // Load from localStorage on mount
  useEffect(() => {
    try {
      const saved = localStorage.getItem('bistro-ui-state');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed.selectedPRIndex !== undefined) {
          store.setState((s: UIState) => ({
            ...s,
            selectedPRIndex: parsed.selectedPRIndex,
          }));
        }
        if (parsed.filterState !== undefined) {
          store.setState((s: UIState) => ({
            ...s,
            filterState: parsed.filterState,
          }));
        }
        if (parsed.sortBy !== undefined) {
          store.setState((s: UIState) => ({
            ...s,
            sortBy: parsed.sortBy,
          }));
        }
      }
    } catch (e) {
      log.warn({ err: e }, 'failed to load state from localStorage');
    }
  }, [store.setState]);
}
