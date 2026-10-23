import type { HotkeySequence, RegisterableHotkey } from '@tanstack/hotkeys';
import {
  type UseHotkeyDefinition,
  type UseHotkeySequenceDefinition,
  useHotkeySequences,
  useHotkeys,
} from '@tanstack/react-hotkeys';
import { useRouterState } from '@tanstack/react-router';
import { useCallback, useMemo, useRef } from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import {
  getBindingAction,
  type KeyActionContext,
} from '../utils/keybindingActions';
import { hotkeyOptions, isEscape } from '../utils/keybindingActivation';
import {
  getEffectiveBindings,
  isActive,
  isSequenceBinding,
  routeBaseFromPath,
} from '../utils/keybindings';
import { SEQUENCE_TIMEOUT_MS } from '../utils/keymaps';
import { useStore } from './useStore';

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
 * `Escape` and `global.cancel` stay active while typing (`ignoreInputs: false`)
 * so they can exit search and comment inputs; other bindings ignore form fields.
 */
export function useAppHotkeys(): void {
  const overlays = useOverlays();
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
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
            // make this flag always true. Control+G does not trigger Radix's
            // dismissal handling, so global.cancel keeps the default behavior.
            if (isEscape(binding) && event.defaultPrevented) return;
            getBindingAction(binding.actionId)?.(makeCtx());
          },
          options: {
            enabled: isActive(binding, { routeBase, activeOverlay }),
            ...hotkeyOptions(binding),
          },
        })),
    [bindings, routeBase, activeOverlay, makeCtx],
  );

  const sequenceDefs = useMemo<UseHotkeySequenceDefinition[]>(
    () =>
      bindings.filter(isSequenceBinding).map((binding) => ({
        sequence: [...binding.keys] as HotkeySequence,
        callback: () => getBindingAction(binding.actionId)?.(makeCtx()),
        options: {
          enabled: isActive(binding, { routeBase, activeOverlay }),
          ignoreInputs: true,
        },
      })),
    [bindings, routeBase, activeOverlay, makeCtx],
  );

  useHotkeys(hotkeyDefs, { conflictBehavior: 'allow' });
  useHotkeySequences(sequenceDefs, {
    conflictBehavior: 'allow',
    timeout: SEQUENCE_TIMEOUT_MS[style],
  });
}
