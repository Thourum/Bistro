import { appStore } from '../store';
import {
  INITIAL_SANDBOX_STATE,
  reduceSandbox,
  type SandboxPane,
  selectSandboxPane,
} from '../utils/onboardingSandbox';

/** Mounts (or resets) the onboarding sandbox. */
export function openSandbox(): void {
  appStore.setState((state) => ({
    ...state,
    onboardingSandbox: INITIAL_SANDBOX_STATE,
  }));
}

/** Unmounts the onboarding sandbox, keeping whether the diff was practiced. */
export function closeSandbox(): void {
  appStore.setState((state) => {
    const completed = state.onboardingSandbox?.completed ?? [];
    return {
      ...state,
      onboardingPracticedDiff:
        state.onboardingPracticedDiff ||
        completed.includes('viewDiff') ||
        completed.includes('sinceLastReview'),
      onboardingSandbox: null,
    };
  });
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

/** Shows a fake pane (stage tab) without completing a task. */
export function selectOnboardingSandboxPane(pane: SandboxPane): void {
  appStore.setState((state) =>
    state.onboardingSandbox === null
      ? state
      : {
          ...state,
          onboardingSandbox: selectSandboxPane(state.onboardingSandbox, pane),
        },
  );
}
