import { useSyncExternalStore } from 'react';
import type { Store } from '@tanstack/store';

export function useStore<TState, TSelected>(
  store: Store<TState>,
  selector: (state: TState) => TSelected
): TSelected {
  return useSyncExternalStore(
    store.subscribe,
    () => selector(store.state),
    () => selector(store.state)
  );
}
