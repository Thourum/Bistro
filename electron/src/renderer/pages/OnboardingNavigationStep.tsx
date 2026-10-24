import { type ReactElement, useEffect, useState } from 'react';
import {
  NAVIGATION_STYLE_CARDS,
  NavigationStylePicker,
} from '../components/NavigationStylePicker';
import { Kbd } from '../components/ui/kbd';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { setNavigationStyle } from '../services/navigationStyle';
import { closeSandbox, openSandbox } from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { findActionKeys } from '../utils/legendPresets';
import { OnboardingPracticeTasks } from './OnboardingPracticeTasks';
import { OnboardingSandbox } from './OnboardingSandbox';

interface OnboardingNavigationStepProps {
  /** Receives a save failure message, or `null` once a save succeeds. */
  readonly onErrorChange: (message: string | null) => void;
}

export function OnboardingNavigationStep({
  onErrorChange,
}: OnboardingNavigationStepProps): ReactElement {
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const sandbox = useStore(appStore, (s) => s.onboardingSandbox);
  const bindings = useEffectiveBindings();
  const [startedAt] = useState(Date.now);

  // Mount-only: switching style keeps practice progress.
  useEffect(() => {
    openSandbox();
    return closeSandbox;
  }, []);

  const nextStyleKeys = findActionKeys(bindings, ['onboarding.nextStyle']);
  const prevStyleKeys = findActionKeys(bindings, ['onboarding.prevStyle']);

  return (
    <section className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-6">
      <div className="flex flex-col gap-8 min-w-0">
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h2 className="mono-text text-[10px] text-outline uppercase tracking-widest">
              NAVIGATION_STYLE
            </h2>
            <span className="flex items-center gap-1.5 mono-text text-[10px] text-outline">
              {nextStyleKeys !== undefined && <Kbd>{nextStyleKeys}</Kbd>}
              {nextStyleKeys !== undefined && prevStyleKeys !== undefined && (
                <span aria-hidden="true">/</span>
              )}
              {prevStyleKeys !== undefined && <Kbd>{prevStyleKeys}</Kbd>}
            </span>
          </div>
          <NavigationStylePicker
            layout="vertical"
            selected={style}
            onSelect={(next) =>
              void setNavigationStyle(next).match(
                () => onErrorChange(null),
                (error) =>
                  onErrorChange(
                    `Navigation style could not be saved: ${error.message}`,
                  ),
              )
            }
          />
          {/* Tab / Shift+Tab switch styles without moving focus, so announce it. */}
          <p className="sr-only" aria-live="polite">
            {NAVIGATION_STYLE_CARDS[style].label} navigation selected
          </p>
        </div>
        {sandbox !== null && (
          <OnboardingPracticeTasks state={sandbox} startedAt={startedAt} />
        )}
      </div>
      <div className="min-w-0">
        {sandbox !== null && <OnboardingSandbox state={sandbox} />}
      </div>
    </section>
  );
}
