import type {
  KeybindingOverrides,
  NavigationStyle,
} from '@renderer/types/settings';
import {
  type Hotkey,
  matchesKeyboardEvent,
  normalizeHotkey,
} from '@tanstack/hotkeys';
import { err, Fault, ok, type Result } from '@thourum/fault';
import type { FirstRunScreen } from './authScreenState';
import {
  type BindingActivationContext,
  isBindingEnabled,
} from './keybindingActivation';
import {
  ALL_ROUTES,
  getEffectiveBindings,
  type KeyBinding,
} from './keybindings';
import { formatBindingKeys } from './legendPresets';
import type { SandboxPane } from './onboardingSandbox';

const AUTH_SCREENS: readonly FirstRunScreen[] = [
  'idle',
  'waiting',
  'timedOut',
  'failed',
  'interrupted',
  'engineDown',
  'connecting',
];
const SANDBOX_PANES: readonly (SandboxPane | null)[] = [
  null,
  'list',
  'detail',
  'diff',
];
const BOOLEAN_STATES = [false, true] as const;

function activationContexts(
  bindings: readonly KeyBinding[],
): BindingActivationContext[] {
  const continueKeys = bindings.find(
    (binding) => binding.actionId === 'onboarding.continue',
  )?.keys;
  const overlays = new Set(
    bindings.flatMap((binding) => binding.activation.overlays ?? []),
  );
  return [...ALL_ROUTES, null].flatMap(
    (routeBase): BindingActivationContext[] => {
      const base: BindingActivationContext = {
        routeBase,
        activeOverlay: null,
        sandboxPane: null,
        continueKeys,
      };
      const overlayContexts = [...overlays].map((activeOverlay) => ({
        ...base,
        activeOverlay,
      }));
      return [base, ...overlayContexts, ...routeContexts(base)];
    },
  );
}

function routeContexts(
  base: BindingActivationContext,
): BindingActivationContext[] {
  if (base.routeBase === 'login' || base.routeBase === 'connecting')
    return AUTH_SCREENS.flatMap((screen) =>
      BOOLEAN_STATES.map((isConnected) => ({ ...base, screen, isConnected })),
    );
  if (base.routeBase === 'list')
    return BOOLEAN_STATES.flatMap((isFirstRunRailVisible) =>
      BOOLEAN_STATES.map((hasListEscapeWork) => ({
        ...base,
        isFirstRunRailVisible,
        hasListEscapeWork,
      })),
    );
  if (base.routeBase !== 'onboarding') return [base];
  return [
    ...BOOLEAN_STATES.flatMap((isScanning) =>
      BOOLEAN_STATES.map((isChoosingFolder) => ({
        ...base,
        onboardingStep: 'sync' as const,
        isScanning,
        isChoosingFolder,
      })),
    ),
    ...SANDBOX_PANES.flatMap((sandboxPane) =>
      BOOLEAN_STATES.map((isPracticeComplete) => ({
        ...base,
        onboardingStep: 'practice' as const,
        sandboxPane,
        isPracticeComplete,
      })),
    ),
  ];
}

function hasSharedScope(
  candidate: KeyBinding,
  other: KeyBinding,
  contexts: readonly BindingActivationContext[],
): boolean {
  // The filter's local Escape and global Back both only close the same overlay.
  // Keep this intentional alias, not a blanket exemption for Escape remaps.
  if (
    candidate.keys === 'Escape' &&
    other.keys === 'Escape' &&
    [candidate.actionId, other.actionId].includes('filter.close') &&
    [candidate.actionId, other.actionId].includes('global.escape')
  )
    return false;
  return contexts.some(
    (context) =>
      isBindingEnabled(candidate, context) && isBindingEnabled(other, context),
  );
}

function keySteps(binding: KeyBinding): readonly Hotkey[] {
  const steps =
    typeof binding.keys === 'string' ? [binding.keys] : binding.keys;
  return steps.map((step) => normalizeHotkey(step));
}

/**
 * Whether the shorter of the two step lists starts the longer one. Candidate
 * step i also collides when its recorded keydown would fire the other step —
 * `Shift+?` and `Shift+/` are different strings for the same key.
 */
function stepsCollide(
  candidateSteps: readonly Hotkey[],
  otherSteps: readonly Hotkey[],
  events: readonly KeyboardEvent[],
): boolean {
  return candidateSteps.slice(0, otherSteps.length).every((step, index) => {
    const otherStep = otherSteps[index];
    if (otherStep === undefined) return false;
    if (step === otherStep) return true;
    const event = events[index];
    return event !== undefined && matchesKeyboardEvent(event, otherStep);
  });
}

/**
 * Bindings that would fire alongside `candidate`: the same keys, or one
 * sequence prefixing the other, in a scope where both can be enabled. Pass the
 * recorder's keydown `events` (one per step) to also catch keys that are
 * spelled differently but fire on the same keypress.
 */
export function findKeybindingConflicts(
  bindings: readonly KeyBinding[],
  candidate: KeyBinding,
  events: readonly KeyboardEvent[] = [],
): KeyBinding[] {
  const steps = keySteps(candidate);
  const prospective = [
    ...bindings.filter((binding) => binding.actionId !== candidate.actionId),
    candidate,
  ];
  const contexts = activationContexts(prospective);
  return bindings.filter(
    (other) =>
      other.actionId !== candidate.actionId &&
      stepsCollide(steps, keySteps(other), events) &&
      hasSharedScope(candidate, other, contexts),
  );
}

/** Shared rejection text for recording and restoring a default. */
export function keybindingConflictMessage(
  bindings: readonly KeyBinding[],
  candidate: KeyBinding,
  events: readonly KeyboardEvent[] = [],
): string | undefined {
  const [conflict] = findKeybindingConflicts(bindings, candidate, events);
  return conflict === undefined
    ? undefined
    : `${formatBindingKeys(candidate)} already runs "${conflict.description}"`;
}

/** Restore a style default only if its freed keys have not been reused. */
export function resetKeybinding({
  style,
  overrides,
  actionId,
}: {
  style: NavigationStyle;
  overrides: KeybindingOverrides;
  actionId: string;
}): Result<KeybindingOverrides, Fault> {
  const { [actionId]: _removed, ...rest } = overrides;
  const bindings = getEffectiveBindings(style, rest);
  const restored = bindings.find((binding) => binding.actionId === actionId);
  const message =
    restored === undefined
      ? undefined
      : keybindingConflictMessage(bindings, restored);
  return message === undefined
    ? ok(rest)
    : err(new Fault(message).withTag('KEYBINDING_CONFLICT'));
}
