import { useEffect } from 'react';
import { subscribeEngineEvents } from '../services/events';
import { runStartupDevFolderScan } from '../services/onboardingSync';
import { checkAuth } from '../services/signIn';
import { appStore } from '../store';

// StrictMode replays effects, but a second scan would cancel the first one.
const startup = { dispatched: false };

/** Keeps engine events subscribed across routes and finishes startup checks. */
export function useEngine(): void {
  useEffect(() => {
    const unsubscribe = subscribeEngineEvents();
    if (!startup.dispatched) {
      startup.dispatched = true;
      appStore.setState((state) => ({ ...state, syncStatus: 'connected' }));
      void checkAuth();
      runStartupDevFolderScan();
    }
    return unsubscribe;
  }, []);
}
