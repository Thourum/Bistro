import React from 'react';
import { Button } from '../components/ui/button';
import { Kbd } from '../components/ui/kbd';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import { getEffectiveBindings } from '../utils/keybindings';
import { formatActionKeys } from '../utils/legendPresets';
import { OnboardingDevFolderStep } from './OnboardingDevFolderStep';
import { OnboardingNavigationStep } from './OnboardingNavigationStep';
import { useOnboardingSync } from './useOnboardingSync';

/** One-time navigation practice, followed by the optional development folder. */
export function OnboardingPage(): React.ReactElement {
  const confirmingAction = useStore(appStore, (s) => s.confirmingAction);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const [step, setStep] = React.useState<'navigation' | 'devFolder'>(
    'navigation',
  );
  const [stepError, setStepError] = React.useState<string | null>(null);
  const sync = useOnboardingSync();

  const continueKeys = React.useMemo(
    () =>
      formatActionKeys(getEffectiveBindings(style, overrides), [
        'onboarding.continue',
      ]),
    [style, overrides],
  );

  const finish = React.useCallback(() => {
    void updateSettings({
      appearance: {
        ...settingsStore.state.appearance,
        onboardingComplete: true,
      },
    }).match(
      () => undefined,
      (error) =>
        setStepError(`Onboarding could not be completed: ${error.message}`),
    );
  }, []);

  const advance = React.useCallback(() => {
    if (step === 'navigation') {
      setStepError(null);
      setStep('devFolder');
      return;
    }
    finish();
  }, [step, finish]);

  React.useEffect(() => {
    if (confirmingAction !== 'onboarding_continue') return;
    appStore.setState((state) => ({ ...state, confirmingAction: null }));
    advance();
  }, [confirmingAction, advance]);

  return (
    <div className="flex flex-col flex-1 min-h-0 h-full w-full bg-background text-on-surface">
      <main className="grow flex items-center justify-center p-6 min-h-0 w-full min-w-0 overflow-auto">
        <div
          className={`${step === 'navigation' ? 'max-w-3xl' : 'max-w-6xl'} w-full`}
        >
          <header className="mb-8">
            <h1 className="text-5xl font-black tracking-tighter text-on-surface uppercase mb-2">
              BISTRO
            </h1>
            <p className="text-xl text-on-surface-variant font-medium max-w-lg leading-snug">
              {step === 'navigation'
                ? 'Pick the keys your hands already know, then try them on a practice review.'
                : 'Choose what to sync. Bistro starts from the PRs already waiting on you.'}
            </p>
          </header>

          {step === 'navigation' ? (
            <OnboardingNavigationStep onErrorChange={setStepError} />
          ) : (
            <OnboardingDevFolderStep sync={sync} />
          )}
          {stepError !== null && (
            <p className="mt-4 text-sm text-error font-medium" role="alert">
              {stepError}
            </p>
          )}
        </div>
      </main>

      <footer className="bg-surface-container-lowest py-8 flex flex-col items-center border-t border-outline-variant/20 shrink-0">
        <div className="flex items-center gap-4">
          <span className="mono-text text-xs text-outline">
            <span className="sr-only">Step </span>
            {step === 'navigation' ? '1 / 2' : '2 / 2'}
          </span>
          <Button
            className="bg-primary! text-on-primary! border-primary! hover:bg-on-primary-fixed-variant!"
            onClick={advance}
          >
            {step === 'navigation' ? 'Continue' : 'Finish'}
          </Button>
          <span className="flex items-center gap-2 mono-text text-[10px] text-outline uppercase tracking-widest">
            or press <Kbd>{continueKeys}</Kbd>
          </span>
        </div>
      </footer>
    </div>
  );
}
