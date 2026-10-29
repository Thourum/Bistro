import { useEffect } from 'react';
import { subscribeEngineEvents } from '../services/events';
import { runStartupDevFolderScan } from '../services/onboardingSync';
import { checkAuth } from '../services/signIn';
import { appStore } from '../store';

/** Keeps engine events subscribed across routes and finishes startup checks. */
export function useEngine(): void {
  useEffect(() => {
    const unsubscribe = subscribeEngineEvents();
    appStore.setState((state) => ({ ...state, syncStatus: 'connected' }));
    void checkAuth();
    runStartupDevFolderScan();
    return unsubscribe;
  }, []);
}
