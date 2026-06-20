import {
  forwardRef,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useReviewThreads } from '../hooks/useReviewThreads';
import { useThreadActions } from '../hooks/useThreadActions';
import {
  type ReviewThread,
  type ThreadLifecycle,
  threadLifecycle,
} from '../types';

interface Props {
  prId: string;
  /**
   * Optional override for tests — lets a test assert the WS payload without a
   * live socket. In the app this is undefined and the component uses
   * {@link useThreadActions}.
   */
  sendAction?: (payload: {
    action: 'resolve_thread' | 'unresolve_thread';
    pr_id: string;
    thread_id: string;
  }) => void;
}

/** Sort weight for the lifecycle buckets — actionable work floats to the top. */
const LIFECYCLE_ORDER: Record<ThreadLifecycle, number> = {
  open: 0,
  outdated: 1,
  unknown: 2,
  resolved: 3,
};

const LIFECYCLE_LABEL: Record<ThreadLifecycle, string> = {
  open: 'Open',
  outdated: 'Outdated',
  unknown: 'Unknown',
  resolved: 'Resolved',
};

/**
 * Badge color per lifecycle. Open is the only "loud" state (amber) because it
 * is the only one demanding action; resolved is the calm accent; outdated and
 * unknown are dimmed so they recede.
 */
const LIFECYCLE_BADGE: Record<ThreadLifecycle, string> = {
  open: 'border-yellow-600/50 text-yellow-600',
  resolved: 'border-primary/50 text-primary',
  outdated: 'border-outline-variant/40 text-outline',
  unknown: 'border-outline-variant/40 text-outline-variant',
};

function sortThreads(threads: ReviewThread[]): ReviewThread[] {
  return [...threads].sort((a, b) => {
    const byLifecycle =
      LIFECYCLE_ORDER[threadLifecycle(a)] - LIFECYCLE_ORDER[threadLifecycle(b)];
    if (byLifecycle !== 0) return byLifecycle;
    const byFile = a.file_path.localeCompare(b.file_path);
    if (byFile !== 0) return byFile;
    return a.line - b.line;
  });
}

/**
 * The unresolved-threads surface (mode `'threads'`, reached via `gu`).
 *
 * Keyboard-first: `j` / `k` move the focus between threads, `r` resolves the
 * focused thread and `u` unresolves it. Those keys are registered as
 * threads-mode keybindings that dispatch DOM custom events; this component
 * listens for them so the keyboard path and the click path share one code
 * path. `Escape` (global) returns to detail.
 *
 * Honesty contract: a thread whose `state_known` is false shows an "Unknown"
 * badge and exposes no resolve/unresolve control — the backend would reject
 * the write, and the UI must not imply otherwise. The disabled reason is
 * stated inline so the reviewer understands why.
 */
