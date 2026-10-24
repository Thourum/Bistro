import type React from 'react';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { cn } from '../lib/utils';
import { useOverlays } from '../overlays/OverlayContext';
import { hideFirstRunRail } from '../services/firstRunRail';
import { appStore } from '../store';
import type { PR } from '../types';
import type { FirstRunStep } from '../utils/firstRunRail';
import type { KeyBinding } from '../utils/keybindings';
import { findActionKeys } from '../utils/legendPresets';
import { Button } from './ui/button';
import { Kbd } from './ui/kbd';

const STEP_ORDER: readonly FirstRunStep[] = ['open', 'range', 'review'];

type StepState = 'done' | 'current' | 'upcoming';

/** Effective keys for each action, joined with ` / `; unbound actions drop out. */
function keysFor(
  bindings: readonly KeyBinding[],
  actionIds: readonly string[],
): string | undefined {
  const keys = actionIds
    .map((id) => findActionKeys(bindings, [id]))
    .filter((found) => found !== undefined);
  return keys.length === 0 ? undefined : keys.join(' / ');
}

function needsYouTitle(count: number): string {
  if (count === 0) return 'Nothing needs you yet';
  return count === 1 ? '1 PR needs you' : `${count} PRs need you`;
}

interface RailStepProps {
  index: number;
  state: StepState;
  title: string;
  keys: string | undefined;
  /** Shown whatever the step's state, e.g. the practice badge. */
  note?: React.ReactNode;
  children?: React.ReactNode;
}

function RailStep({
  index,
  state,
  title,
  keys,
  note,
  children,
}: RailStepProps): React.ReactElement {
  const isCurrent = state === 'current';
  const isDone = state === 'done';
  return (
    <li
      aria-current={isCurrent ? 'step' : undefined}
      className={cn(
        // -mx-6 bleeds the row to the rail edge; 22px + the 2px accent
        // border lines the content up with the rail's p-6.
        'flex items-start gap-4 border-l-2 py-4 pr-6 pl-[22px] -mx-6',
        isCurrent
          ? 'border-primary bg-surface-container-low'
          : 'border-transparent',
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          'w-4 shrink-0 pt-0.5 font-mono text-[10px]',
          isDone || isCurrent ? 'text-primary' : 'text-outline',
        )}
      >
        {isDone ? '✓' : isCurrent ? '→' : String(index + 1).padStart(2, '0')}
      </span>
      <div className="min-w-0 flex-1">
        <p
          className={cn(
            'text-sm',
            isDone && 'text-outline line-through',
            isCurrent && 'font-semibold text-on-surface',
            state === 'upcoming' && 'text-on-surface-variant',
          )}
        >
          {title}
          {isDone && <span className="sr-only"> (done)</span>}
        </p>
        {!isDone && (
          <div className="mt-1 space-y-1 text-xs text-on-surface-variant">
            {children}
          </div>
        )}
        {note}
      </div>
      {keys !== undefined && (
        <Kbd
          className={cn(
            'shrink-0',
            isCurrent && 'border-primary bg-primary text-on-primary',
          )}
        >
          {keys}
        </Kbd>
      )}
    </li>
  );
}

/**
 * Non-modal first-review checklist beside the live inbox. Purely
 * instructional: steps complete from dispatched live actions, and only
 * Hide/Close (or the firstRun.hide key) persists its dismissal.
 */
