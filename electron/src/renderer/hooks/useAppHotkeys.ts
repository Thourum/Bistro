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
  getEffectiveBindings,
  isActive,
  isSequenceBinding,
  type KeyActionContext,
  routeBaseFromPath,
} from '../utils/keybindings';
import { useStore } from './useStore';

/** Sequence steps must follow each other within this window (was 500ms before the migration). */
const SEQUENCE_TIMEOUT_MS = 500;

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
 * `Escape` stays active while typing (`ignoreInputs: false`) so it can exit
 * search and comment inputs; everything else is suppressed in form fields.
 */
export function useAppHotkeys(): void {
  const overlays = useOverlays();
  const overrides = useStore(settingsStore, (s) => s.keybindings);
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

  const bindings = useMemo(() => getEffectiveBindings(overrides), [overrides]);

  const hotkeyDefs = useMemo<UseHotkeyDefinition[]>(
    () =>
      bindings
        .filter((b) => !isSequenceBinding(b))
        .map((binding) => ({
          hotkey: binding.keys as RegisterableHotkey,
          // `enabled` is evaluated from the render that registered this
          // binding, so it reflects the overlay state at keypress time. The
          // action, however, reads live state via refs — and a Radix dialog
          // dismisses Escape from a document CAPTURE listener, which runs
          // BEFORE this bubble-phase callback. Passing the registration-time
          // overlay keeps the two in agreement; otherwise `global.escape`
          // observed `activeOverlay: null` for a keypress that had just closed
          // an overlay and fell through to its route branch, so dismissing the
          // help overlay on /diff also navigated out of the diff.
          callback: () => binding.action({ ...makeCtx(), activeOverlay }),
          options: {
            enabled: isActive(binding, { routeBase, activeOverlay }),
            ignoreInputs: binding.keys !== 'Escape',
          },
        })),
    [bindings, routeBase, activeOverlay, makeCtx],
  );

  const sequenceDefs = useMemo<UseHotkeySequenceDefinition[]>(
    () =>
      bindings.filter(isSequenceBinding).map((binding) => ({
        sequence: [...binding.keys] as HotkeySequence,
        callback: () => binding.action(makeCtx()),
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
    timeout: SEQUENCE_TIMEOUT_MS,
  });
}
