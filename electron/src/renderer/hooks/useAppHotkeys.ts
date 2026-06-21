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

  // Stable makeCtx that always reads current values from refs.
  const makeCtx = useCallback(
    (): KeyActionContext => ({
      store: appStore,
      routeBase: routeBaseRef.current,
      activeOverlay: activeOverlayRef.current,
      overlays: overlaysRef.current,
      location: locationRef.current,
    }),
    [],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  const bindings = useMemo(() => getEffectiveBindings(overrides), [overrides]);

  const hotkeyDefs = useMemo<UseHotkeyDefinition[]>(
    () =>
      bindings
        .filter((b) => !isSequenceBinding(b))
        .map((binding) => ({
          hotkey: binding.keys as unknown as Parameters<
            typeof useHotkeys
          >[0][0]['hotkey'],
          callback: () => binding.action(makeCtx()),
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
        sequence: [...binding.keys] as unknown as Parameters<
          typeof useHotkeySequences
        >[0][0]['sequence'],
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
