import { type Fault, okAsync, type ResultAsync } from '@thourum/fault';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import {
  INITIAL_FIRST_RUN_RAIL,
  reduceFirstRunRail,
} from '../utils/firstRunRail';
import { addToast } from './toasts';

function setDismissPending(firstRunDismissPending: boolean): void {
  appStore.setState((state) => ({ ...state, firstRunDismissPending }));
}

/**
 * Counts a dispatched live action toward the rail, only between finishing
 * onboarding and dismissing the rail. Session-only by design.
 */
export function recordFirstRunAction(actionId: string): void {
  const { onboardingComplete, tourComplete } = settingsStore.state.appearance;
  if (!onboardingComplete || tourComplete) return;
  appStore.setState((state) => {
    const firstRunRail = reduceFirstRunRail(state.firstRunRail, actionId);
    return firstRunRail === state.firstRunRail
      ? state
      : { ...state, firstRunRail };
  });
}

/**
 * Persists the rail's dismissal. tourComplete is not set optimistically:
 * updateSettings writes it into settingsStore only once main confirms.
 */
export function dismissFirstRunRail(): ResultAsync<void, Fault> {
  if (appStore.state.firstRunDismissPending) return okAsync(undefined);
  setDismissPending(true);
  return updateSettings({
    appearance: { tourComplete: true },
  })
    .map(() => setDismissPending(false))
    .mapErr((error) => {
      setDismissPending(false);
      return error;
    });
}

/** `:tour`: brings the rail back with its steps reset. */
export function showFirstRunRail(): ResultAsync<void, Fault> {
  return updateSettings({
    appearance: { tourComplete: false },
  }).map(() =>
    appStore.setState((state) => ({
      ...state,
      firstRunRail: INITIAL_FIRST_RUN_RAIL,
    })),
  );
}

/** Hide/Close from the rail's buttons or its key: one toast on failure, rail stays retryable. */
export function hideFirstRunRail(): void {
  void dismissFirstRunRail().match(
    () => undefined,
    (error) =>
      addToast({
        type: 'error',
        message: `Couldn't hide the first-review rail: ${error.message}`,
        dismissible: true,
      }),
  );
}
