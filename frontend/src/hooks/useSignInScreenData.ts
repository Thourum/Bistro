import type { EngineStatus } from '@renderer/types/electron/engine';
import { useEffect, useState } from 'react';
import { appStore } from '../store';
import {
  type AuthScreenState,
  getAuthScreenState,
} from '../utils/authScreenState';
import { useStore } from './useStore';

const TIMER_TICK_MS = 1_000;
const SECONDS_PER_MINUTE = 60;

interface SignInScreenData {
  readonly screen: AuthScreenState;
  readonly elapsed: string;
  readonly isConnected: boolean;
  readonly authLoading: boolean;
  readonly authError: string | null;
  readonly authExchange: 'proxy' | 'local' | null;
  readonly engineStatus: EngineStatus | null;
  readonly appVersion: string | null;
  readonly lastSyncAt: string | null;
  readonly syncError: string | null;
  readonly platform: string;
}

function formatElapsed(seconds: number): string {
  const minutes = String(Math.floor(seconds / SECONDS_PER_MINUTE));
  const rest = String(seconds % SECONDS_PER_MINUTE);
  return `${minutes.padStart(2, '0')}:${rest.padStart(2, '0')}`;
}

/** Reactive view data for the sign-in screen, including the waiting timer. */
export function useSignInScreenData(): SignInScreenData {
  const authLoading = useStore(appStore, (state) => state.authLoading);
  const authUrl = useStore(appStore, (state) => state.authUrl);
  const authStartedAt = useStore(appStore, (state) => state.authStartedAt);
  const authError = useStore(appStore, (state) => state.authError);
  const authTimedOut = useStore(appStore, (state) => state.authTimedOut);
  const authInterrupted = useStore(appStore, (state) => state.authInterrupted);
  const authExchange = useStore(appStore, (state) => state.authExchange);
  const engineStatus = useStore(appStore, (state) => state.engineStatus);
  const appVersion = useStore(appStore, (state) => state.appVersion);
  const lastSyncAt = useStore(appStore, (state) => state.lastSyncAt);
  const syncError = useStore(appStore, (state) => state.syncError);
  const isConnected = true;
  const [now, setNow] = useState(Date.now);
  const screen = getAuthScreenState({
    isConnected,
    authLoading,
    authUrl,
    authError,
    authTimedOut,
    authInterrupted,
  });

  // The timer starts at AUTH_REDIRECT (authStartedAt), not at click time.
  useEffect(() => {
    if (screen !== 'waiting' || authStartedAt === null) return;
    setNow(Date.now());
    const interval = window.setInterval(
      () => setNow(Date.now()),
      TIMER_TICK_MS,
    );
    return () => window.clearInterval(interval);
  }, [screen, authStartedAt]);

  const seconds =
    authStartedAt === null
      ? 0
      : Math.max(0, Math.floor((now - authStartedAt) / TIMER_TICK_MS));
  return {
    screen,
    elapsed: formatElapsed(seconds),
    isConnected,
    authLoading,
    authError,
    authExchange,
    engineStatus,
    appVersion,
    lastSyncAt,
    syncError,
    platform: window.electron?.platform ?? 'unknown',
  };
}
