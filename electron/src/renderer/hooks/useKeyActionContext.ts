import { useRouterState } from '@tanstack/react-router';
import { useCallback, useRef } from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { KeyActionContext } from '../utils/keybindingActions';
import { routeBaseFromPath } from '../utils/keybindings';

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
 * Stable factory for the context keybinding actions receive. Reads the latest
 * overlay and router state through a ref, so callers never hold stale closures.
 * Shared by useAppHotkeys and the command palette (same handler, same context).
 */
export function useKeyActionContext(): () => KeyActionContext {
  const overlays = useOverlays();
  const routerState = useRouterState();
  const latestRef = useRef({ overlays, routerState });
  latestRef.current = { overlays, routerState };

  return useCallback((): KeyActionContext => {
    const { overlays: current, routerState: state } = latestRef.current;
    const context: KeyActionContext = {
      store: appStore,
      routeBase: routeBaseFromPath(state.location.pathname),
      activeOverlay: current.activeOverlay,
      overlays: current,
      location: state.location,
    };
    const prId = routePrId(state.matches);
    return prId === undefined ? context : { ...context, prId };
  }, []);
}
