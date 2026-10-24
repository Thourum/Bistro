import { type ReactElement, useEffect, useMemo, useState } from 'react';
import { Kbd } from '../components/ui/kbd';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { cn } from '../lib/utils';
import { selectOnboardingSandboxPane } from '../services/onboardingSandbox';
import { findActionKeys } from '../utils/legendPresets';
import {
  firstUnfinishedSandboxStage,
  SANDBOX_STAGES,
  SANDBOX_TASKS,
  type SandboxStage,
  type SandboxState,
} from '../utils/onboardingSandbox';

const LABEL = 'mono-text text-[10px] uppercase tracking-widest';
const MS_PER_SECOND = 1_000;
const SECONDS_PER_MINUTE = 60;

/** A stage the user clicked, kept until a new task completes. */
interface ManualStage {
  readonly stage: SandboxStage;
  readonly completedCount: number;
}

function formatElapsed(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / MS_PER_SECOND));
  const rest = String(seconds % SECONDS_PER_MINUTE).padStart(2, '0');
  return `${Math.floor(seconds / SECONDS_PER_MINUTE)}:${rest}`;
}

const pad2 = (value: number): string => String(value).padStart(2, '0');

export function OnboardingPracticeTasks({
  state,
  startedAt,
}: {
  readonly state: SandboxState;
  readonly startedAt: number;
}): ReactElement {
  const bindings = useEffectiveBindings();
  const [manual, setManual] = useState<ManualStage | null>(null);
  const [completedAt, setCompletedAt] = useState<number | null>(null);
  const { completed } = state;
  const isComplete = completed.length === SANDBOX_TASKS.length;

  // Frozen once: the elapsed time is from entering Step 2 to the last task.
  useEffect(() => {
    if (isComplete) setCompletedAt((at) => at ?? Date.now());
  }, [isComplete]);

  const automatic = firstUnfinishedSandboxStage(state);
  const activeStage =
    manual !== null &&
    (manual.completedCount === completed.length || automatic === null)
      ? manual.stage
      : (automatic ?? 'diff');

  const tasks = useMemo(
    () =>
      SANDBOX_TASKS.map((task) => ({
        task,
        keys: findActionKeys(bindings, task.actionIds),
      })),
    [bindings],
  );
  const stageTasks = tasks.filter((entry) => entry.task.stage === activeStage);
  const currentId = stageTasks.find(
    (entry) => !completed.includes(entry.task.id),
  )?.task.id;
  const continueKeys = findActionKeys(bindings, ['onboarding.continue']);

  return (
    <section aria-label="Practice checklist" className="space-y-3">
      <div className="flex items-baseline justify-between">
        <h2 className={cn(LABEL, 'text-outline')}>TRY_IT</h2>
        <span
          className={cn(
            'mono-text text-xs',
            isComplete ? 'text-green-600' : 'text-outline',
          )}
        >
          {completed.length} / {SANDBOX_TASKS.length}
        </span>
      </div>

      <div
        role="tablist"
        aria-label="Practice stages"
        className="grid grid-cols-3"
      >
        {SANDBOX_STAGES.map((stage, index) => {
          const inStage = tasks.filter(
            (entry) => entry.task.stage === stage.id,
          );
          const done = inStage.filter((entry) =>
            completed.includes(entry.task.id),
          ).length;
          const isStageDone = done === inStage.length;
          const isActive = stage.id === activeStage;
          return (
            <button
              key={stage.id}
              type="button"
              role="tab"
              aria-selected={isActive}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                setManual({
                  stage: stage.id,
                  completedCount: completed.length,
                });
                selectOnboardingSandboxPane(stage.pane);
              }}
              className={cn(
                'flex flex-col items-start gap-1 border border-outline-variant/30! border-b-2! px-3 py-2.5 text-left',
                isActive
                  ? 'bg-surface-container-lowest! border-b-primary!'
                  : isStageDone
                    ? 'bg-surface-container-low!'
                    : 'bg-surface-container-lowest!',
              )}
            >
              <span
                className={cn(
                  'mono-text text-[10px]',
                  isStageDone
                    ? 'text-green-600'
                    : isActive
                      ? 'text-primary'
                      : 'text-outline',
                )}
              >
                {isStageDone && '✓ '}
                {`${pad2(index + 1)} // ${done}/${inStage.length}`}
              </span>
              <span
                className={cn(
                  'text-sm',
                  isActive
                    ? 'font-bold text-on-surface'
                    : 'text-on-surface-variant',
                )}
              >
                {stage.label}
              </span>
            </button>
          );
        })}
      </div>

      <ol aria-label={`${activeStage} tasks`}>
        {stageTasks.map(({ task, keys }, index) => {
          const isDone = completed.includes(task.id);
          const isCurrent = task.id === currentId;
          return (
            <li
              key={task.id}
              aria-current={isCurrent ? 'step' : undefined}
              className={cn(
                'grid grid-cols-[2rem_1fr_auto] items-start gap-x-2 px-3 py-2.5 border-l-2',
                isCurrent
                  ? 'border-primary bg-surface-container-low'
                  : 'border-transparent',
              )}
            >
              <span
                aria-hidden="true"
                className={cn(
                  'mono-text text-xs pt-0.5',
                  isDone || isCurrent ? 'text-primary' : 'text-outline',
                )}
              >
                {isDone ? '✓' : isCurrent ? '→' : pad2(index + 1)}
              </span>
              <span className="min-w-0">
                <span
                  className={cn(
                    'block text-sm',
                    isDone && 'text-outline line-through',
                    isCurrent && 'font-bold text-on-surface',
                  )}
                >
                  {task.label}
                  {isDone && <span className="sr-only"> (done)</span>}
                </span>
                {task.reason !== undefined && !isDone && (
                  <span className="block mt-0.5 text-xs text-on-surface-variant">
                    {task.reason}
                  </span>
                )}
              </span>
              {keys !== undefined && (
                <Kbd
                  className={cn(
                    isCurrent && 'bg-primary border-primary text-on-primary',
                  )}
                >
                  {keys}
                </Kbd>
              )}
            </li>
          );
        })}
      </ol>

      {isComplete && completedAt !== null && (
        <div className="border-t border-outline-variant/30 pt-3 space-y-1">
          <p className={cn(LABEL, 'text-green-600')}>
            {`PRACTICE_COMPLETE // ${formatElapsed(completedAt - startedAt)}`}
          </p>
          <p className="text-sm text-on-surface">
            That's the whole review loop.
            {continueKeys !== undefined && (
              <>
                {' '}
                Press{' '}
                <Kbd className="bg-primary border-primary text-on-primary">
                  {continueKeys}
                </Kbd>{' '}
                to continue.
              </>
            )}
          </p>
        </div>
      )}

      <p className="mono-text text-[11px] text-outline leading-relaxed">
        {'// '}Practice is optional and every stage can be skipped. Nothing here
        reaches GitHub.
      </p>
    </section>
  );
}
