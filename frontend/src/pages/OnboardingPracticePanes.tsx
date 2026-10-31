import type { ReactElement } from 'react';
import { keepFocus } from '../components/NavigationStylePicker';
import { cn } from '../lib/utils';
import {
  dispatchSandboxAction,
  selectOnboardingSandboxPr,
} from '../services/onboardingSandbox';
import type { SandboxState } from '../utils/onboardingSandbox';
import { DiffPane } from './OnboardingPracticeDiff';
import {
  ActionKey,
  BOT_COMMENTS,
  CommentCard,
  FAKE_CHECKS,
  FAKE_FILES,
  FAKE_PRS,
  FakeComposer,
  fileTotals,
  HUMAN_COMMENTS,
  LABEL,
  LINK_CLASS,
  PR_DESCRIPTION,
  STATUS_SQUARE,
} from './OnboardingPracticeParts';

interface PaneProps {
  readonly state: SandboxState;
}

function ListPane({ state }: PaneProps): ReactElement {
  return (
    <ul aria-label="Practice inbox">
      {FAKE_PRS.map((pr, index) => {
        const isSelected = state.cursor === index;
        return (
          <li key={pr.number}>
            <button
              type="button"
              aria-current={isSelected ? 'true' : undefined}
              onMouseDown={keepFocus}
              onClick={() =>
                isSelected
                  ? dispatchSandboxAction('list.open')
                  : selectOnboardingSandboxPr(index)
              }
              className={cn(
                'grid w-full grid-cols-[auto_4rem_1fr_auto_4rem] items-center gap-4 px-4 py-2.5 text-left border-l-2 border-transparent! hover:bg-surface-container-low',
                isSelected && 'bg-surface-container-high! border-l-primary!',
              )}
            >
              <span
                aria-hidden="true"
                className={cn('size-2.5', STATUS_SQUARE[pr.status])}
              />
              <span className="mono-text text-xs text-outline">
                {pr.number}
              </span>
              <span className="truncate text-sm font-medium text-on-surface">
                {pr.title}
              </span>
              <span className="mono-text text-xs text-outline">
                {pr.author}
              </span>
              <span
                className={cn(
                  'mono-text text-xs text-right',
                  isSelected ? 'text-primary' : 'text-outline',
                )}
              >
                {pr.age}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function BotFold({ isFolded }: { readonly isFolded: boolean }): ReactElement {
  if (!isFolded)
    return (
      <>
        {BOT_COMMENTS.map((comment) => (
          <CommentCard key={comment.author} comment={comment} meta="bot" />
        ))}
        <button
          type="button"
          onMouseDown={keepFocus}
          onClick={() => dispatchSandboxAction('detail.dismissBots')}
          className={LINK_CLASS}
        >
          Fold bot comments
          <ActionKey actionIds={['detail.dismissBots']} />
        </button>
      </>
    );
  return (
    <div className="flex items-center gap-3 border border-dashed border-outline-variant px-4 py-2.5">
      <span className={LABEL}>
        {'2 bot comments folded // coderabbit, greptile'}
      </span>
      <button
        type="button"
        onMouseDown={keepFocus}
        onClick={() => dispatchSandboxAction('detail.undoDismiss')}
        className={cn(LABEL, 'ml-auto flex items-center gap-2')}
      >
        <ActionKey
          actionIds={['detail.undoDismiss', 'detail.undoDismissAlt']}
        />
        Undo
      </button>
    </div>
  );
}

const REVIEW_BUTTONS: readonly {
  label: string;
  actionId: string;
  className: string;
}[] = [
  {
    label: 'Comment',
    actionId: 'detail.comment',
    className: 'border-outline-variant!',
  },
  {
    label: 'Request changes',
    actionId: 'detail.requestChanges',
    className: 'border-error! text-error!',
  },
  {
    label: 'Approve',
    actionId: 'detail.approve',
    className: 'bg-primary! border-primary! text-on-primary!',
  },
  {
    label: 'Open diff',
    actionId: 'detail.viewDiff',
    className: 'border-transparent!',
  },
];

function reviewerStatus(state: SandboxState): string {
  if (state.approved) return 'Approved';
  if (state.composer === 'requestChanges') return 'Changes requested';
  return 'Requested';
}

function Totals({
  added,
  deleted,
}: {
  readonly added: number;
  readonly deleted: number;
}): ReactElement {
  return (
    <span className="mono-text">
      <span className="text-green-600">+{added}</span>{' '}
      <span className="text-error">-{deleted}</span>
    </span>
  );
}

function PrRail({ state }: PaneProps): ReactElement {
  return (
    <aside className="space-y-6 border-t md:border-t-0 md:border-l border-outline-variant/30 p-4 mono-text text-xs">
      <section className="space-y-2">
        <h4 className={LABEL}>Checks</h4>
        {FAKE_CHECKS.map((check) => (
          <p key={check.name} className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className={cn('size-2', STATUS_SQUARE[check.status])}
            />
            {check.name}
            <span className="ml-auto text-outline">{check.detail}</span>
          </p>
        ))}
      </section>
      <section className="space-y-2">
        <h4 className={LABEL}>Reviewers</h4>
        <p className="flex justify-between">
          @you
          <span className={state.approved ? 'text-green-600' : 'text-primary'}>
            {reviewerStatus(state)}
          </span>
        </p>
        <p className="flex justify-between">
          @mira<span className="text-outline">Commented</span>
        </p>
      </section>
      <section className="space-y-2">
        <h4 className={LABEL}>Files ({FAKE_FILES.length})</h4>
        {FAKE_FILES.map((file, index) => (
          <p
            key={file.path}
            className={cn(
              'flex justify-between gap-2',
              index === state.fileIndex
                ? 'text-on-surface font-bold'
                : 'text-outline',
            )}
          >
            <span className="truncate">{file.path.split('/').at(-1)}</span>
            <Totals {...fileTotals(file)} />
          </p>
        ))}
      </section>
    </aside>
  );
}

const PR_TOTALS = FAKE_FILES.map(fileTotals).reduce((sum, file) => ({
  added: sum.added + file.added,
  deleted: sum.deleted + file.deleted,
}));

function PrPane({ state }: PaneProps): ReactElement {
  const pr = FAKE_PRS[state.cursor];
  return (
    <div className="grid flex-1 grid-cols-1 md:grid-cols-[minmax(0,1fr)_14rem]">
      <div className="flex flex-col gap-4 p-6 min-w-0">
        <p className="mono-text text-xs text-outline">
          {`${pr?.number ?? ''} // practice/sync-queue → main`}
        </p>
        <h3 className="flex flex-wrap items-center gap-3 text-2xl font-bold tracking-tight text-on-surface">
          {pr?.title}
          {state.approved && (
            <span className="bg-green-500 px-1.5 py-0.5 mono-text text-[10px] tracking-widest text-on-primary">
              APPROVED
            </span>
          )}
        </h3>
        <p className="text-sm text-on-surface-variant">
          <span className="mono-text">{pr?.author}</span> wants to merge 2
          commits · {pr?.age} <Totals {...PR_TOTALS} />
        </p>
        <p className="text-sm text-on-surface max-w-prose">{PR_DESCRIPTION}</p>
        <h4 className={LABEL}>Conversation</h4>
        <BotFold isFolded={state.botsFolded} />
        {HUMAN_COMMENTS.map((comment) => (
          <CommentCard key={comment.author} comment={comment} meta="1d ago" />
        ))}
        {state.composer !== null && (
          <FakeComposer
            key={state.composer}
            label={
              state.composer === 'comment'
                ? 'General comment'
                : 'Request changes'
            }
          />
        )}
        <div className="mt-auto flex flex-wrap gap-2 border-t border-outline-variant/30 pt-4">
          {REVIEW_BUTTONS.map((button) => (
            <button
              key={button.actionId}
              type="button"
              onMouseDown={keepFocus}
              onClick={() => dispatchSandboxAction(button.actionId)}
              className={cn(
                'flex items-center gap-2 border px-3 py-1.5 mono-text text-[11px] uppercase tracking-widest',
                button.className,
              )}
            >
              {button.label}
              <ActionKey actionIds={[button.actionId]} />
            </button>
          ))}
        </div>
      </div>
      <PrRail state={state} />
    </div>
  );
}

/** Fake list, PR and diff panes rendered from the sandbox reducer state. */
export function OnboardingPracticePanes({ state }: PaneProps): ReactElement {
  if (state.pane === 'list') return <ListPane state={state} />;
  if (state.pane === 'detail') return <PrPane state={state} />;
  return <DiffPane state={state} />;
}
