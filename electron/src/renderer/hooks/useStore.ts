import type { Store } from '@tanstack/store';
import { useSyncExternalStore } from 'react';

export function useStore<TState, TSelected>(
  store: Store<TState>,
  selector: (state: TState) => TSelected,
): TSelected {
  return useSyncExternalStore(
    (callback) => {
      // @tanstack/store <0.9 returns a cleanup fn; ≥0.9 returns { unsubscribe }
      const result = store.subscribe((_: TState) => callback());
      if (typeof result === 'function') return result as () => void;
      return () => (result as { unsubscribe: () => void }).unsubscribe();
    },
    () => selector(store.state),
    () => selector(store.state),
  );
}
