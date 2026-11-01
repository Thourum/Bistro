import {
  type HotkeyOptions,
  matchesKeyboardEvent,
  normalizeHotkey,
  type SequenceRegistrationView,
} from '@tanstack/hotkeys';
import type { OverlayKind } from '../overlays/OverlayContext';
import type { UIState } from '../store/ui';
import type { FirstRunScreen } from './authScreenState';
import type { KeyBinding, RouteBase } from './keybindings';
import { SANDBOX_ACTION_IDS, type SandboxPane } from './onboardingSandbox';

const SANDBOX_STEP_ACTION_IDS: ReadonlySet<string> = new Set([
  'onboarding.nextStyle',
  'onboarding.prevStyle',
]);
const SYNC_ACTION_IDS: ReadonlySet<string> = new Set([
  'onboarding.repoDown',
  'onboarding.repoUp',
  'onboarding.repoToggle',
  'onboarding.repoAll',
  'onboarding.repoNone',
  'onboarding.repoFilter',
  'onboarding.chooseFolder',
  'onboarding.stopScan',
]);
const BACK_ACTION_IDS: ReadonlySet<string> = new Set([
  'global.escape',
  'global.cancel',
]);
const CONTINUE_ACTION_ID = 'onboarding.continue';
const NATIVELY_ACTIVATED_SELECTOR =
  'button, summary, [role="button"], input[type="button"], input[type="submit"], input[type="reset"]';
const ONBOARDING_REPO_FILTER_SELECTOR = '[data-onboarding-repo-filter]';
const SANDBOX_COMPOSER_SELECTOR = '[data-onboarding-sandbox-composer]';

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

/** Step-1 Escape blurs the filter before stopping a scan or doing anything else. */
export function blurOnboardingRepoFilter(target: EventTarget | null): boolean {
  if (typeof HTMLElement === 'undefined' || !(target instanceof HTMLElement))
    return false;
  const filter = target.closest(ONBOARDING_REPO_FILTER_SELECTOR);
  if (!(filter instanceof HTMLElement)) return false;
  filter.blur();
  return true;
}

/** Whether a key event came from a fake practice composer's textarea. */
export function isSandboxComposerTarget(target: EventTarget | null): boolean {
  return (
    typeof Element !== 'undefined' &&
    target instanceof Element &&
    target.closest(SANDBOX_COMPOSER_SELECTOR) !== null
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
    binding.actionId === CONTINUE_ACTION_ID ||
    binding.keys === 'Enter' ||
    binding.keys === 'Space' ||
    binding.keys === 'Tab' ||
    binding.keys === 'Shift+Tab'
      ? { preventDefault: false }
      : {}),
    // A held key auto-repeats; without a reset it would chain Continue into Finish.
    ...(binding.actionId === CONTINUE_ACTION_ID ? { requireReset: true } : {}),
  };
}

/** Cleanup owns list Escape before the first-run rail can be hidden. */
export function hasListEscapeWork(
  state: Pick<
    UIState,
    'confirmingAction' | 'searchQuery' | 'repoFilter' | 'filterState'
  >,
): boolean {
  return (
    state.confirmingAction !== null ||
    state.searchQuery !== '' ||
    state.repoFilter !== null ||
    state.filterState !== 'open'
  );
}

/** Compare normalized hotkey steps, including user-overridden sequences. */
export function sameBindingKeys(
  left: KeyBinding['keys'],
  right: KeyBinding['keys'],
): boolean {
  const leftSteps = typeof left === 'string' ? [left] : left;
  const rightSteps = typeof right === 'string' ? [right] : right;
  return (
    leftSteps.length === rightSteps.length &&
    leftSteps.every((step, index) => {
      const rightStep = rightSteps[index];
      return (
        rightStep !== undefined &&
        normalizeHotkey(step) === normalizeHotkey(rightStep)
      );
    })
  );
}

const FIRST_RUN_ACTION_STATES: Readonly<
  Record<string, readonly FirstRunScreen[]>
