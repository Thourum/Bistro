import { type ReactElement, useEffect, useRef, useState } from 'react';
import { keepFocus } from '../components/NavigationStylePicker';
import { Kbd } from '../components/ui/kbd';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { cn } from '../lib/utils';
import { findActionKeys } from '../utils/legendPresets';

export type CheckStatus = 'pass' | 'pending' | 'fail';

export const STATUS_SQUARE = {
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

export interface SandboxFile {
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

export interface FakeComment {
  readonly author: string;
  readonly body: string;
}

export const HUMAN_COMMENTS: readonly FakeComment[] = [
  { author: 'mira', body: 'Can queued reviews retry safely?' },
  { author: 'tomas', body: 'Thanks for keeping sync idempotent.' },
];

export const BOT_COMMENTS: readonly FakeComment[] = [
  { author: 'coderabbit', body: 'Summary: 2 files changed, 1 lock added.' },
  { author: 'greptile', body: 'No issues found in go/internal/sync.' },
];

/** `detail` is the check's duration, or what it is doing while unfinished. */
export const FAKE_CHECKS: readonly {
  name: string;
  status: CheckStatus;
  detail: string;
}[] = [
  { name: 'build', status: 'pass', detail: '1m 12s' },
  { name: 'test', status: 'pass', detail: '3m 40s' },
  { name: 'lint', status: 'pending', detail: 'running' },
];

export const PR_DESCRIPTION =
  'Locks the queue while sync flushes and retries every 5xx, so reconnects stop dropping queued reviews.';

export const LABEL =
  'mono-text text-[10px] uppercase tracking-widest text-outline';
export const LINK_CLASS = cn(
  LABEL,
  'flex items-center gap-2 hover:text-primary',
);

/** Added and deleted line counts of a fake file's full hunk. */
export function fileTotals(file: SandboxFile): {
  added: number;
  deleted: number;
} {
  return {
    added: file.lines.filter((line) => line.startsWith('+')).length,
    deleted: file.lines.filter((line) => line.startsWith('-')).length,
  };
}

/** Keys for a practice action, or nothing when an override unbinds it. */
export function ActionKey({
  actionIds,
  className,
}: {
  readonly actionIds: readonly string[];
  readonly className?: string;
}): ReactElement | null {
  const keys = findActionKeys(useEffectiveBindings(), actionIds);
  return keys === undefined ? null : (
    <Kbd className={cn('shrink-0 whitespace-nowrap', className)}>{keys}</Kbd>
  );
}

export function CommentCard({
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
export function FakeComposer({
  label,
}: {
  readonly label: string;
}): ReactElement {
  const [body, setBody] = useState('');
  const [preview, setPreview] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  // Mounts only when its key opens it, so the draft is ready to type into.
  useEffect(() => textareaRef.current?.focus(), []);
  return (
    <div className="space-y-2">
      <label className="block space-y-1">
        <span className={LABEL}>{label}</span>
        <textarea
          ref={textareaRef}
          value={body}
          onChange={(event) => setBody(event.target.value)}
          rows={2}
          data-onboarding-sandbox-composer
          placeholder="Practice text stays on this screen."
          className="block w-full resize-none border border-primary bg-surface-container-lowest p-2 text-sm text-on-surface focus:outline-none"
        />
      </label>
      <button
        type="button"
        disabled={body.trim() === ''}
        onMouseDown={keepFocus}
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
