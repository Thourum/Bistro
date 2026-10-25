import {
  getSequenceManager,
  type HotkeySequence,
  type RegisterableHotkey,
} from '@tanstack/hotkeys';
import {
  type UseHotkeyDefinition,
  type UseHotkeySequenceDefinition,
  useHotkeySequences,
  useHotkeys,
} from '@tanstack/react-hotkeys';
import { useRouterState } from '@tanstack/react-router';
import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { dispatchSandboxAction } from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import {
  getBindingAction,
  type KeyActionContext,
} from '../utils/keybindingActions';
import {
  continuesSequence,
  hotkeyOptions,
  isEscape,
  isStyleCycle,
  isStylePickerTarget,
  yieldsToFocusedControl,
} from '../utils/keybindingActivation';
import {
  getEffectiveBindings,
  isBindingEnabled,
  isSequenceBinding,
  routeBaseFromPath,
} from '../utils/keybindings';
import { SEQUENCE_TIMEOUT_MS } from '../utils/keymaps';
import { SANDBOX_ACTION_IDS } from '../utils/onboardingSandbox';
import { useStore } from './useStore';

/** Sandbox actions on the onboarding route drive the fake PR, not the app. */
function runBinding(actionId: string, context: KeyActionContext): void {
  if (
    context.routeBase === 'onboarding' &&
    context.activeOverlay === null &&
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

/**
 * Registers every effective keybinding (defaults + settings overrides)
 * through TanStack Hotkeys. Bindings are route+overlay-gated via the `enabled`
 * option; overlapping keys across contexts are intentional, hence
 * `conflictBehavior: 'allow'`.
 *
 * Must be rendered inside both OverlayProvider and a TanStack RouterProvider.
 *
 * Escape stays active in inputs; Emacs cancel also stays active in overlay
 * inputs. Review composers keep their local Escape cleanup.
 */
export function useAppHotkeys(): void {
  const overlays = useOverlays();
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const sandboxPane = useStore(
    appStore,
    (s) => s.onboardingSandbox?.pane ?? null,
  );
  const routerState = useRouterState();

  const routeBase = routeBaseFromPath(routerState.location.pathname);
  const { activeOverlay } = overlays;

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
  // Weak membership identifies this dispatch without retaining DOM targets.
  const midSequenceEventsRef = useRef(new WeakSet<KeyboardEvent>());

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

  const bindings = useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );

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
            if (midSequenceEventsRef.current.has(event)) return;
            if (binding.actionId === 'detail.approve' && event.repeat) return;
            if (isStyleCycle(binding)) {
              if (!isStylePickerTarget(event.target)) return;
              // Tab selects a style but still moves focus to the next control.
            } else {
              if (yieldsToFocusedControl(binding, event.target)) return;
              if (
                binding.keys === 'Enter' ||
                binding.keys === 'Space' ||
                binding.keys === 'Tab' ||
                binding.keys === 'Shift+Tab'
              )
                event.preventDefault();
            }
            runBinding(binding.actionId, makeCtx());
          },
          options: {
            enabled: isBindingEnabled(binding, {
              routeBase,
              activeOverlay,
              sandboxPane,
            }),
            ...hotkeyOptions(binding, activeOverlay),
            ...(isStyleCycle(binding) ? { stopPropagation: false } : {}),
          },
        })),
    [bindings, routeBase, activeOverlay, sandboxPane, makeCtx],
  );

  const sequenceDefs = useMemo<UseHotkeySequenceDefinition[]>(
    () =>
      bindings.filter(isSequenceBinding).map((binding) => ({
        sequence: [...binding.keys] as HotkeySequence,
        callback: () => runBinding(binding.actionId, makeCtx()),
        options: {
          enabled: isBindingEnabled(binding, {
            routeBase,
            activeOverlay,
            sandboxPane,
          }),
          ignoreInputs: true,
        },
      })),
    [bindings, routeBase, activeOverlay, sandboxPane, makeCtx],
  );

  const timeoutMs = SEQUENCE_TIMEOUT_MS[style];

  // Read TanStack's public progress before its document listeners dispatch.
  // Unlike a parallel history, this never counts keys the manager ignored.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent): void => {
      if (
        continuesSequence(event, {
          sequences: [...getSequenceManager().registrations.state.values()],
          timeoutMs,
          now: Date.now(),
        })
      )
        midSequenceEventsRef.current.add(event);
    };
    window.addEventListener('keydown', onKeyDown, { capture: true });
    return () => {
      window.removeEventListener('keydown', onKeyDown, { capture: true });
      midSequenceEventsRef.current = new WeakSet();
    };
  }, [timeoutMs]);

  useHotkeys(hotkeyDefs, { conflictBehavior: 'allow' });
  useHotkeySequences(sequenceDefs, {
    conflictBehavior: 'allow',
    timeout: SEQUENCE_TIMEOUT_MS[style],
  });
}
