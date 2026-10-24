import { type ReactElement, useState } from 'react';
import { Kbd } from '../components/ui/kbd';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { cn } from '../lib/utils';
import {
  dispatchSandboxAction,
  selectOnboardingSandboxPr,
} from '../services/onboardingSandbox';
import { settingsStore } from '../store/settings';
import { findActionKeys } from '../utils/legendPresets';
import type { SandboxState } from '../utils/onboardingSandbox';

type CheckStatus = 'pass' | 'pending' | 'fail';

const STATUS_SQUARE = {
  pass: 'bg-green-500',
  pending: 'bg-yellow-500',
  fail: 'bg-error',
} satisfies Record<CheckStatus, string>;

interface SandboxPr {
  readonly number: string;
  readonly title: string;
  readonly author: string;
  readonly age: string;
  readonly status: CheckStatus;
}

export const FAKE_PRS: readonly SandboxPr[] = [
  {
    number: '#412',
    title: 'Fix race in sync queue',
    author: '@mira',
    age: '1h ago',
    status: 'pass',
  },
  {
    number: '#413',
    title: 'Add repository health checks',
    author: '@tomas',
    age: '3h ago',
    status: 'pass',
  },
  {
    number: '#414',
    title: 'Keep review drafts offline',
    author: '@ana',
    age: '5h ago',
    status: 'pending',
  },
  {
    number: '#415',
    title: 'Show changed files in the inbox',
    author: '@dependabot',
    age: '1d ago',
    status: 'fail',
  },
  {
    number: '#416',
    title: 'Retry pending approvals after reconnect',
    author: '@samuel',
    age: '2d ago',
    status: 'pass',
  },
];

interface SandboxFile {
  readonly path: string;
  /** First line number of the hunk on both sides. */
  readonly start: number;
  readonly lines: readonly string[];
  /** The hunk after only the commits pushed since the last review. */
  readonly sinceLastReview: readonly string[];
}

export const FAKE_FILES: readonly SandboxFile[] = [
  {
    path: 'go/internal/sync/queue.go',
    start: 41,
    lines: [
      ' func (q *Queue) Sync() error {',
      '-    return q.flush()',
      '+    q.mu.Lock()',
      '+    defer q.mu.Unlock()',
      '+    return q.flush()',
      ' }',
    ],
    sinceLastReview: [
      ' func (q *Queue) Sync() error {',
      '     q.mu.Lock()',
      '-    defer q.mu.Unlock()',
      '+    defer q.unlockAndNotify()',
      ' }',
    ],
  },
  {
    path: 'go/internal/sync/retry.go',
    start: 18,
    lines: [
      ' func shouldRetry(status int) bool {',
      '-    return status == 500',
      '+    return status >= 500',
      ' }',
    ],
    sinceLastReview: [
      ' func shouldRetry(status int) bool {',
      '-    return status >= 500',
      '+    return status >= 500 || status == 429',
      ' }',
    ],
  },
];

interface FakeComment {
  readonly author: string;
  readonly body: string;
}

const HUMAN_COMMENTS: readonly FakeComment[] = [
  { author: 'mira', body: 'Can queued reviews retry safely?' },
  { author: 'tomas', body: 'Thanks for keeping sync idempotent.' },
];

const BOT_COMMENTS: readonly FakeComment[] = [
  { author: 'coderabbit', body: 'Summary: 2 files changed, 1 lock added.' },
  { author: 'greptile', body: 'No issues found in go/internal/sync.' },
];

const FAKE_CHECKS: readonly { name: string; status: CheckStatus }[] = [
  { name: 'build', status: 'pass' },
  { name: 'test', status: 'pass' },
  { name: 'lint', status: 'pending' },
];

const PR_DESCRIPTION =
  'Locks the queue while sync flushes and retries every 5xx, so reconnects stop dropping queued reviews.';

const LABEL = 'mono-text text-[10px] uppercase tracking-widest text-outline';

interface PaneProps {
  readonly state: SandboxState;
}

