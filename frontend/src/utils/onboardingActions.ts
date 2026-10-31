import { cycleNavigationStyle } from '../services/navigationStyle';
import { updateOnboardingSelection } from '../services/onboardingSync';
import { appStore } from '../store';
import type { KeyActionContext } from './keybindingActions';
import {
  moveOnboardingCursor,
  selectAllOnboardingRepos,
  toggleFocusedOnboardingRepo,
} from './onboardingRepos';

export const ONBOARDING_ACTIONS: Readonly<
  Record<string, (context: KeyActionContext) => void>
> = {
  'onboarding.continue': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'onboarding_continue',
    })),
  'onboarding.repoDown': () =>
    updateOnboardingSelection((state) => moveOnboardingCursor(state, 1)),
  'onboarding.repoUp': () =>
    updateOnboardingSelection((state) => moveOnboardingCursor(state, -1)),
  'onboarding.repoToggle': () =>
    updateOnboardingSelection(toggleFocusedOnboardingRepo),
  'onboarding.repoAll': () =>
    updateOnboardingSelection((state) =>
      selectAllOnboardingRepos(state, { watched: true }),
    ),
  'onboarding.repoNone': () =>
    updateOnboardingSelection((state) =>
      selectAllOnboardingRepos(state, { watched: false }),
    ),
  'onboarding.repoFilter': () =>
    appStore.setState((state) => ({
      ...state,
      onboardingIntent: 'focusRepoFilter',
    })),
  'onboarding.chooseFolder': () =>
    appStore.setState((state) => ({
      ...state,
      onboardingIntent: 'chooseFolder',
    })),
  'onboarding.stopScan': () =>
    appStore.setState((state) => ({ ...state, onboardingIntent: 'stopScan' })),
  'onboarding.nextStyle': () => cycleNavigationStyle(1),
  'onboarding.prevStyle': () => cycleNavigationStyle(-1),
};
