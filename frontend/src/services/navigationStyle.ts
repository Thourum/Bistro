import {
  NAVIGATION_STYLES,
  type NavigationStyle,
} from '@renderer/types/electron/settings';
import type { Fault, ResultAsync } from '@thourum/fault';
import { addToast } from '../components/ToastContainer';
import { settingsStore, updateSettings } from '../store/settings';

/** Persist the style so the real hotkey layer switches immediately. */
export function setNavigationStyle(
  style: NavigationStyle,
): ResultAsync<void, Fault> {
  return updateSettings({ navigation: { style } });
}

/** The neighbouring style in picker order, wrapping at both ends. */
export function neighbourNavigationStyle(
  current: NavigationStyle,
  delta: 1 | -1,
): NavigationStyle {
  const index = NAVIGATION_STYLES.indexOf(current) + delta;
  // `.at` wraps -1 to the last style; `%` wraps past-the-end to the first.
  return NAVIGATION_STYLES.at(index % NAVIGATION_STYLES.length) ?? current;
}

/** Step to the neighbouring style; reports a failed save as a toast. */
export function cycleNavigationStyle(delta: 1 | -1): void {
  const next = neighbourNavigationStyle(
    settingsStore.state.navigation.style,
    delta,
  );
  void setNavigationStyle(next).match(
    () => undefined,
    (error) =>
      addToast({
        type: 'error',
        message: `Navigation style could not be saved: ${error.message}`,
        dismissible: true,
      }),
  );
}
