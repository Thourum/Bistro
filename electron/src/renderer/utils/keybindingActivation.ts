import type { HotkeyOptions } from '@tanstack/hotkeys';
import type { OverlayKind } from '../overlays/OverlayContext';
import type { KeyBinding, RouteBase } from './keybindings';
import { SANDBOX_ACTION_IDS, type SandboxPane } from './onboardingSandbox';

/** Onboarding actions that only make sense while the sandbox step is showing. */
const SANDBOX_STEP_ACTION_IDS: ReadonlySet<string> = new Set([
  'onboarding.nextStyle',
  'onboarding.prevStyle',
]);

const CONTINUE_ACTION_ID = 'onboarding.continue';
// Controls whose native Space activation must win over `onboarding.continue`.
const NATIVELY_ACTIVATED_SELECTOR = 'button, summary, [role="button"]';

/** `onboarding.continue` steps aside while a button-like control has focus. */
export function yieldsToFocusedControl(
  binding: Pick<KeyBinding, 'actionId'>,
  target: EventTarget | null,
): boolean {
  return (
    binding.actionId === CONTINUE_ACTION_ID &&
    typeof Element !== 'undefined' &&
    target instanceof Element &&
    target.closest(NATIVELY_ACTIVATED_SELECTOR) !== null
  );
}

/** Only literal Escape participates in Radix's dismissal handling. */
export function isEscape(binding: Pick<KeyBinding, 'keys'>): boolean {
  return binding.keys === 'Escape';
}

/** Input handling for single hotkeys; Escape and continue manage defaults themselves. */
export function hotkeyOptions(
  binding: Pick<KeyBinding, 'keys' | 'actionId'>,
): Pick<HotkeyOptions, 'ignoreInputs' | 'preventDefault'> {
  return {
    // Composers cancel locally on Escape only; C-g must not bypass that cleanup.
    ignoreInputs: !isEscape(binding),
    // Escape keeps `defaultPrevented` meaningful as "already handled";
    // continue must not cancel a focused button's native Space click.
    ...(isEscape(binding) || binding.actionId === CONTINUE_ACTION_ID
      ? { preventDefault: false }
      : {}),
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

/**
 * `isActive`, plus the onboarding sandbox: while it is mounted, sandbox
 * actions activate as if the app were on the sandbox's pane.
 */
export function isBindingEnabled(
  binding: KeyBinding,
  {
    routeBase,
    activeOverlay,
    sandboxPane,
  }: {
    routeBase: RouteBase | null;
    activeOverlay: OverlayKind | null;
    sandboxPane: SandboxPane | null;
  },
): boolean {
  const real = { routeBase, activeOverlay };
  if (routeBase !== 'onboarding') return isActive(binding, real);
  if (sandboxPane === null) {
    return (
      !SANDBOX_STEP_ACTION_IDS.has(binding.actionId) && isActive(binding, real)
    );
  }
  // In an opened fake PR, Space belongs to the PR (vim detail.viewDiff).
  if (binding.actionId === CONTINUE_ACTION_ID) {
    return sandboxPane === 'list' && isActive(binding, real);
  }
  if (SANDBOX_ACTION_IDS.has(binding.actionId)) {
    return isActive(binding, { routeBase: sandboxPane, activeOverlay });
  }
  return isActive(binding, real);
}
