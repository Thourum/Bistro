import { appStore } from '../store';
import {
  closeSandboxComposer,
  INITIAL_SANDBOX_STATE,
  reduceSandbox,
  type SandboxPane,
  type SandboxState,
  selectSandboxPane,
  selectSandboxPr,
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

/** Mouse-selects a fake inbox row without counting as a movement task. */
export function selectOnboardingSandboxPr(cursor: number): void {
  applySandbox((state) => selectSandboxPr(state, cursor));
}

/** Closes the fake composer the user pressed Escape in. */
export function closeOnboardingSandboxComposer(): void {
  applySandbox(closeSandboxComposer);
}

function applySandbox(update: (state: SandboxState) => SandboxState): void {
  appStore.setState((state) =>
    state.onboardingSandbox === null
      ? state
      : { ...state, onboardingSandbox: update(state.onboardingSandbox) },
  );
}
