import {
  type Hotkey,
  matchesKeyboardEvent,
  normalizeHotkey,
} from '@tanstack/hotkeys';
import { isActive } from './keybindingActivation';
import { ALL_ROUTES, type KeyBinding } from './keybindings';

function hasSharedScope(candidate: KeyBinding, other: KeyBinding): boolean {
  return (
    ALL_ROUTES.some((routeBase) => {
      const context = { routeBase, activeOverlay: null };
      return isActive(candidate, context) && isActive(other, context);
    }) ||
    (candidate.activation.overlays ?? []).some((activeOverlay) => {
      const context = { routeBase: null, activeOverlay };
      return isActive(candidate, context) && isActive(other, context);
    })
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
 * sequence prefixing the other, in a scope where both can be active. Pass the
 * recorder's keydown `events` (one per step) to also catch keys that are
 * spelled differently but fire on the same keypress.
 */
export function findKeybindingConflicts(
  bindings: readonly KeyBinding[],
  candidate: KeyBinding,
  events: readonly KeyboardEvent[] = [],
): KeyBinding[] {
  const steps = keySteps(candidate);
  return bindings.filter(
    (other) =>
      other.actionId !== candidate.actionId &&
      stepsCollide(steps, keySteps(other), events) &&
      hasSharedScope(candidate, other),
  );
}
