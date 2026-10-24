import {
  type HotkeyOptions,
  type HotkeySequence,
  matchesKeyboardEvent,
  normalizeHotkey,
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
const NATIVELY_ACTIVATED_SELECTOR = 'button, summary, [role="button"]';
const ONBOARDING_REPO_FILTER_SELECTOR = '[data-onboarding-repo-filter]';
const SANDBOX_COMPOSER_SELECTOR = '[data-onboarding-sandbox-composer]';

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

/** Input handling for single hotkeys; Escape and continue manage defaults themselves. */
export function hotkeyOptions(
  binding: Pick<KeyBinding, 'keys' | 'actionId'>,
): Pick<HotkeyOptions, 'ignoreInputs' | 'preventDefault' | 'requireReset'> {
  return {
    // Composers cancel locally on Escape only; C-g must not bypass that cleanup.
    ignoreInputs: !isEscape(binding),
    // Keep Radix's defaultPrevented meaningful and focused buttons' native Space click.
    ...(isEscape(binding) || binding.actionId === CONTINUE_ACTION_ID
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
  'connecting.restart': ['crashed'],
  'connecting.openLogs': ['slow', 'crashed'],
  'connecting.report': ['crashed'],
};

function isFirstRunAction(actionId: string): boolean {
  return actionId.startsWith('login.') || actionId.startsWith('connecting.');
}

/** A keydown seen by the dispatcher, kept to spot a sequence in progress. */
export interface KeyPress {
  readonly event: KeyboardEvent;
  readonly time: number;
}

const MODIFIER_KEYS: ReadonlySet<string> = new Set([
  'Control',
  'Shift',
  'Alt',
  'Meta',
]);

/** Holding a modifier for a chord is not a sequence step of its own. */
export function isModifierPress(event: Pick<KeyboardEvent, 'key'>): boolean {
  return MODIFIER_KEYS.has(event.key);
}

/**
 * True when `current` is a later step of an enabled sequence that the recent
 * presses started. TanStack runs single hotkeys and sequences as separate
 * listeners, so without this check vim's `g 1` also fires `1` (jump to PR)
 * and emacs `C-c C-v` also fires `C-v` (page down).
 */
export function continuesSequence(
  recent: readonly KeyPress[],
  current: KeyPress,
  options: {
    readonly sequences: readonly HotkeySequence[];
    readonly timeoutMs: number;
  },
): boolean {
  return options.sequences.some((sequence) =>
    sequence.some((step, stepIndex) => {
      if (stepIndex === 0 || !matchesKeyboardEvent(current.event, step))
        return false;
      const started = recent.slice(-stepIndex);
      if (started.length !== stepIndex) return false;
      return started.every((press, offset) => {
        const expected = sequence[offset];
        const next = started[offset + 1] ?? current;
        return (
          expected !== undefined &&
          matchesKeyboardEvent(press.event, expected) &&
          next.time - press.time <= options.timeoutMs
        );
      });
    }),
  );
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
  isSpawned?: boolean;
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
  if (binding.actionId === 'connecting.restart' && context.isSpawned !== true)
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
