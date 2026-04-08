/**
 * useKeyboardIntegration
 *
 * Thin integration hook that activates the keyboard event handler for the
 * whole application. The underlying useKeyboard hook already wires every
 * binding directly to appStore, so this layer exists solely to provide a
 * named, testable entry-point that App.tsx can call without coupling itself
 * to the keyboard implementation details.
 */
import { useKeyboard } from './useKeyboard';

export function useKeyboardIntegration(): void {
  useKeyboard();
}
