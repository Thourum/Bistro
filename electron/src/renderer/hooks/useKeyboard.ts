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

const MODIFIER_KEYS = new Set(['Shift', 'Control', 'Alt', 'Meta']);

/**
 * Resolves the canonical key string from a KeyboardEvent.
 *
 * Modifier combos are expressed as prefix+key strings:
 *   ctrl+d, meta+q, alt+F4
 * Plain keys are returned as event.key.
 */
export function resolveEventKey(event: KeyboardEvent): string {
  if (event.ctrlKey && !MODIFIER_KEYS.has(event.key)) {
    return `ctrl+${event.key}`;
  }
  if (event.metaKey && !MODIFIER_KEYS.has(event.key)) {
    return `meta+${event.key}`;
  }
  if (event.altKey && !MODIFIER_KEYS.has(event.key)) {
    return `alt+${event.key}`;
  }
  return event.key;
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
 * Finds a matching binding for a resolved key string.
 *
 * Matching strategy:
 * 1. Modifier combos (keys[0] contains '+') — always single-entry, matched directly.
 * 2. Two-plain-key timed sequences — pendingKey + key within timeout.
 * 3. Single plain-key binding.
 */
export function findBinding(
  key: string,
  mode: AppMode,
  pendingKey: string | undefined,
  pendingKeyTime: number | undefined
): { binding: BindingDefinition; consumed: boolean } | null {
  // 1. Modifier combo (e.g. 'ctrl+d', 'meta+q')
  if (key.includes('+')) {
    const modifierBinding = KEYBOARD_BINDING_GROUPS.find(
      (b) =>
        isBindingActiveForMode(b, mode) &&
        b.keys.length === 1 &&
        b.keys[0] === key
    );
    if (modifierBinding !== undefined) {
      return { binding: modifierBinding, consumed: false };
    }
    return null;
  }

  const now = Date.now();
  const withinTimeout =
    pendingKey !== undefined &&
    pendingKeyTime !== undefined &&
    now - pendingKeyTime <= SEQUENCE_TIMEOUT_MS;

  // 2. Two-plain-key timed sequence
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

  // 3. Single plain-key binding
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
 * Returns true when the keyboard event originates from an editable element.
 * Covers standard form controls, contenteditable nodes, and ARIA textboxes.
 */
function isInputFocused(): boolean {
  const active = document.activeElement as HTMLElement | null;
  if (active === null) return false;
  const tag = active.tagName.toLowerCase();
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
  if (active.isContentEditable) return true;
  return false;
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
      if (MODIFIER_KEYS.has(event.key)) return;

      const { lastKey, lastKeyTime } = stateRef.current;
      const key = resolveEventKey(event);

      // Special case: digit keys 1–9 jump directly to the Nth PR.
      if (mode === 'list' && /^[1-9]$/.test(key)) {
        event.preventDefault();
        appStore.setState((s) => ({ ...s, selectedPRIndex: parseInt(key, 10) - 1 }));
        stateRef.current = { lastAction: key };
        return;
      }

      const result = findBinding(key, mode, lastKey, lastKeyTime);

      if (result !== null) {
        event.preventDefault();
        result.binding.action(appStore);
        stateRef.current = { lastAction: bindingSequence(result.binding) };
        return;
      }

      // Check if this plain key could be the start of a two-key sequence.
      // Modifier combos never start sequences.
      if (!key.includes('+')) {
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
