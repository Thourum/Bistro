import React from 'react';
import { useStore } from '../hooks/useStore';
import { setOnboardingStep } from '../services/onboardingSync';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import { isSandboxComplete } from '../utils/onboardingSandbox';
import { OnboardingDevFolderStep } from './OnboardingDevFolderStep';
import { OnboardingFooter } from './OnboardingFooter';
import { OnboardingNavigationStep } from './OnboardingNavigationStep';
import { useOnboardingSync } from './useOnboardingSync';

const SUBTITLE = {
  sync: 'Choose what to sync. Your inbox starts filling while you practise.',
  practice: 'Pick the keys your hands already know, then try the review loop.',
} as const satisfies Record<'sync' | 'practice', string>;

/** First run: choose what to sync, then practise the keys, then Finish. */
export function OnboardingPage(): React.ReactElement {
  const confirmingAction = useStore(appStore, (s) => s.confirmingAction);
  const step = useStore(appStore, (s) => s.onboardingStep) ?? 'sync';
  const isPracticeComplete = useStore(appStore, (s) =>
    isSandboxComplete(s.onboardingSandbox),
  );
  const [isFinishing, setIsFinishing] = React.useState(false);
  const [stepError, setStepError] = React.useState<string | null>(null);
  const sync = useOnboardingSync();
  const { canContinue, continueFromSync } = sync;

  React.useEffect(() => {
    setOnboardingStep('sync');
    return () => setOnboardingStep(null);
  }, []);

  const advance = React.useCallback(() => {
    if (isFinishing) return;
    if (appStore.state.onboardingStep === 'sync') {
      if (!canContinue) return;
      continueFromSync();
      return;
    }
    setIsFinishing(true);
    // Success flips onboardingComplete; Bootstrap's router invalidation leaves.
    void updateSettings({
      appearance: {
        ...settingsStore.state.appearance,
        onboardingComplete: true,
      },
    }).match(
      () => setIsFinishing(false),
      (fault) => {
        setIsFinishing(false);
        setStepError(`Onboarding could not be completed: ${fault.message}`);
      },
    );
  }, [isFinishing, canContinue, continueFromSync]);

  React.useEffect(() => {
    if (confirmingAction !== 'onboarding_continue') return;
    appStore.setState((state) => ({ ...state, confirmingAction: null }));
    advance();
  }, [confirmingAction, advance]);

  return (
    <div className="flex flex-col flex-1 min-h-0 h-full w-full bg-background text-on-surface">
      <main className="grow min-h-0 min-w-0 w-full overflow-auto p-6 md:px-12 md:py-8">
        <div className="max-w-screen-2xl mx-auto w-full min-h-full flex flex-col">
          <header className="mb-6">
            <h1 className="flex items-center gap-3 mb-3">
              <span
                aria-hidden="true"
                className="flex w-9 h-9 items-center justify-center border-2 border-on-surface text-xl font-black text-on-surface"
              >
                B
              </span>
              <span className="text-2xl font-black tracking-tight text-on-surface uppercase">
                BISTRO
              </span>
            </h1>
            <p className="text-base text-on-surface-variant leading-snug">
              {SUBTITLE[step]}
            </p>
          </header>

          {step === 'sync' ? (
            <OnboardingDevFolderStep sync={sync} />
          ) : (
            <OnboardingNavigationStep onErrorChange={setStepError} />
          )}
          {stepError !== null && (
            <p className="mt-4 text-sm text-error font-medium" role="alert">
              {stepError}
            </p>
          )}
        </div>
      </main>

      <OnboardingFooter
        step={step}
        canContinue={step === 'practice' || canContinue}
        isFinishing={isFinishing}
        isPracticeComplete={isPracticeComplete}
        onContinue={advance}
      />
    </div>
  );
}
