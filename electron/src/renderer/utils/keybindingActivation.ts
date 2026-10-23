import type { HotkeyOptions } from '@tanstack/hotkeys';
import type { OverlayKind } from '../overlays/OverlayContext';
import type { KeyBinding, RouteBase } from './keybindings';

/** Only literal Escape participates in Radix's dismissal handling. */
export function isEscape(binding: Pick<KeyBinding, 'keys'>): boolean {
  return binding.keys === 'Escape';
}

/** Input handling for single hotkeys; Escape preserves prior dismissal state. */
export function hotkeyOptions(
  binding: Pick<KeyBinding, 'actionId' | 'keys'>,
): Pick<HotkeyOptions, 'ignoreInputs' | 'preventDefault'> {
  return {
    ignoreInputs: !isEscape(binding) && binding.actionId !== 'global.cancel',
    // Keep `defaultPrevented` meaningful as "already handled" for Escape only.
    ...(isEscape(binding) ? { preventDefault: false } : {}),
  };
}

/** Derive the route used for binding activation from the current pathname. */
export function routeBaseFromPath(pathname: string): RouteBase | null {
  if (/^\/prs\/[^/]+\/diff$/.test(pathname)) return 'diff';
  if (/^\/prs\/[^/]+$/.test(pathname)) return 'detail';
  if (pathname === '/prs' || pathname === '/prs/') return 'list';
  if (pathname === '/settings') return 'settings';
  if (pathname === '/login') return 'login';
  if (pathname === '/onboarding') return 'onboarding';
  return null;
}

/** An open overlay takes precedence over every route-level binding. */
export function isActive(
  binding: KeyBinding,
  {
    routeBase,
    activeOverlay,
  }: { routeBase: RouteBase | null; activeOverlay: OverlayKind | null },
): boolean {
  if (activeOverlay !== null) {
    return binding.activation.overlays?.includes(activeOverlay) === true;
  }
  if (
    binding.activation.overlays !== undefined &&
    binding.activation.routes === undefined
  ) {
    return false;
  }
  const routes = binding.activation.routes;
  if (routes !== undefined && routes.length === 0) return false;
  return (
    routeBase !== null && (routes === undefined || routes.includes(routeBase))
  );
}