/** Keys for a practice action, or nothing when an override unbinds it. */
function ActionKey({
  actionIds,
  className,
}: {
  readonly actionIds: readonly string[];
  readonly className?: string;
}): ReactElement | null {
  const keys = findActionKeys(useEffectiveBindings(), actionIds);
  return keys === undefined ? null : <Kbd className={className}>{keys}</Kbd>;
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

function CommentCard({
  comment,
  meta,
}: {
  readonly comment: FakeComment;
  readonly meta?: string;
}): ReactElement {
  return (
    <article className="border border-outline-variant/30 bg-surface-container-lowest">
      <header className="flex items-center gap-3 px-4 py-2 border-b border-outline-variant/30">
        <span
          aria-hidden="true"
          className="flex size-6 items-center justify-center bg-primary text-on-primary mono-text text-[10px] uppercase"
        >
          {comment.author[0]}
        </span>
        <span className="mono-text text-xs font-bold uppercase tracking-wider">
          {comment.author}
        </span>
        {meta !== undefined && (
          <span className={cn(LABEL, 'ml-auto')}>{meta}</span>
        )}
      </header>
      <p className="px-4 py-3 text-sm text-on-surface">{comment.body}</p>
    </article>
  );
}

/** Local-only draft: typing and previewing never leaves the sandbox. */
function FakeComposer({ label }: { readonly label: string }): ReactElement {
  const [body, setBody] = useState('');
  const [preview, setPreview] = useState<string | null>(null);
  return (
    <div className="space-y-2">
      <label className="block space-y-1">
        <span className={LABEL}>{label}</span>
        <textarea
          value={body}
          onChange={(event) => setBody(event.target.value)}
          rows={2}
          placeholder="Practice text stays on this screen."
          className="block w-full resize-none border border-primary bg-surface-container-lowest p-2 text-sm text-on-surface focus:outline-none"
        />
      </label>
      <button
        type="button"
        disabled={body.trim() === ''}
        onClick={() => setPreview(body.trim())}
        className="mono-text text-[10px] uppercase tracking-widest text-primary disabled:text-outline"
      >
        Preview
      </button>
      {preview !== null && (
        <CommentCard
          comment={{ author: 'you', body: preview }}
          meta="Preview // not sent"
        />
      )}
    </div>
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
          onClick={() => dispatchSandboxAction('detail.dismissBots')}
          className={cn(LABEL, 'flex items-center gap-2 hover:text-primary')}
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
            <span className="ml-auto text-outline">
              {check.status === 'pending' ? 'running' : 'passed'}
            </span>
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
              'truncate',
              index === state.fileIndex
                ? 'text-on-surface font-bold'
                : 'text-outline',
            )}
          >
            {file.path.split('/').at(-1)}
          </p>
        ))}
      </section>
    </aside>
  );
}

