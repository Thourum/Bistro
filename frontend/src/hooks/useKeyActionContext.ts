import { SLOW_ENGINE_MS } from '@renderer/types/electron/engine';
import { useRouterState } from '@tanstack/react-router';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import {
  type FirstRunScreen,
  getAuthScreenState,
} from '../utils/authScreenState';
import type { KeyActionContext } from '../utils/keybindingActions';
import {
  type BindingActivationContext,
  hasListEscapeWork,
} from '../utils/keybindingActivation';
import { type KeyBinding, routeBaseFromPath } from '../utils/keybindings';
import { SANDBOX_TASKS } from '../utils/onboardingSandbox';
import { useStore } from './useStore';

// Re-evaluates the slow-start threshold without a store push.
const TIMER_TICK_MS = 1_000;

function routePrId(
  matches: readonly { params?: Record<string, unknown> }[] | undefined,
): string | undefined {
  if (matches === undefined) return undefined;
  for (let i = matches.length - 1; i >= 0; i -= 1) {
    const prId = matches[i]?.params?.prId;
    if (typeof prId === 'string') return prId;
  }
  return undefined;
}

/** Shared subscriptions keep footer hints and keyboard dispatch on the same gate. */
export function useBindingActivationContext(
  continueKeys: KeyBinding['keys'] | undefined,
): {
  activation: BindingActivationContext;
  overlays: ReturnType<typeof useOverlays>;
  routerState: ReturnType<typeof useRouterState>;
} {
  const overlays = useOverlays();
  const sandboxPane = useStore(
    appStore,
    (s) => s.onboardingSandbox?.pane ?? null,
  );
  const authLoading = useStore(appStore, (s) => s.authLoading);
  const authUrl = useStore(appStore, (s) => s.authUrl);
  const authError = useStore(appStore, (s) => s.authError);
  const authTimedOut = useStore(appStore, (s) => s.authTimedOut);
  const authInterrupted = useStore(appStore, (s) => s.authInterrupted);
  const authChecked = useStore(appStore, (s) => s.authChecked);
  const isConnected = useStore(appStore, (s) => s.isWsOpen);
  const engineStatus = useStore(appStore, (s) => s.engineStatus);
  const routerState = useRouterState();

  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (authChecked || engineStatus?.state !== 'starting') return;
    const interval = window.setInterval(
      () => setNow(Date.now()),
      TIMER_TICK_MS,
    );
    return () => window.clearInterval(interval);
  }, [authChecked, engineStatus?.state]);

  // Pre-auth and crashed screens render over any URL, so derive the route from state.
  const pathRouteBase = routeBaseFromPath(routerState.location.pathname);
  const authScreen = getAuthScreenState({
    isConnected,
    authLoading,
    authUrl,
    authError,
    authTimedOut,
    authInterrupted,
  });
  const isCrashed = engineStatus?.state === 'crashed';
  const routeBase = isCrashed || !authChecked ? 'connecting' : pathRouteBase;
  const isSlow =
    engineStatus?.state === 'starting' &&
    now - engineStatus.startedAt >= SLOW_ENGINE_MS;
  const screen: FirstRunScreen = isCrashed
    ? 'crashed'
    : !authChecked
      ? isSlow
        ? 'slow'
        : 'connecting'
      : authScreen;
  const isSpawned = engineStatus?.isSpawned === true;
  const { activeOverlay } = overlays;
  const onboardingStep = useStore(appStore, (s) => s.onboardingStep);
  const isScanning = useStore(
    appStore,
    (s) => s.onboardingSync?.folder.status === 'scanning',
  );
  const isChoosingFolder = useStore(
    appStore,
    (s) => s.onboardingSync?.folder.status === 'choosing',
  );
  const isPracticeComplete = useStore(
    appStore,
    (s) =>
      s.onboardingSandbox !== null &&
      SANDBOX_TASKS.every((task) =>
        s.onboardingSandbox?.completed.includes(task.id),
      ),
  );
  const listEscapeWork = useStore(appStore, hasListEscapeWork);
  const onboardingComplete = useStore(
    settingsStore,
    (s) => s.appearance.onboardingComplete,
  );
  const tourComplete = useStore(
    settingsStore,
    (s) => s.appearance.tourComplete,
  );
  const isFirstRunRailVisible = onboardingComplete && !tourComplete;
  const activation = useMemo<BindingActivationContext>(
    () => ({
      routeBase,
      activeOverlay,
      sandboxPane,
      screen,
      isSpawned,
      isConnected,
      onboardingStep,
      isScanning,
      isChoosingFolder,
      isPracticeComplete,
      continueKeys,
      isFirstRunRailVisible,
      hasListEscapeWork: listEscapeWork,
    }),
    [
      routeBase,
      activeOverlay,
      sandboxPane,
      screen,
      isSpawned,
      isConnected,
      onboardingStep,
      isScanning,
      isChoosingFolder,
      isPracticeComplete,
      continueKeys,
      isFirstRunRailVisible,
      listEscapeWork,
    ],
  );
  return { activation, overlays, routerState };
}

/**
 * Hotkeys and palette actions share the effective screen route and a stable
 * factory reading the latest overlay, location and PR matches through a ref.
 */
export function useKeyActionContext(
  continueKeys: KeyBinding['keys'] | undefined,
): {
  activation: BindingActivationContext;
  makeCtx: () => KeyActionContext;
} {
  const { activation, overlays, routerState } =
    useBindingActivationContext(continueKeys);
  const latestRef = useRef({ activation, overlays, routerState });
  latestRef.current = { activation, overlays, routerState };

  const makeCtx = useCallback((): KeyActionContext => {
    const {
      activation: currentActivation,
      overlays: current,
      routerState: state,
    } = latestRef.current;
    const context: KeyActionContext = {
      store: appStore,
      routeBase: currentActivation.routeBase,
      activeOverlay: currentActivation.activeOverlay,
      overlays: current,
      location: state.location,
    };
    const prId = routePrId(state.matches);
    return prId === undefined ? context : { ...context, prId };
  }, []);
  return { activation, makeCtx };
}