export function FirstRunRail({
  needsAction,
}: {
  readonly needsAction: readonly PR[];
}): React.ReactElement {
  const completed = useStore(appStore, (s) => s.firstRunRail.completed);
  const isDismissPending = useStore(appStore, (s) => s.firstRunDismissPending);
  const hasPracticedDiff = useStore(appStore, (s) => s.onboardingPracticedDiff);
  const bindings = useEffectiveBindings();
  const { openCommand, openHelp } = useOverlays();

  const currentIndex = STEP_ORDER.findIndex(
    (step) => !completed.includes(step),
  );
  const isDone = currentIndex === -1;
  const stateOf = (step: FirstRunStep): StepState => {
    if (completed.includes(step)) return 'done';
    return step === STEP_ORDER[currentIndex] ? 'current' : 'upcoming';
  };
  const topPR = needsAction[0];
  const hideKeys = findActionKeys(bindings, ['firstRun.hide']);
  const sinceLastReviewKeys = findActionKeys(bindings, [
    'diff.sinceLastReview',
  ]);
  const commandKeys = findActionKeys(bindings, ['global.command']);
  const helpKeys = findActionKeys(bindings, ['global.help']);
  const hideLabel = isDone ? 'Close' : 'Hide';

  return (
    <aside
      aria-label="First review"
      className="flex w-full shrink-0 flex-col border-t border-outline-variant/30 bg-surface-container-lowest lg:w-80 lg:overflow-y-auto lg:border-t-0 lg:border-l"
    >
      <div className="flex-1 p-6">
        <div className="flex items-center justify-between">
          <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
            {isDone
              ? 'FIRST_REVIEW // DONE'
              : `FIRST_REVIEW // ${currentIndex + 1} OF 3`}
          </span>
          <button
            type="button"
            onClick={hideFirstRunRail}
            disabled={isDismissPending}
            className="flex items-center gap-2 font-mono text-[10px] uppercase tracking-widest text-outline hover:text-primary disabled:opacity-50"
          >
            {hideKeys !== undefined && <Kbd>{hideKeys}</Kbd>}
            {hideLabel}
          </button>
        </div>
        <div aria-hidden="true" className="mt-3 grid grid-cols-3 gap-1">
          {STEP_ORDER.map((step, index) => (
            <span
              key={step}
              className={cn(
                'h-0.5',
                isDone || index <= currentIndex || completed.includes(step)
                  ? 'bg-primary'
                  : 'bg-outline-variant/30',
              )}
            />
          ))}
        </div>

        <h2 className="mt-8 text-2xl font-bold tracking-tight text-on-surface">
          {isDone ? "That's the loop." : needsYouTitle(needsAction.length)}
        </h2>
        <p className="mt-3 text-sm leading-relaxed text-on-surface-variant">
          {isDone
            ? 'Everything else is one key away. Close to keep this rail hidden.'
            : needsAction.length === 0
              ? 'Your review requests appear as they arrive.'
              : "They're in Needs Action because a review or follow-up needs your attention. The list updates as GitHub does."}
        </p>

        <ol className="mt-6">
          <RailStep
            index={0}
            state={stateOf('open')}
            title="Open a PR"
            keys={keysFor(bindings, ['list.open'])}
          >
            {topPR === undefined ? (
              <p>Open a PR when it appears.</p>
            ) : (
              <>
                <p>Start at the top of Needs Action.</p>
                <p className="truncate text-on-surface">
                  #{topPR.number} — {topPR.title}
                </p>
                <p className="font-mono text-[10px] text-outline">
                  {topPR.owner}/{topPR.repo}
                </p>
              </>
            )}
          </RailStep>
          <RailStep
            index={1}
            state={stateOf('range')}
            title="Open the diff"
            keys={keysFor(bindings, [
              'detail.viewDiff',
              'diff.sinceLastReview',
            ])}
            note={
              hasPracticedDiff && (
                <p className="mt-1 font-mono text-[10px] uppercase tracking-wider text-green-500">
                  ✓ PRACTICED IN ONBOARDING
                </p>
              )
            }
          >
            <p>
              After a previous review,{' '}
              {sinceLastReviewKeys === undefined ? (
                'the since-last-review range'
              ) : (
                <Kbd>{sinceLastReviewKeys}</Kbd>
              )}{' '}
              shows only what changed.
            </p>
          </RailStep>
          <RailStep
            index={2}
            state={stateOf('review')}
            title="Approve or comment"
            keys={keysFor(bindings, ['detail.approve', 'detail.comment'])}
          >
            <p>Opens the review controls. Nothing posts until you confirm.</p>
          </RailStep>
        </ol>

        {isDone && (
          <Button
            onClick={hideFirstRunRail}
            disabled={isDismissPending}
            className="mt-6 gap-2 bg-primary! text-on-primary! border-primary! hover:bg-on-primary-fixed-variant!"
          >
            Close
            {hideKeys !== undefined && (
              <Kbd className="border-on-primary/50 bg-transparent text-on-primary">
                {hideKeys}
              </Kbd>
            )}
          </Button>
        )}
      </div>

      <div className="border-t border-outline-variant/30 p-6">
        <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
          EVERYTHING ELSE
        </span>
        <div className="mt-3 flex flex-col items-start gap-2">
          <button
            type="button"
            onClick={openCommand}
            className="flex items-center gap-3 text-sm text-on-surface hover:text-primary"
          >
            {commandKeys !== undefined && <Kbd>{commandKeys}</Kbd>}
            Command palette
          </button>
          <button
            type="button"
            onClick={() => openHelp('list')}
            className="flex items-center gap-3 text-sm text-on-surface hover:text-primary"
          >
            {helpKeys !== undefined && <Kbd>{helpKeys}</Kbd>}
            Keyboard shortcuts
          </button>
        </div>
        <p className="mt-3 font-mono text-[10px] text-outline">
          {'// Run :tour to bring this guide back.'}
        </p>
      </div>
    </aside>
  );
}