function PrPane({ state }: PaneProps): ReactElement {
  const pr = FAKE_PRS[state.cursor];
  return (
    <div className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_14rem]">
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
          commits · {pr?.age}{' '}
          <span className="mono-text text-green-600">+4</span>{' '}
          <span className="mono-text text-error">-2</span>
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
        <div className="mt-2 flex flex-wrap gap-2 border-t border-outline-variant/30 pt-4">
          {REVIEW_BUTTONS.map((button) => (
            <button
              key={button.actionId}
              type="button"
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

interface DiffRow {
  readonly line: string;
  readonly oldNumber: number | null;
  readonly newNumber: number | null;
}

function numberLines(start: number, lines: readonly string[]): DiffRow[] {
  let oldNumber = start;
  let newNumber = start;
  return lines.map((line) => {
    const isAdded = line.startsWith('+');
    const isDeleted = line.startsWith('-');
    const row = {
      line,
      oldNumber: isAdded ? null : oldNumber,
      newNumber: isDeleted ? null : newNumber,
    };
    if (!isAdded) oldNumber += 1;
    if (!isDeleted) newNumber += 1;
    return row;
  });
}

const rowTone = (line: string): string =>
  line.startsWith('+')
    ? 'bg-green-500/10'
    : line.startsWith('-')
      ? 'bg-error/10'
      : '';

function DiffCell({
  number,
  line,
}: {
  readonly number: number | null;
  readonly line: string | null;
}): ReactElement {
  return (
    <div
      className={cn(
        'grid grid-cols-[3rem_1fr] min-w-0',
        line !== null && rowTone(line),
      )}
    >
      <span className="pr-3 text-right text-outline select-none">
        {number ?? ''}
      </span>
      <code className="whitespace-pre overflow-hidden">{line ?? ''}</code>
    </div>
  );
}

function Hunk({ rows }: { readonly rows: readonly DiffRow[] }): ReactElement {
  const isSplit = useStore(settingsStore, (s) => s.diff.style) === 'split';
  return (
    <div className="mono-text text-xs leading-6 border-b border-outline-variant/30">
      {rows.map((row) =>
        isSplit ? (
          <div
            key={row.line}
            className="grid grid-cols-2 divide-x divide-outline-variant/30"
          >
            <DiffCell
              number={row.oldNumber}
              line={row.oldNumber === null ? null : row.line}
            />
            <DiffCell
              number={row.newNumber}
              line={row.newNumber === null ? null : row.line}
            />
          </div>
        ) : (
          <DiffCell
            key={row.line}
            number={row.newNumber ?? row.oldNumber}
            line={row.line}
          />
        ),
      )}
    </div>
  );
}

function segmentClass(isOn: boolean): string {
  return cn(
    'border px-3 py-1 mono-text text-[10px] uppercase tracking-widest',
    isOn
      ? 'bg-primary! border-primary! text-on-primary!'
      : 'border-outline-variant!',
  );
}

function DiffPane({ state }: PaneProps): ReactElement | null {
  const diffStyle = useStore(settingsStore, (s) => s.diff.style);
  const file = FAKE_FILES[state.fileIndex];
  if (file === undefined) return null;
  const toggleSince = () => dispatchSandboxAction('diff.sinceLastReview');
  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 px-4 py-3 border-b border-outline-variant/30">
        <div className="flex">
          <button
            type="button"
            aria-pressed={!state.sinceLastReview}
            onClick={() => state.sinceLastReview && toggleSince()}
            className={segmentClass(!state.sinceLastReview)}
          >
            All changes
          </button>
          <button
            type="button"
            aria-pressed={state.sinceLastReview}
            onClick={() => !state.sinceLastReview && toggleSince()}
            className={cn(
              segmentClass(state.sinceLastReview),
              'flex items-center gap-2',
            )}
          >
            Since last review
            <ActionKey actionIds={['diff.sinceLastReview']} />
          </button>
        </div>
        <span className={LABEL}>{diffStyle}</span>
        <span className={cn(LABEL, 'ml-auto')}>
          {`File ${state.fileIndex + 1} / ${FAKE_FILES.length} // ${file.path}`}
        </span>
        <button
          type="button"
          onClick={() => dispatchSandboxAction('diff.nextFile')}
          className={cn(LABEL, 'flex items-center gap-2 hover:text-primary')}
        >
          Next file
          <ActionKey actionIds={['diff.nextFile']} />
        </button>
      </div>
      {state.sinceLastReview && (
        <p className="px-4 py-2 text-xs text-on-surface-variant border-b border-outline-variant/30">
          Only commits pushed after your last pass.
        </p>
      )}
      <Hunk
        rows={numberLines(
          file.start,
          state.sinceLastReview ? file.sinceLastReview : file.lines,
        )}
      />
      <div className="space-y-3 p-4">
        <div className="flex items-center gap-3">
          <h4 className={LABEL}>
            Thread {state.commentIndex + 1} / {HUMAN_COMMENTS.length}
          </h4>
          <button
            type="button"
            onClick={() => dispatchSandboxAction('diff.nextComment')}
            className={cn(LABEL, 'flex items-center gap-2 hover:text-primary')}
          >
            Next comment
            <ActionKey actionIds={['diff.nextComment']} />
          </button>
        </div>
        {HUMAN_COMMENTS.map((comment, index) => (
          <div
            key={comment.author}
            aria-current={state.commentIndex === index ? 'true' : undefined}
            className={cn(
              'border-l-2',
              state.commentIndex === index
                ? 'border-primary'
                : 'border-transparent opacity-60',
            )}
          >
            <CommentCard comment={comment} meta="1d ago" />
          </div>
        ))}
        {state.inlineReplyOpen ? (
          <FakeComposer label="Reply to the thread" />
        ) : (
          <button
            type="button"
            onClick={() => dispatchSandboxAction('detail.inlineReply')}
            className={cn(LABEL, 'flex items-center gap-2 hover:text-primary')}
          >
            <ActionKey actionIds={['detail.inlineReply']} />
            Reply
          </button>
        )}
      </div>
    </div>
  );
}

/** Fake list, PR and diff panes rendered from the sandbox reducer state. */
export function OnboardingPracticePanes({ state }: PaneProps): ReactElement {
  if (state.pane === 'list') return <ListPane state={state} />;
  if (state.pane === 'detail') return <PrPane state={state} />;
  return <DiffPane state={state} />;
}
