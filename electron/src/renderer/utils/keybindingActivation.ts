import {
  type HotkeyOptions,
  matchesKeyboardEvent,
  type SequenceRegistrationView,
} from '@tanstack/hotkeys';
import type { OverlayKind } from '../overlays/OverlayContext';
import type { FirstRunScreen } from './authScreenState';
import type { KeyBinding, RouteBase } from './keybindings';
import { SANDBOX_ACTION_IDS, type SandboxPane } from './onboardingSandbox';

/** Onboarding actions that only make sense while the sandbox step is showing. */
const SANDBOX_STEP_ACTION_IDS: ReadonlySet<string> = new Set([
  'onboarding.nextStyle',
  'onboarding.prevStyle',
]);

const CONTINUE_ACTION_ID = 'onboarding.continue';
const NATIVELY_ACTIVATED_SELECTOR =
  'button, summary, [role="button"], input[type="button"], input[type="submit"], input[type="reset"]';

/** Native activation must win over route-level Enter/Space bindings. */
export function yieldsToFocusedControl(
  binding: Pick<KeyBinding, 'keys'>,
  target: EventTarget | null,
): boolean {
  if (typeof Element === 'undefined' || !(target instanceof Element))
    return false;
  if (binding.keys === 'Tab' || binding.keys === 'Shift+Tab')
    return (
      target.closest(`${NATIVELY_ACTIVATED_SELECTOR}, a[href], [tabindex]`) !==
      null
    );
  if (binding.keys !== 'Enter' && binding.keys !== 'Space') return false;
  const selector =
    binding.keys === 'Enter'
      ? `${NATIVELY_ACTIVATED_SELECTOR}, a[href]`
      : NATIVELY_ACTIVATED_SELECTOR;
  return target.closest(selector) !== null;
}

/** Style cycling is local to the picker, never a global Tab focus trap. */
export function isStyleCycle(binding: Pick<KeyBinding, 'actionId'>): boolean {
  return SANDBOX_STEP_ACTION_IDS.has(binding.actionId);
}

export function isStylePickerTarget(target: EventTarget | null): boolean {
  return (
    typeof Element !== 'undefined' &&
    target instanceof Element &&
    target.closest('[data-navigation-style-picker]') !== null
  );
}

/** Only literal Escape participates in Radix's dismissal handling. */
export function isEscape(binding: Pick<KeyBinding, 'keys'>): boolean {
  return binding.keys === 'Escape';
}

/** Escape, native activation and picker traversal manage defaults at dispatch. */
export function hotkeyOptions(
  binding: Pick<KeyBinding, 'keys' | 'actionId'>,
  activeOverlay: OverlayKind | null = null,
): Pick<HotkeyOptions, 'ignoreInputs' | 'preventDefault' | 'requireReset'> {
  return {
    // C-g can exit overlay inputs, but must not bypass composer cleanup.
    ignoreInputs: !(
      isEscape(binding) ||
      (binding.actionId === 'global.cancel' && activeOverlay !== null)
    ),
    ...(binding.actionId === 'detail.approve' ? { requireReset: true } : {}),
    ...(isEscape(binding) ||
    isStyleCycle(binding) ||
    binding.keys === 'Enter' ||
    binding.keys === 'Space' ||
    binding.keys === 'Tab' ||
    binding.keys === 'Shift+Tab'
      ? { preventDefault: false }
      : {}),
  };
}

/** First-run screens on which each sign-in / engine action is meaningful. */
const FIRST_RUN_ACTION_STATES: Readonly<
  Record<string, readonly FirstRunScreen[]>
> = {
  'login.provider.github': ['idle', 'failed', 'interrupted', 'timedOut'],
  'login.reopen': ['waiting'],
  'login.copyLink': ['waiting'],
  'login.cancel': ['waiting', 'timedOut'],
  'login.retry': ['timedOut'],
  'connecting.restart': ['crashed'],
  'connecting.openLogs': ['slow', 'crashed'],
  'connecting.report': ['crashed'],
};

function isFirstRunAction(actionId: string): boolean {
  return actionId.startsWith('login.') || actionId.startsWith('connecting.');
}

/**
 * True when the event advances an enabled sequence's actual manager progress.
 * TanStack runs single hotkeys and sequences as separate
 * listeners, so without this check vim's `g 1` also fires `1` (jump to PR)
 * and emacs `C-c C-v` also fires `C-v` (page down).
 */
export function continuesSequence(
  event: KeyboardEvent,
  options: {
    readonly sequences: readonly SequenceRegistrationView[];
    readonly timeoutMs: number;
    readonly now: number;
  },
): boolean {
  if (event.repeat || event.isComposing) return false;
  return options.sequences.some((registration) => {
    const next = registration.sequence[registration.matchedStepCount];
    return (
      registration.options.enabled === true &&
      registration.matchedStepCount > 0 &&
      next !== undefined &&
      options.now - registration.partialMatchLastKeyTime <= options.timeoutMs &&
      matchesKeyboardEvent(event, next)
    );
  });
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
    screen,
    isSpawned,
    isConnected,
  }: {
    routeBase: RouteBase | null;
    activeOverlay: OverlayKind | null;
    sandboxPane: SandboxPane | null;
    screen?: FirstRunScreen;
    isSpawned?: boolean;
    isConnected?: boolean;
  },
): boolean {
  const real = { routeBase, activeOverlay };
  if (isFirstRunAction(binding.actionId)) {
    if (!isActive(binding, real)) return false;
    const allowed = FIRST_RUN_ACTION_STATES[binding.actionId];
    if (
      screen === undefined ||
      allowed === undefined ||
      !allowed.includes(screen)
    )
      return false;
    if (binding.actionId === 'connecting.restart' && isSpawned !== true)
      return false;
    const needsEngine =
      binding.actionId.startsWith('login.provider.') ||
      binding.actionId === 'login.retry';
    return !needsEngine || isConnected === true;
  }
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
