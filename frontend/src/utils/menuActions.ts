import type { OverlayKind } from '../overlays/OverlayContext';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import { isActive } from './keybindingActivation';
import { ACTION_CATALOG, type RouteBase } from './keybindings';

/** Exactly the ids the native menu dispatches (services.MenuActionIDs). */
export const MENU_ACTION_IDS = [
  'global.settings',
  'global.inbox',
  'global.search',
  'global.command',
  'global.help',
] as const;

interface MenuActivation {
  readonly routeBase: RouteBase | null;
  readonly activeOverlay: OverlayKind | null;
}

/** Same gate as keys and the palette, so an enabled menu item always acts. */
export function enabledMenuActions(context: MenuActivation): string[] {
  return MENU_ACTION_IDS.filter((actionId) => {
    const definition = ACTION_CATALOG.find(
      (entry) => entry.actionId === actionId,
    );
    return definition !== undefined && isActive(definition, context);
  });
}

/** Re-checks activation at click time; a sign-out race must not run the action. */
export function runMenuAction(
  actionId: string,
  context: KeyActionContext,
): boolean {
  if (!enabledMenuActions(context).includes(actionId)) return false;
  const action = getBindingAction(actionId);
  if (action === undefined) return false;
  action(context);
  return true;
}
