// RouterProvider is mounted unconditionally (after settings are ready).
// The old auth-branching gate is replaced by:
//
//   Bootstrap — non-visual component that:
//     • runs useSettingsEffect() + useWebSocket() BESIDE RouterProvider so the
//       WS is NEVER unmounted by navigation
//     • subscribes to authChecked / isAuthenticated / onboardingComplete and
//       calls router.invalidate() so route guards re-evaluate on auth changes
//
//   RouterProvider — mounted unconditionally (after settings ready).
//     Route guards in router.tsx read appStore directly and redirect as needed.
//
// The initSettings() / ready gate (Initialising screen) is kept so settings
// are available before the router first renders.

import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { ResultAsync } from 'neverthrow';
import React from 'react';
import { InitialisingScreen } from './components/ConnectingScreen';
import { FooterSlotProvider } from './components/FooterSlot';
import { addToast } from './components/ToastContainer';
import { useSettingsEffect } from './hooks/useSettingsEffect';
import { useStore } from './hooks/useStore';
import { useWebSocket } from './hooks/useWebSocket';
import { router } from './router';
import { appStore } from './store';
import { initSettings, settingsStore } from './store/settings';
import { createLogger } from './utils/logger';

const log = createLogger('App');

const queryClient = new QueryClient();

// ---------------------------------------------------------------------------
// Bootstrap — persistent non-visual component
// ---------------------------------------------------------------------------

/**
 * Runs side-effects and keeps the router in sync with auth state.
 * Lives BESIDE RouterProvider (not inside) so the WebSocket singleton is
 * never torn down by navigation — it outlives any route component.
 */
function Bootstrap(): null {
  useSettingsEffect();
  useWebSocket();

  const authChecked = useStore(appStore, (s) => s.authChecked);
  const isAuthenticated = useStore(appStore, (s) => s.isAuthenticated);
  const onboardingComplete = useStore(
    settingsStore,
    (s) => s.appearance.onboardingComplete,
  );

  // Invalidate the router whenever auth state changes so guards re-run.
  // This is what drives the reconnect-safe + auth-expired redirect flows.
  React.useEffect(() => {
    void router.invalidate();
  }, [authChecked, isAuthenticated, onboardingComplete]);

  return null;
}

// ---------------------------------------------------------------------------
// Inner — uses useQueryClient() to pass the correct client to RouterProvider
// ---------------------------------------------------------------------------

function Inner(): React.ReactElement {
  const qc = useQueryClient();
  return (
    <>
      <Bootstrap />
      <RouterProvider router={router} context={{ queryClient: qc }} />
    </>
  );
}

// ---------------------------------------------------------------------------
// App root
// ---------------------------------------------------------------------------

export default function App(): React.ReactElement {
  const [ready, setReady] = React.useState(false);

  React.useEffect(() => {
    void ResultAsync.fromPromise(initSettings(), (cause) => cause).match(
      () => setReady(true),
      (cause) => {
        log.error({ cause }, 'failed to initialize settings');
        addToast({
          type: 'error',
          message: 'Could not load settings; using defaults.',
          dismissible: true,
        });
        setReady(true);
      },
    );
  }, []);

  if (!ready) return <InitialisingScreen />;

  return (
    <QueryClientProvider client={queryClient}>
      <FooterSlotProvider>
        <Inner />
      </FooterSlotProvider>
    </QueryClientProvider>
  );
}
