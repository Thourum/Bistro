import type { ReactElement } from 'react';
import { keepFocus } from '../components/NavigationStylePicker';
import { useStore } from '../hooks/useStore';
import { cn } from '../lib/utils';
import { dispatchSandboxAction } from '../services/onboardingSandbox';
import { settingsStore } from '../store/settings';
import type { SandboxState } from '../utils/onboardingSandbox';
import {
  ActionKey,
  CommentCard,
  FAKE_FILES,
  FakeComposer,
  HUMAN_COMMENTS,
  LABEL,
  LINK_CLASS,
} from './OnboardingPracticeParts';

interface DiffRow {
  readonly id: number;
  readonly line: string;
  readonly oldNumber: number | null;
  readonly newNumber: number | null;
}

interface SplitRow {
  readonly id: number;
  readonly left: DiffRow | null;
  readonly right: DiffRow | null;
}

const isAdded = (line: string): boolean => line.startsWith('+');
const isDeleted = (line: string): boolean => line.startsWith('-');

function numberLines(start: number, lines: readonly string[]): DiffRow[] {
  let oldNumber = start;
  let newNumber = start;
  return lines.map((line, id) => {
    const row = {
      id,
      line,
      oldNumber: isAdded(line) ? null : oldNumber,
      newNumber: isDeleted(line) ? null : newNumber,
    };
    if (!isAdded(line)) oldNumber += 1;
    if (!isDeleted(line)) newNumber += 1;
    return row;
  });
}

/** Pairs each run of deletions with the additions that follow it. */
function pairRows(rows: readonly DiffRow[]): SplitRow[] {
  const paired: SplitRow[] = [];
  let deleted: DiffRow[] = [];
  let added: DiffRow[] = [];
  const flush = (): void => {
    for (let k = 0; k < Math.max(deleted.length, added.length); k += 1)
      paired.push({
        id: paired.length,
        left: deleted[k] ?? null,
        right: added[k] ?? null,
      });
    deleted = [];
    added = [];
  };
  for (const row of rows) {
    if (isDeleted(row.line)) {
      if (added.length > 0) flush();
      deleted.push(row);
    } else if (isAdded(row.line)) {
      added.push(row);
    } else {
      flush();
      paired.push({ id: paired.length, left: row, right: row });
    }
  }
  flush();
  return paired;
}

const rowTone = (line: string): string =>
  isAdded(line) ? 'bg-green-500/10' : isDeleted(line) ? 'bg-error/10' : '';

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

function Hunk({
  rows,
  isSplit,
}: {
  readonly rows: readonly DiffRow[];
  readonly isSplit: boolean;
}): ReactElement {
  return (
    <div className="mono-text text-xs leading-6 border-b border-outline-variant/30">
      {isSplit
        ? pairRows(rows).map((row) => (
            <div
              key={row.id}
              className="grid grid-cols-2 divide-x divide-outline-variant/30"
            >
              <DiffCell
                number={row.left?.oldNumber ?? null}
                line={row.left?.line ?? null}
              />
              <DiffCell
                number={row.right?.newNumber ?? null}
                line={row.right?.line ?? null}
              />
            </div>
          ))
        : rows.map((row) => (
            <DiffCell
              key={row.id}
              number={row.newNumber ?? row.oldNumber}
              line={row.line}
            />
          ))}
    </div>
  );
}

function segmentClass(isOn: boolean): string {
  return cn(
    'border px-3 py-1 mono-text text-[10px] uppercase tracking-widest',
    isOn
      ? 'bg-primary! border-primary! text-on-primary!'
      : 'border-outline-variant! text-on-surface-variant',
  );
}

/** Static: the real diff's style setting decides, there is no toggle here. */
function StyleIndicator({
  isSplit,
}: {
  readonly isSplit: boolean;
}): ReactElement {
  return (
    <span className="flex">
      <span aria-hidden="true" className={segmentClass(!isSplit)}>
        Unified
      </span>
      <span aria-hidden="true" className={segmentClass(isSplit)}>
        Split
      </span>
      <span className="sr-only">{isSplit ? 'Split view' : 'Unified view'}</span>
    </span>
  );
}

function DiffToolbar({
  state,
}: {
  readonly state: SandboxState;
}): ReactElement {
  const isSplit = useStore(settingsStore, (s) => s.diff.style) === 'split';
  const file = FAKE_FILES[state.fileIndex];
  const toggleSince = () => dispatchSandboxAction('diff.sinceLastReview');
  return (
    <div className="flex flex-wrap items-center gap-3 px-4 py-3 border-b border-outline-variant/30">
      <div className="flex">
        <button
          type="button"
          aria-pressed={!state.sinceLastReview}
          onMouseDown={keepFocus}
          onClick={() => state.sinceLastReview && toggleSince()}
          className={segmentClass(!state.sinceLastReview)}
        >
          All changes
        </button>
        <button
          type="button"
          aria-pressed={state.sinceLastReview}
          onMouseDown={keepFocus}
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
      <StyleIndicator isSplit={isSplit} />
      <span className={cn(LABEL, 'ml-auto')}>
        {`File ${state.fileIndex + 1} / ${FAKE_FILES.length} // ${file?.path ?? ''}`}
      </span>
      <button
        type="button"
        onMouseDown={keepFocus}
        onClick={() => dispatchSandboxAction('diff.nextFile')}
        className={LINK_CLASS}
      >
        Next file
        <ActionKey actionIds={['diff.nextFile']} />
      </button>
    </div>
  );
}

function Threads({ state }: { readonly state: SandboxState }): ReactElement {
  return (
    <div className="space-y-3 p-4">
      <div className="flex items-center gap-3">
        <h4 className={LABEL}>
          Thread {state.commentIndex + 1} / {HUMAN_COMMENTS.length}
        </h4>
        <button
          type="button"
          onMouseDown={keepFocus}
          onClick={() => dispatchSandboxAction('diff.nextComment')}
          className={LINK_CLASS}
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
          onMouseDown={keepFocus}
          onClick={() => dispatchSandboxAction('detail.inlineReply')}
          className={LINK_CLASS}
        >
          <ActionKey actionIds={['detail.inlineReply']} />
          Reply
        </button>
      )}
    </div>
  );
}

/** Fake diff: file toolbar, one hunk and its inline threads. */
export function DiffPane({
  state,
}: {
  readonly state: SandboxState;
}): ReactElement | null {
  const isSplit = useStore(settingsStore, (s) => s.diff.style) === 'split';
  const file = FAKE_FILES[state.fileIndex];
  if (file === undefined) return null;
  return (
    <div>
      <DiffToolbar state={state} />
      {state.sinceLastReview && (
        <p className="px-4 py-2 text-xs text-on-surface-variant border-b border-outline-variant/30">
          Only commits pushed after your last pass.
        </p>
      )}
      <Hunk
        isSplit={isSplit}
        rows={numberLines(
          file.start,
          state.sinceLastReview ? file.sinceLastReview : file.lines,
        )}
      />
      <Threads state={state} />
    </div>
  );
}
