import type { ReactElement } from 'react';
import { Badge } from '../components/ui/badge';
import { cn } from '../lib/utils';
import {
  SANDBOX_COMMENT_COUNT,
  SANDBOX_FILE_COUNT,
  SANDBOX_PR_COUNT,
  type SandboxState,
} from '../utils/onboardingSandbox';

interface SandboxPr {
  readonly number: string;
  readonly title: string;
}

const FAKE_PRS: readonly SandboxPr[] = [
  { number: '#412', title: 'Fix race in sync queue' },
  { number: '#413', title: 'Add repository health checks' },
  { number: '#414', title: 'Keep review drafts offline' },
  { number: '#415', title: 'Show changed files in the inbox' },
  { number: '#416', title: 'Retry pending approvals after reconnect' },
];

const FAKE_COMMENTS: readonly string[] = [
  'Can queued reviews retry safely?',
  'Thanks for keeping sync idempotent.',
];

interface SandboxFile {
  readonly path: string;
  readonly lines: readonly string[];
}

const FAKE_FILES: readonly SandboxFile[] = [
  {
    path: 'go/internal/sync/queue.go',
    lines: [
      ' func (q *Queue) Sync() error {',
      '-    return q.flush()',
      '+    q.mu.Lock()',
      '+    defer q.mu.Unlock()',
      '+    return q.flush()',
      ' }',
    ],
  },
  {
    path: 'go/internal/sync/retry.go',
    lines: [
      ' func shouldRetry(status int) bool {',
      '-    return status == 500',
      '+    return status >= 500',
      ' }',
    ],
  },
];

const PANE_LABELS = {
  list: 'Pull request list',
  detail: 'Pull request detail',
  diff: 'Diff',
} satisfies Record<SandboxState['pane'], string>;

interface OnboardingSandboxProps {
  readonly state: SandboxState;
}

/** Where the fake review is, so each pane change is visible at a glance. */
function SandboxPath({
  segments,
}: {
  readonly segments: readonly string[];
}): ReactElement {
  return (
    <h3 className="flex gap-2 px-4 py-2 border-b border-outline-variant/30 text-[10px] uppercase tracking-widest text-outline">
      {segments.map((segment, index) => (
        <span
          key={segment}
          className={cn(index === segments.length - 1 && 'text-on-surface')}
        >
          {index > 0 && <span aria-hidden="true">/ </span>}
          {segment}
        </span>
      ))}
    </h3>
  );
}

function SandboxList({ state }: OnboardingSandboxProps): ReactElement {
  return (
    <>
      <SandboxPath segments={['Practice inbox']} />
      <ul>
        {FAKE_PRS.slice(0, SANDBOX_PR_COUNT).map((pr, index) => (
          <li
            key={pr.number}
            aria-current={state.cursor === index ? 'true' : undefined}
            className={cn(
              'flex gap-3 px-4 py-2.5 border-l-2 border-transparent',
              state.cursor === index &&
                'bg-surface-container-high border-l-primary',
            )}
          >
            <span className="text-outline">{pr.number}</span>
            {pr.title}
          </li>
        ))}
      </ul>
    </>
  );
}

function SandboxDetail({ state }: OnboardingSandboxProps): ReactElement {
  const pr = FAKE_PRS[state.cursor];
  return (
    <>
      <SandboxPath segments={['Practice inbox', pr?.number ?? '']} />
      <div className="p-4 space-y-4">
        <div className="flex items-center gap-3">
          <span className="font-bold">{pr?.title}</span>
          {state.approved && <Badge>Approved</Badge>}
        </div>
        <p className="text-on-surface-variant">
          Locks the queue while sync flushes and retries every 5xx.
        </p>
        <ul className="space-y-1">
          {FAKE_FILES.slice(0, SANDBOX_FILE_COUNT).map((file) => (
            <li key={file.path} className="text-on-surface-variant">
              {file.path}
            </li>
          ))}
        </ul>
      </div>
    </>
  );
}

function SandboxDiff({ state }: OnboardingSandboxProps): ReactElement | null {
  const pr = FAKE_PRS[state.cursor];
  const file = FAKE_FILES[state.fileIndex];
  if (file === undefined) return null;
  return (
    <>
      <SandboxPath segments={['Practice inbox', pr?.number ?? '', file.path]} />
      <div className="p-4 space-y-3">
        <div className="whitespace-pre-wrap break-words">
          {file.lines.map((line) => (
            <div
              key={line}
              className={cn(
                line.startsWith('+') && 'text-primary',
                line.startsWith('-') && 'text-error',
              )}
            >
              <code>{line}</code>
            </div>
          ))}
        </div>
        <ol aria-label="Inline comments" className="space-y-1">
          {FAKE_COMMENTS.slice(0, SANDBOX_COMMENT_COUNT).map(
            (comment, index) => (
              <li
                key={comment}
                aria-current={state.commentIndex === index ? 'true' : undefined}
                className={cn(
                  'flex items-center gap-2 px-3 py-1 border border-outline-variant/30 text-on-surface-variant',
                  state.commentIndex === index &&
                    'bg-surface-container-high text-on-surface border-primary',
                )}
              >
                <span
                  aria-hidden="true"
                  className="material-symbols-outlined text-sm"
                >
                  comment
                </span>
                {comment}
              </li>
            ),
          )}
        </ol>
      </div>
    </>
  );
}

/** Presentational fake review; `useAppHotkeys` drives it through appStore. */
export function OnboardingSandbox({
  state,
}: OnboardingSandboxProps): ReactElement {
  const pr = FAKE_PRS[state.cursor];
  return (
    <section
      aria-label="Practice pull request"
      className="h-72 overflow-auto bg-surface-container-lowest mono-text text-sm"
    >
      <p className="sr-only" aria-live="polite">
        {`${PANE_LABELS[state.pane]}: ${pr?.number ?? ''} ${pr?.title ?? ''}`}
      </p>
      {state.pane === 'list' && <SandboxList state={state} />}
      {state.pane === 'detail' && <SandboxDetail state={state} />}
      {state.pane === 'diff' && <SandboxDiff state={state} />}
    </section>
  );
}
