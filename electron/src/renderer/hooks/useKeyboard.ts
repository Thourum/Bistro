import { useEffect, useRef } from 'react';
import { appStore } from '../store';
import { KEYBOARD_BINDING_GROUPS, bindingSequence } from '../utils/keyboard';
import type { AppMode, BindingDefinition } from '../utils/keyboard';

const SEQUENCE_TIMEOUT_MS = 500;

export interface KeyboardState {
  lastKey?: string;
  lastKeyTime?: number;
  lastAction?: string;
}

/**
 * Determines whether a binding is active for the given mode.
 * Bindings with an empty modes array are active in all modes.
 */
function isBindingActiveForMode(binding: BindingDefinition, mode: AppMode): boolean {
  if (binding.modes.length === 0) return true;
  return binding.modes.includes(mode);
}

/**
 * Finds a matching binding for a key or key sequence.
 * Returns the binding if found, or null.
 *
 * Matching strategy:
 * - If pendingKey is set and within timeout: try two-key sequence first.
 * - Fall back to single-key binding.
 */
function findBinding(
  key: string,
  mode: AppMode,
  pendingKey: string | undefined,
  pendingKeyTime: number | undefined
): { binding: BindingDefinition; consumed: boolean } | null {
  const now = Date.now();
  const withinTimeout =
    pendingKey !== undefined &&
    pendingKeyTime !== undefined &&
    now - pendingKeyTime <= SEQUENCE_TIMEOUT_MS;

  // Try two-key sequence first when a pending key exists
  if (withinTimeout && pendingKey !== undefined) {
    const sequence = pendingKey + key;
    const seqBinding = KEYBOARD_BINDING_GROUPS.find(
      (b) =>
        isBindingActiveForMode(b, mode) &&
        b.keys.length === 2 &&
        bindingSequence(b) === sequence
    );
    if (seqBinding !== undefined) {
      return { binding: seqBinding, consumed: true };
    }
  }

  // Try single-key binding
  const singleBinding = KEYBOARD_BINDING_GROUPS.find(
    (b) =>
      isBindingActiveForMode(b, mode) &&
      b.keys.length === 1 &&
      b.keys[0] === key
  );

  if (singleBinding !== undefined) {
    return { binding: singleBinding, consumed: false };
  }

  return null;
}

/**
 * Returns true when the keyboard event originates from an input element.
 * In search mode the search input should handle its own characters.
 */
function isInputFocused(): boolean {
  const active = document.activeElement;
  if (active === null) return false;
  const tag = active.tagName.toLowerCase();
  return tag === 'input' || tag === 'textarea' || tag === 'select';
}

/**
 * Core keyboard hook.
 *
 * Attaches a single keydown listener to the window, tracks multi-key sequences,
 * and dispatches store actions for matched bindings.
 *
 * Usage:
 *   function AppContent() {
 *     useKeyboard();
 *     ...
 *   }
 */
export function useKeyboard(): void {
  const stateRef = useRef<KeyboardState>({});

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent): void {
      const mode = appStore.state.mode as AppMode;

      // In search mode, let the input element handle regular characters.
      // Only process Escape (to exit search) and nothing else.
      if (mode === 'search') {
        if (event.key !== 'Escape') return;
      }

      // Skip when focus is inside an input unless we are processing Escape.
      if (isInputFocused() && event.key !== 'Escape') return;

      // Skip modifier-only key presses (Shift, Control, Alt, Meta alone)
      const MODIFIER_KEYS = new Set(['Shift', 'Control', 'Alt', 'Meta']);
      if (MODIFIER_KEYS.has(event.key)) return;

      const { lastKey, lastKeyTime } = stateRef.current;
      const key = event.key;

      const result = findBinding(key, mode, lastKey, lastKeyTime);

      if (result !== null) {
        event.preventDefault();
        result.binding.action(appStore);
        stateRef.current = { lastAction: bindingSequence(result.binding) };
        return;
      }

      // Check if this key could be the start of a multi-key sequence.
      const couldStartSequence = KEYBOARD_BINDING_GROUPS.some(
        (b) =>
          isBindingActiveForMode(b, mode) &&
          b.keys.length === 2 &&
          b.keys[0] === key
      );

      if (couldStartSequence) {
        stateRef.current = { lastKey: key, lastKeyTime: Date.now() };
        return;
      }

      // Unbound key: reset pending state.
      stateRef.current = {};
    }

    window.addEventListener('keydown', handleKeyDown);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, []);
}