export function UnresolvedThreadsView({
  prId,
  sendAction,
}: Props): React.ReactElement {
  const { threads, loading, error, refetch } = useReviewThreads(prId);
  const { resolveThread, unresolveThread, isThreadPending } =
    useThreadActions(prId);

  const sorted = useMemo(() => sortThreads(threads), [threads]);

  const [focusedIndex, setFocusedIndex] = useState(0);
  const rowRefs = useRef<Array<HTMLLIElement | null>>([]);

  // Keep focus in range as the list changes length (e.g. a thread resolves and
  // re-sorts to the bottom, or the count shrinks).
  useEffect(() => {
    setFocusedIndex((i) => Math.min(i, Math.max(0, sorted.length - 1)));
  }, [sorted.length]);

  // Move the actual DOM focus to the focused row so screen readers and the
  // browser's own scroll-into-view follow the keyboard.
  useEffect(() => {
    rowRefs.current[focusedIndex]?.focus();
  }, [focusedIndex]);

  const dispatchAction = useCallback(
    (action: 'resolve_thread' | 'unresolve_thread', thread: ReviewThread) => {
      if (!thread.state_known) return;
      if (sendAction !== undefined) {
        sendAction({ action, pr_id: prId, thread_id: thread.id });
        return;
      }
      if (action === 'resolve_thread') resolveThread(thread.id);
      else unresolveThread(thread.id);
    },
    [prId, sendAction, resolveThread, unresolveThread],
  );

  // Bridge threads-mode keybindings (which dispatch DOM custom events) into the
  // focused-row actions and movement.
  useEffect(() => {
    const focused = sorted[focusedIndex];

    const onResolve = (): void => {
      if (focused && !focused.resolved) {
        dispatchAction('resolve_thread', focused);
      }
    };
    const onUnresolve = (): void => {
      if (focused?.resolved) dispatchAction('unresolve_thread', focused);
    };
    const onNext = (): void =>
      setFocusedIndex((i) => Math.min(i + 1, sorted.length - 1));
    const onPrev = (): void => setFocusedIndex((i) => Math.max(i - 1, 0));

    document.addEventListener('bistro:threads:resolve', onResolve);
    document.addEventListener('bistro:threads:unresolve', onUnresolve);
    document.addEventListener('bistro:threads:next', onNext);
    document.addEventListener('bistro:threads:prev', onPrev);
    return () => {
      document.removeEventListener('bistro:threads:resolve', onResolve);
      document.removeEventListener('bistro:threads:unresolve', onUnresolve);
      document.removeEventListener('bistro:threads:next', onNext);
      document.removeEventListener('bistro:threads:prev', onPrev);
    };
  }, [sorted, focusedIndex, dispatchAction]);

  const openCount = sorted.filter((t) => threadLifecycle(t) === 'open').length;

  return (
    <section
      aria-label="Review threads"
      className="absolute inset-0 z-10 flex flex-col bg-surface text-on-surface"
    >
      <header className="shrink-0 flex items-center justify-between px-8 py-5 border-b border-outline-variant/30">
        <div className="flex items-baseline gap-4">
          <h2 className="font-mono text-[11px] font-bold tracking-[0.25em] uppercase">
            Review Threads
          </h2>
          <span className="font-mono text-[10px] text-outline">
            {openCount.toString().padStart(2, '0')} OPEN
          </span>
        </div>
        <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
          ESC to exit · J/K to move · R/U to resolve
        </span>
      </header>

      <div className="flex-1 min-h-0 overflow-auto" data-diff-viewport>
        {loading ? (
          <CenteredNote tone="muted">Loading review threads…</CenteredNote>
        ) : error !== null ? (
          <CenteredNote tone="error">
            <span>Couldn’t load review threads.</span>
            <button
              type="button"
              onClick={refetch}
              className="font-mono text-[10px] uppercase tracking-widest text-primary border border-primary/40 px-3 py-1 hover:bg-primary/10 transition-colors"
            >
              Retry
            </button>
          </CenteredNote>
        ) : sorted.length === 0 ? (
          <CenteredNote tone="muted">
            <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
              All clear
            </span>
            <span>No review threads on this PR.</span>
          </CenteredNote>
        ) : (
          <ul className="divide-y divide-outline-variant/20">
            {sorted.map((thread, i) => (
              <ThreadRow
                key={thread.id}
                ref={(el) => {
                  rowRefs.current[i] = el;
                }}
                thread={thread}
                focused={i === focusedIndex}
                pending={isThreadPending(thread.id)}
                onFocusRow={() => setFocusedIndex(i)}
                onResolve={() => dispatchAction('resolve_thread', thread)}
                onUnresolve={() => dispatchAction('unresolve_thread', thread)}
              />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

interface ThreadRowProps {
  thread: ReviewThread;
  focused: boolean;
  pending: boolean;
  onFocusRow: () => void;
  onResolve: () => void;
  onUnresolve: () => void;
}

const ThreadRow = forwardRef<HTMLLIElement, ThreadRowProps>(function ThreadRow(
  { thread, focused, pending, onFocusRow, onResolve, onUnresolve },
  ref,
): React.ReactElement {
  const lifecycle = threadLifecycle(thread);
  const canResolve = thread.state_known && !thread.resolved;
  const canUnresolve = thread.state_known && thread.resolved;

  return (
    <li
      ref={ref}
      tabIndex={focused ? 0 : -1}
      onFocus={onFocusRow}
      aria-label={`Thread on ${thread.file_path} line ${thread.line}, ${LIFECYCLE_LABEL[lifecycle]}`}
      className={`group relative flex items-center gap-5 px-8 py-4 outline-none transition-colors ${
        focused
          ? 'bg-surface-container-low'
          : 'hover:bg-surface-container-lowest'
      }`}
    >
      {/* Focus rail */}
      <span
        aria-hidden="true"
        className={`absolute left-0 top-0 bottom-0 w-0.5 ${
          focused ? 'bg-primary' : 'bg-transparent'
        }`}
      />

      <LifecycleBadge lifecycle={lifecycle} />

      <div className="min-w-0 flex-1">
        <div className="font-mono text-xs text-on-surface truncate">
          {thread.file_path}
          <span className="text-outline-variant">:{thread.line}</span>
        </div>
        {!thread.state_known && (
          <p className="mt-1 font-mono text-[10px] text-outline-variant">
            State unconfirmed — resolve/unresolve unavailable until the next
            sync.
          </p>
        )}
      </div>

      <div className="shrink-0 flex items-center gap-2">
        {pending ? (
          <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
            Working…
          </span>
        ) : canResolve ? (
          <RowButton onClick={onResolve}>Resolve</RowButton>
        ) : canUnresolve ? (
          <RowButton onClick={onUnresolve} variant="muted">
            Unresolve
          </RowButton>
        ) : null}
      </div>
    </li>
  );
});

function LifecycleBadge({
  lifecycle,
}: {
  lifecycle: ThreadLifecycle;
}): React.ReactElement {
  return (
    <span
      className={`shrink-0 font-mono text-[9px] font-bold uppercase tracking-widest px-2 py-1 border ${LIFECYCLE_BADGE[lifecycle]}`}
    >
      {LIFECYCLE_LABEL[lifecycle]}
    </span>
  );
}

function RowButton({
  children,
  onClick,
  variant = 'primary',
}: {
  children: React.ReactNode;
  onClick: () => void;
  variant?: 'primary' | 'muted';
}): React.ReactElement {
  const tone =
    variant === 'primary'
      ? 'border-primary/50 text-primary hover:bg-primary hover:text-on-primary'
      : 'border-outline-variant/50 text-outline hover:border-outline hover:text-on-surface';
  return (
    <button
      type="button"
      onClick={onClick}
      className={`font-mono text-[10px] uppercase tracking-widest px-3 py-1 border transition-colors ${tone}`}
    >
      {children}
    </button>
  );
}

function CenteredNote({
  children,
  tone,
}: {
  children: React.ReactNode;
  tone: 'muted' | 'error';
}): React.ReactElement {
  return (
    <div
      className={`h-full flex flex-col items-center justify-center gap-3 px-8 text-center font-mono text-xs ${
        tone === 'error' ? 'text-error' : 'text-on-surface-variant'
      }`}
    >
      {children}
    </div>
  );
}
