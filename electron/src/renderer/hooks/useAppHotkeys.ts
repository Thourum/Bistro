import type { HotkeySequence, RegisterableHotkey } from '@tanstack/hotkeys';
import {
  type UseHotkeyDefinition,
  type UseHotkeySequenceDefinition,
  useHotkeySequences,
  useHotkeys,
} from '@tanstack/react-hotkeys';
import { useRouterState } from '@tanstack/react-router';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { SLOW_ENGINE_MS } from '../../types/engine';
import { useOverlays } from '../overlays/OverlayContext';
import {
  closeOnboardingSandboxComposer,
  dispatchSandboxAction,
} from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import {
  type FirstRunScreen,
  getAuthScreenState,
} from '../utils/authScreenState';
import {
  getBindingAction,
  type KeyActionContext,
} from '../utils/keybindingActions';
import {
  type BindingActivationContext,
  blurOnboardingRepoFilter,
  continuesSequence,
  hasListEscapeWork,
  hotkeyOptions,
  isEscape,
  isModifierPress,
  isSandboxComposerTarget,
  type KeyPress,
  yieldsToFocusedControl,
} from '../utils/keybindingActivation';
import {
  getEffectiveBindings,
  isBindingEnabled,
  isSequenceBinding,
  type KeyBinding,
  routeBaseFromPath,
} from '../utils/keybindings';
import { SEQUENCE_TIMEOUT_MS } from '../utils/keymaps';
import { SANDBOX_ACTION_IDS, SANDBOX_TASKS } from '../utils/onboardingSandbox';
import { useStore } from './useStore';

// Re-evaluates the slow-start threshold without a store push.
const TIMER_TICK_MS = 1_000;

/** Sandbox actions on the onboarding route drive the fake PR, not the app. */
function runBinding(actionId: string, context: KeyActionContext): void {
  if (
    context.routeBase === 'onboarding' &&
    context.activeOverlay === null &&
    appStore.state.onboardingStep === 'practice' &&
    appStore.state.onboardingSandbox !== null &&
    SANDBOX_ACTION_IDS.has(actionId)
  ) {
    dispatchSandboxAction(actionId);
    return;
  }
  getBindingAction(actionId)?.(context);
}

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
function useBindingActivationContext(
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

/** Allows footer hints to use exactly the same runtime gate as registration. */
export function useBindingEnabled(binding: KeyBinding | undefined): boolean {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const bindings = useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );
  const continueKeys = bindings.find(
    (candidate) => candidate.actionId === 'onboarding.continue',
  )?.keys;
  const { activation } = useBindingActivationContext(continueKeys);
  return binding !== undefined && isBindingEnabled(binding, activation);
}

