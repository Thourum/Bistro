import {
  useHotkeys,
  useHotkeySequences,
  type UseHotkeyDefinition,
  type UseHotkeySequenceDefinition,
} from '@tanstack/react-hotkeys';
import { useMemo } from 'react';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import {
  getEffectiveBindings,
  isSequenceBinding,
  type AppMode,
  type KeyBinding,
} from '../utils/keybindings';
import { useStore } from './useStore';

/** Sequence steps must follow each other within this window (was 500ms before the migration). */
const SEQUENCE_TIMEOUT_MS = 500;

function isActiveInMode(binding: KeyBinding, mode: AppMode): boolean {
  return binding.modes.length === 0 || binding.modes.includes(mode);
}

/**
 * Registers every effective keybinding (defaults + settings overrides)
 * through TanStack Hotkeys. Bindings are mode-gated via the `enabled`
 * option; overlapping keys across modes are intentional, hence
 * `conflictBehavior: 'allow'`.
 *
 * `Escape` stays active while typing (`ignoreInputs: false`) so it can exit
 * search and comment inputs; everything else is suppressed in form fields.
 */
export function useAppHotkeys(): void {
  const mode = useStore(appStore, (s) => s.mode) as AppMode;
  const overrides = useStore(settingsStore, (s) => s.keybindings);

  const bindings = useMemo(() => getEffectiveBindings(overrides), [overrides]);

  const hotkeyDefs = useMemo<UseHotkeyDefinition[]>(
    () =>
      bindings.filter((b) => !isSequenceBinding(b)).map((binding) => ({
        hotkey: binding.keys as string,
        callback: () => binding.action(appStore),
        options: {
          enabled: isActiveInMode(binding, mode),
          ignoreInputs: binding.keys !== 'Escape',
        },
      })),
    [bindings, mode],
  );

  const sequenceDefs = useMemo<UseHotkeySequenceDefinition[]>(
    () =>
      bindings.filter(isSequenceBinding).map((binding) => ({
        sequence: [...binding.keys],
        callback: () => binding.action(appStore),
        options: {
          enabled: isActiveInMode(binding, mode),
          ignoreInputs: true,
        },
      })),
    [bindings, mode],
  );

  useHotkeys(hotkeyDefs, { conflictBehavior: 'allow' });
  useHotkeySequences(sequenceDefs, {
    conflictBehavior: 'allow',
    timeout: SEQUENCE_TIMEOUT_MS,
  });
}
