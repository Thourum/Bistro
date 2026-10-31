import { useEffect, useMemo } from 'react';
import { createMenuActionReporter, reportMenuActions } from '../services/menu';
import { appStore } from '../store';
import type { KeyActionContext } from '../utils/keybindingActions';
import type { BindingActivationContext } from '../utils/keybindingActivation';
import { createLogger } from '../utils/logger';
import {
  enabledMenuActions,
  MENU_ACTION_IDS,
  runMenuAction,
} from '../utils/menuActions';
import { useStore } from './useStore';

const log = createLogger('MenuActions');

/** Native menu requests share the hotkeys' ref-backed action context. */
export function useMenuActions(
  activation: BindingActivationContext,
  makeCtx: () => KeyActionContext,
): void {
  const { routeBase, activeOverlay } = activation;
  const enabledKey = useMemo(
    () => enabledMenuActions({ routeBase, activeOverlay }).join(','),
    [routeBase, activeOverlay],
  );
  const menuActionRequests = useStore(appStore, (s) => s.menuActionRequests);
  const reportLatest = useMemo(
    () =>
      createMenuActionReporter({
        send: reportMenuActions,
        onError: (fault) =>
          log.warn({ err: fault.message }, 'menu availability report failed'),
      }),
    [],
  );

  useEffect(() => {
    const enabled = enabledKey.split(',');
    const ids = MENU_ACTION_IDS.filter((actionId) =>
      enabled.includes(actionId),
    );
    reportLatest(ids);
  }, [enabledKey, reportLatest]);

  useEffect(() => {
    if (menuActionRequests.length === 0) return;
    // Read the live queue: StrictMode can replay an effect with a stale snapshot.
    const pendingRequests = appStore.state.menuActionRequests;
    if (pendingRequests.length === 0) return;
    // Clear before dispatch so renders caused by handlers cannot replay requests.
    appStore.setState((state) => ({ ...state, menuActionRequests: [] }));
    for (const request of pendingRequests) {
      if (!runMenuAction(request.actionId, makeCtx()) && import.meta.env.DEV)
        log.warn(
          { actionId: request.actionId },
          'menu action is no longer active',
        );
    }
  }, [menuActionRequests, makeCtx]);
}