/** Registers all effective bindings; runtime gates give overlapping keys one owner. */
export function useAppHotkeys(): void {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const bindings = useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );
  const continueKeys = bindings.find(
    (binding) => binding.actionId === 'onboarding.continue',
  )?.keys;
  const { activation, overlays, routerState } =
    useBindingActivationContext(continueKeys);
  const { routeBase, activeOverlay } = activation;
  const activationRef = useRef(activation);
  activationRef.current = activation;

  // Use refs so callbacks always read current values without needing to
  // be recreated on every render (prevents stale closure issues).
  const overlaysRef = useRef(overlays);
  overlaysRef.current = overlays;
  const routeBaseRef = useRef(routeBase);
  routeBaseRef.current = routeBase;
  const activeOverlayRef = useRef(activeOverlay);
  activeOverlayRef.current = activeOverlay;
  const locationRef = useRef(routerState.location);
  locationRef.current = routerState.location;
  const matchesRef = useRef(routerState.matches);
  matchesRef.current = routerState.matches;
  // A keydown that advanced an enabled sequence; its single-key binding skips it.
  const midSequenceEventRef = useRef<KeyboardEvent | null>(null);

  // Stable makeCtx that always reads current values from refs.
  const makeCtx = useCallback((): KeyActionContext => {
    const prId = routePrId(matchesRef.current);
    const ctx: KeyActionContext = {
      store: appStore,
      routeBase: routeBaseRef.current,
      activeOverlay: activeOverlayRef.current,
      overlays: overlaysRef.current,
      location: locationRef.current,
    };
    return prId === undefined ? ctx : { ...ctx, prId };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const hotkeyDefs = useMemo<UseHotkeyDefinition[]>(
    () =>
      bindings
        .filter((b) => !isSequenceBinding(b))
        .map((binding) => ({
          hotkey: binding.keys as RegisterableHotkey,
          callback: (event: KeyboardEvent) => {
            // Escape only: bail when a handler closer to the event already
            // consumed it.
            //
            // Radix's dismissable layer listens in the document CAPTURE phase
            // and calls preventDefault() when it dismisses. Escape is a
            // discrete event, so React flushes that state update *between* the
            // capture and bubble phases — and `useHotkeys` re-syncs
            // `handle.callback` on every render. So by the time this
            // bubble-phase callback runs it is a fresh closure that already
            // sees `activeOverlay: null`, and `global.escape` fell through to
            // its route branch: one keypress both closed the overlay and
            // navigated out of the route.
            //
            // Capturing state at registration time cannot fix that (the
            // callback is replaced), so we read the event instead. This
            // requires `preventDefault: false` below — the manager otherwise
            // calls preventDefault() itself before every callback, which would
            // make this flag always true.
            if (isEscape(binding) && event.defaultPrevented) return;
            if (event === midSequenceEventRef.current) return;
            const currentActivation = activationRef.current;
            if (!isBindingEnabled(binding, currentActivation)) return;
            if (
              isEscape(binding) &&
              currentActivation.routeBase === 'onboarding' &&
              currentActivation.activeOverlay === null &&
              currentActivation.onboardingStep === 'sync' &&
              blurOnboardingRepoFilter(event.target)
            ) {
              event.preventDefault();
              return;
            }
            // Escape in a fake composer cancels it, not the sandbox's Back.
            if (
              isEscape(binding) &&
              currentActivation.routeBase === 'onboarding' &&
              currentActivation.onboardingStep === 'practice' &&
              isSandboxComposerTarget(event.target)
            ) {
              closeOnboardingSandboxComposer();
              event.preventDefault();
              return;
            }
            if (yieldsToFocusedControl(binding, event.target)) return;
            // Continue opts out of the manager's preventDefault (see
            // `hotkeyOptions`), so stop Space scrolling the page here.
            if (binding.actionId === 'onboarding.continue')
              event.preventDefault();
            runBinding(binding.actionId, makeCtx());
            // A store update can re-enable another Escape owner during this event.
            if (isEscape(binding)) event.preventDefault();
          },
          options: {
            enabled: isBindingEnabled(binding, activation),
            ...hotkeyOptions(binding),
          },
        })),
    [bindings, activation, makeCtx],
  );

  const sequenceDefs = useMemo<UseHotkeySequenceDefinition[]>(
    () =>
      bindings.filter(isSequenceBinding).map((binding) => ({
        sequence: [...binding.keys] as HotkeySequence,
        callback: () => {
          if (isBindingEnabled(binding, activationRef.current))
            runBinding(binding.actionId, makeCtx());
        },
        options: {
          enabled: isBindingEnabled(binding, activation),
          ignoreInputs: true,
        },
      })),
    [bindings, activation, makeCtx],
  );

  const enabledSequencesRef = useRef<readonly HotkeySequence[]>([]);
  enabledSequencesRef.current = sequenceDefs
    .filter((definition) => definition.options?.enabled === true)
    .map((definition) => definition.sequence);
  const timeoutMs = SEQUENCE_TIMEOUT_MS[style];

  // Capture phase runs before both TanStack listeners on document, so the
  // single-key callback can tell a key that continues a sequence.
  useEffect(() => {
    let recent: readonly KeyPress[] = [];
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.repeat || isModifierPress(event)) return;
      const press = { event, time: event.timeStamp };
      const sequences = enabledSequencesRef.current;
      if (continuesSequence(recent, press, { sequences, timeoutMs }))
        midSequenceEventRef.current = event;
      const longest = Math.max(1, ...sequences.map((steps) => steps.length));
      recent = [...recent, press].slice(-longest);
    };
    window.addEventListener('keydown', onKeyDown, { capture: true });
    return () =>
      window.removeEventListener('keydown', onKeyDown, { capture: true });
  }, [timeoutMs]);

  useHotkeys(hotkeyDefs, { conflictBehavior: 'allow' });
  useHotkeySequences(sequenceDefs, {
    conflictBehavior: 'allow',
    timeout: SEQUENCE_TIMEOUT_MS[style],
  });
}
