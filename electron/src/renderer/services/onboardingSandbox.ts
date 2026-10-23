import { appStore } from '../store';
import {
  INITIAL_SANDBOX_STATE,
  reduceSandbox,
} from '../utils/onboardingSandbox';

/** Mounts (or resets) the onboarding sandbox. */
export function openSandbox(): void {
  appStore.setState((state) => ({
    ...state,
    onboardingSandbox: INITIAL_SANDBOX_STATE,
  }));
}

/** Unmounts the onboarding sandbox. */
export function closeSandbox(): void {
  appStore.setState((state) => ({ ...state, onboardingSandbox: null }));
}

/** Applies a fake PR action only while the sandbox is mounted. */
export function dispatchSandboxAction(actionId: string): void {
  appStore.setState((state) =>
    state.onboardingSandbox === null
      ? state
      : {
          ...state,
          onboardingSandbox: reduceSandbox(state.onboardingSandbox, actionId),
        },
  );
}
