import { useMemo } from 'react';
import { settingsStore } from '../store/settings';
import { getEffectiveBindings, type KeyBinding } from '../utils/keybindings';
import { useStore } from './useStore';

/** Effective bindings for the current navigation style and user overrides. */
export function useEffectiveBindings(): KeyBinding[] {
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  return useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );
}
