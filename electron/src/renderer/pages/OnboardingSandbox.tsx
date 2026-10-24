import { type ReactElement, useMemo, useState } from 'react';
import {
  keepFocus,
  NAVIGATION_STYLE_CARDS,
} from '../components/NavigationStylePicker';
import { Kbd } from '../components/ui/kbd';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { cn } from '../lib/utils';
import {
  dispatchSandboxAction,
  selectOnboardingSandboxPane,
} from '../services/onboardingSandbox';
import { settingsStore } from '../store/settings';
import { findActionKeys } from '../utils/legendPresets';
import {
  SANDBOX_TASKS,
  type SandboxPane,
  type SandboxState,
  type SandboxTaskId,
  sandboxTaskDisplayIds,
} from '../utils/onboardingSandbox';
import { OnboardingPracticePanes } from './OnboardingPracticePanes';
import { FAKE_FILES, FAKE_PRS } from './OnboardingPracticeParts';

const PANE_TABS = [
  { pane: 'list', label: 'List' },
  { pane: 'detail', label: 'PR' },
  { pane: 'diff', label: 'Diff' },
] as const satisfies readonly { pane: SandboxPane; label: string }[];

/** Movement bar pairs; each half is the matching Task-5 practice task. */
const MOVE_PAIRS: readonly {
  label: string;
  tasks: readonly [SandboxTaskId, SandboxTaskId];
}[] = [
  { label: 'Down / Up', tasks: ['moveDown', 'moveUp'] },
  { label: 'Top / Bottom', tasks: ['top', 'bottom'] },
  { label: 'Open / Back', tasks: ['open', 'back'] },
];

const LABEL = 'mono-text text-[10px] uppercase tracking-widest';

interface OnboardingSandboxProps {
  readonly state: SandboxState;
}

function Breadcrumb({ state }: OnboardingSandboxProps): ReactElement {
  const pr = FAKE_PRS[state.cursor];
  const segments = [
    'Practice inbox',
    ...(state.pane === 'list' ? [] : [pr?.number ?? '']),
    ...(state.pane === 'diff' ? [FAKE_FILES[state.fileIndex]?.path ?? ''] : []),
  ];
  return (
    <p className="flex flex-wrap gap-2 px-4 py-2.5 bg-surface-container-low border-b border-outline-variant/30 mono-text text-xs text-on-surface">
      <span className={cn(LABEL, 'text-primary')}>Practice</span>
      <span aria-hidden="true" className="text-outline">
        {'//'}
      </span>
      {segments.map((segment, index) => (
        <span key={segment}>
          {index > 0 && (
            <span aria-hidden="true" className="text-outline">
              /{' '}
            </span>
          )}
          {segment}
        </span>
      ))}
    </p>
  );
}

function PaneTabs({ state }: OnboardingSandboxProps): ReactElement {
  return (
    <div
      role="tablist"
      aria-label="Practice pane"
      className="flex px-2 border-b border-outline-variant/30"
    >
      {PANE_TABS.map((tab) => {
        const isSelected = state.pane === tab.pane;
        return (
          <button
            key={tab.pane}
            type="button"
            role="tab"
            aria-selected={isSelected}
            onMouseDown={keepFocus}
            onClick={() => selectOnboardingSandboxPane(tab.pane)}
            className={cn(
              LABEL,
              'px-4 py-2 border-0! border-b-2! border-transparent!',
              isSelected ? 'text-on-surface border-primary!' : 'text-outline',
            )}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}

function MovementBar({ state }: OnboardingSandboxProps): ReactElement {
  const bindings = useEffectiveBindings();
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const tasks = useMemo(
    () =>
      SANDBOX_TASKS.map((task) => ({
        task,
        keys: findActionKeys(bindings, sandboxTaskDisplayIds(task, style)),
      })),
    [bindings, style],
  );
  const next = SANDBOX_TASKS.find((task) => !state.completed.includes(task.id));
  const keysFor = (id: SandboxTaskId) =>
    tasks.find((entry) => entry.task.id === id);
  const [isReferenceOpen, setIsReferenceOpen] = useState(false);

  return (
    <footer className="border-t border-outline-variant/30 bg-surface-container-low">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-2.5">
        <span className={cn(LABEL, 'font-bold text-on-surface')}>
          {`Move // ${NAVIGATION_STYLE_CARDS[style].label}`}
        </span>
        {MOVE_PAIRS.map((pair) => (
          <span key={pair.label} className="flex items-center gap-1.5">
            {pair.tasks.map((id) => {
              const entry = keysFor(id);
              // Clickable even when an override leaves the task unbound.
              return (
                <button
                  key={id}
                  type="button"
                  aria-label={entry?.task.label}
                  onMouseDown={keepFocus}
                  onClick={() => {
                    const actionId = entry?.task.actionIds[0];
                    if (actionId !== undefined) dispatchSandboxAction(actionId);
                  }}
                  className="min-w-6 min-h-5"
                >
                  {entry?.keys !== undefined ? (
                    <Kbd>{entry.keys}</Kbd>
                  ) : (
                    <span className={cn(LABEL, 'text-primary')}>
                      {entry?.task.label}
                    </span>
                  )}
                </button>
              );
            })}
            <span className={cn(LABEL, 'text-outline')}>{pair.label}</span>
          </span>
        ))}
        <button
          type="button"
          aria-expanded={isReferenceOpen}
          aria-controls="practice-all-keys"
          onMouseDown={keepFocus}
          onClick={() => setIsReferenceOpen((open) => !open)}
          className={cn(LABEL, 'text-outline hover:text-primary')}
        >
          All keys
        </button>
        <span className={cn(LABEL, 'ml-auto text-on-surface-variant')}>
          {next === undefined ? 'Practice complete' : `Next: ${next.label}`}
        </span>
      </div>
      {isReferenceOpen && (
        <dl
          id="practice-all-keys"
          className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 px-4 pb-3 text-xs"
        >
          {tasks.map(({ task, keys }) => (
            <div key={task.id} className="flex justify-between gap-3">
              <dt className="text-on-surface-variant">{task.label}</dt>
              <dd>{keys === undefined ? null : <Kbd>{keys}</Kbd>}</dd>
            </div>
          ))}
        </dl>
      )}
    </footer>
  );
}

/** Practice composition: breadcrumb, pane tabs, fake pane, movement bar. */
export function OnboardingSandbox({
  state,
}: OnboardingSandboxProps): ReactElement {
  return (
    <section
      aria-label="Practice pull request"
      className="flex flex-col min-h-[32rem] border border-outline-variant/30 bg-surface-container-lowest"
    >
      <Breadcrumb state={state} />
      <PaneTabs state={state} />
      <div className="flex flex-1 flex-col min-h-0 overflow-auto">
        <OnboardingPracticePanes state={state} />
      </div>
      <MovementBar state={state} />
    </section>
  );
}