> = {
  'login.provider.github': ['idle', 'failed', 'interrupted', 'timedOut'],
  'login.reopen': ['waiting'],
  'login.copyLink': ['waiting'],
  'login.cancel': ['waiting', 'timedOut'],
  'login.retry': ['timedOut'],
  'connecting.openLogs': ['connecting'],
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
  binding: Pick<KeyBinding, 'activation'>,
  {
    routeBase,
    activeOverlay,
  }: { routeBase: RouteBase | null; activeOverlay: OverlayKind | null },
): boolean {
  if (activeOverlay !== null)
    return binding.activation.overlays?.includes(activeOverlay) === true;
  if (
    binding.activation.overlays !== undefined &&
    binding.activation.routes === undefined
  )
    return false;
  const routes = binding.activation.routes;
  if (routes !== undefined && routes.length === 0) return false;
  return (
    routeBase !== null && (routes === undefined || routes.includes(routeBase))
  );
}

export interface BindingActivationContext {
  routeBase: RouteBase | null;
  activeOverlay: OverlayKind | null;
  sandboxPane: SandboxPane | null;
  screen?: FirstRunScreen;
  isConnected?: boolean;
  onboardingStep?: 'sync' | 'practice' | null;
  isScanning?: boolean;
  isChoosingFolder?: boolean;
  isPracticeComplete?: boolean;
  continueKeys?: KeyBinding['keys'] | undefined;
  isFirstRunRailVisible?: boolean;
  hasListEscapeWork?: boolean;
}

function isAuthBindingEnabled(
  binding: KeyBinding,
  context: BindingActivationContext,
): boolean {
  if (!isActive(binding, context)) return false;
  const allowed = FIRST_RUN_ACTION_STATES[binding.actionId];
  if (
    context.screen === undefined ||
    allowed === undefined ||
    !allowed.includes(context.screen)
  )
    return false;
  const needsEngine =
    binding.actionId.startsWith('login.provider.') ||
    binding.actionId === 'login.retry';
  return !needsEngine || context.isConnected === true;
}

/** Runtime ownership on top of route activation: overlays, sync, practice and rail. */
export function isBindingEnabled(
  binding: KeyBinding,
  context: BindingActivationContext,
): boolean {
  const { routeBase, activeOverlay, onboardingStep } = context;
  if (isFirstRunAction(binding.actionId))
    return isAuthBindingEnabled(binding, context);
  if (activeOverlay !== null) return isActive(binding, context);
  if (binding.actionId === 'firstRun.hide') {
    return (
      routeBase === 'list' &&
      context.isFirstRunRailVisible === true &&
      !context.hasListEscapeWork
    );
  }
  if (
    routeBase === 'list' &&
    BACK_ACTION_IDS.has(binding.actionId) &&
    context.isFirstRunRailVisible &&
    !context.hasListEscapeWork
  )
    return false;
  if (SYNC_ACTION_IDS.has(binding.actionId)) {
    if (routeBase !== 'onboarding' || onboardingStep !== 'sync') return false;
    if (binding.actionId === 'onboarding.chooseFolder')
      return !context.isChoosingFolder;
    if (binding.actionId === 'onboarding.stopScan')
      return context.isScanning === true;
    return true;
  }
  if (routeBase !== 'onboarding') return isActive(binding, context);
  return isOnboardingBindingEnabled(binding, context);
}

function isOnboardingBindingEnabled(
  binding: KeyBinding,
  context: BindingActivationContext,
): boolean {
  const { activeOverlay, sandboxPane, onboardingStep } = context;
  if (
    BACK_ACTION_IDS.has(binding.actionId) &&
    (onboardingStep === 'sync' || sandboxPane === null)
  ) {
    return onboardingStep === 'sync' && !context.isScanning;
  }
  if (SANDBOX_STEP_ACTION_IDS.has(binding.actionId)) {
    return onboardingStep === 'practice' && sandboxPane !== null;
  }
  if (binding.actionId === CONTINUE_ACTION_ID) {
    return (
      (onboardingStep === 'sync' ||
        (onboardingStep === 'practice' &&
          sandboxPane !== null &&
          (sandboxPane === 'list' || context.isPracticeComplete === true))) &&
      isActive(binding, context)
    );
  }
  if (SANDBOX_ACTION_IDS.has(binding.actionId)) {
    if (onboardingStep !== 'practice' || sandboxPane === null) return false;
    if (
      context.isPracticeComplete &&
      context.continueKeys !== undefined &&
      sameBindingKeys(binding.keys, context.continueKeys)
    )
      return false;
    return isActive(binding, { routeBase: sandboxPane, activeOverlay });
  }
  return isActive(binding, context);
}
