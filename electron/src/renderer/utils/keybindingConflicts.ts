import { normalizeHotkey } from '@tanstack/hotkeys';
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

function keySteps(binding: KeyBinding): readonly string[] {
  const steps =
    typeof binding.keys === 'string' ? [binding.keys] : binding.keys;
  return steps.map((step) => normalizeHotkey(step));
}

function isPrefixOrEqual(
  shorter: readonly string[],
  longer: readonly string[],
): boolean {
  return shorter.every((step, index) => longer[index] === step);
}

/**
 * Bindings that would fire alongside `candidate`: the same keys, or one
 * sequence prefixing the other, in a scope where both can be active.
 */
export function findKeybindingConflicts(
  bindings: readonly KeyBinding[],
  candidate: KeyBinding,
): KeyBinding[] {
  const steps = keySteps(candidate);
  return bindings.filter((other) => {
    if (other.actionId === candidate.actionId) return false;
    const otherSteps = keySteps(other);
    const keysCollide =
      otherSteps.length <= steps.length
        ? isPrefixOrEqual(otherSteps, steps)
        : isPrefixOrEqual(steps, otherSteps);
    return keysCollide && hasSharedScope(candidate, other);
  });
}
