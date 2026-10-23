import { type ReactElement, useEffect, useMemo } from 'react';
import {
  NAVIGATION_STYLE_CARDS,
  NavigationStylePicker,
} from '../components/NavigationStylePicker';
import { Kbd } from '../components/ui/kbd';
import { useStore } from '../hooks/useStore';
import { cn } from '../lib/utils';
import { setNavigationStyle } from '../services/navigationStyle';
import { closeSandbox, openSandbox } from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { getEffectiveBindings } from '../utils/keybindings';
import { formatActionKeys } from '../utils/legendPresets';
import { SANDBOX_TASKS } from '../utils/onboardingSandbox';
import { OnboardingSandbox } from './OnboardingSandbox';

interface OnboardingNavigationStepProps {
  /** Receives a save failure message, or `null` once a save succeeds. */
  readonly onErrorChange: (message: string | null) => void;
}

export function OnboardingNavigationStep({
  onErrorChange,
}: OnboardingNavigationStepProps): ReactElement {
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const sandbox = useStore(appStore, (s) => s.onboardingSandbox);

  // biome-ignore lint/correctness/useExhaustiveDependencies: switching style restarts practice from the inbox pane.
  useEffect(() => {
    openSandbox();
    return closeSandbox;
  }, [style]);

  const keys = useMemo(() => {
    const bindings = getEffectiveBindings(style, overrides);
    return {
      nextStyle: formatActionKeys(bindings, ['onboarding.nextStyle']),
      prevStyle: formatActionKeys(bindings, ['onboarding.prevStyle']),
      tasks: SANDBOX_TASKS.map((task) => ({
        task,
        keys: formatActionKeys(bindings, task.actionIds),
      })),
    };
  }, [style, overrides]);

  const completed = sandbox?.completed ?? [];

  return (
    <section className="space-y-8">
      <div className="space-y-3">
        <p className="flex items-center gap-2 mono-text text-[10px] text-outline uppercase tracking-widest">
          <Kbd>{keys.nextStyle}</Kbd>
          <Kbd>{keys.prevStyle}</Kbd>
          <span>to switch styles</span>
        </p>
        <NavigationStylePicker
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
      <div className="grid grid-cols-1 md:grid-cols-12 border border-outline-variant/30">
        <div className="md:col-span-8">
          {sandbox !== null && <OnboardingSandbox state={sandbox} />}
        </div>
        <div className="md:col-span-4 bg-surface-container-low p-4 border-t md:border-t-0 md:border-l border-outline-variant/30">
          <div className="flex items-baseline justify-between mb-3">
            <h2 className="mono-text text-xs font-bold uppercase tracking-widest">
              Try it
            </h2>
            <span className="mono-text text-[10px] text-outline">
              {completed.length} / {SANDBOX_TASKS.length}
            </span>
          </div>
          <ol aria-label="Practice checklist" className="space-y-2">
            {keys.tasks.map(({ task, keys: taskKeys }) => {
              const isDone = completed.includes(task.id);
              return (
                <li
                  key={task.id}
                  className="flex items-center justify-between gap-3 text-sm"
                >
                  <span className="flex items-center gap-2 min-w-0">
                    <span
                      aria-hidden="true"
                      className={cn(
                        'material-symbols-outlined text-sm',
                        isDone ? 'text-primary' : 'text-outline-variant',
                      )}
                    >
                      {isDone ? 'check' : 'radio_button_unchecked'}
                    </span>
                    <span
                      className={
                        isDone ? 'text-outline line-through' : 'text-on-surface'
                      }
                    >
                      {task.label}
                    </span>
                    {isDone && <span className="sr-only">(done)</span>}
                  </span>
                  <Kbd className="shrink-0">{taskKeys}</Kbd>
                </li>
              );
            })}
          </ol>
          <p className="mt-4 text-xs text-on-surface-variant leading-relaxed">
            Practice is optional. Nothing here reaches GitHub.
          </p>
        </div>
      </div>
    </section>
  );
}
