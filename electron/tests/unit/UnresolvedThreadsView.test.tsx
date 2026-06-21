// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { UnresolvedThreadsView } from '../../src/renderer/components/UnresolvedThreadsView';
import type { ReviewThread } from '../../src/renderer/types';

expect.extend(matchers);

vi.mock('../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: vi.fn(),
}));
vi.mock('../../src/renderer/hooks/useThreadActions', () => ({
  useThreadActions: () => ({
    resolveThread: vi.fn(),
    unresolveThread: vi.fn(),
    isThreadPending: () => false,
  }),
}));

import { useReviewThreads } from '../../src/renderer/hooks/useReviewThreads';

const mockThreads = vi.mocked(useReviewThreads);

function thread(over: Partial<ReviewThread>): ReviewThread {
  return {
    id: 'T1',
    pr_id: 'pr1',
    file_path: 'src/foo.ts',
    line: 42,
    resolved: false,
    outdated: false,
    state_known: true,
    ...over,
  };
}

function state(threads: ReviewThread[], over = {}) {
  return {
    threads,
    loading: false,
    error: null,
    refetch: vi.fn(),
    ...over,
  };
}

describe('UnresolvedThreadsView', () => {
  it('renders an Open badge and resolve control for an open thread', () => {
    mockThreads.mockReturnValue(state([thread({})]));
    render(<UnresolvedThreadsView prId="pr1" />);
    expect(screen.getByText('Open')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /resolve/i }),
    ).toBeInTheDocument();
  });

  it('renders a Resolved badge and an unresolve control for a resolved thread', () => {
    mockThreads.mockReturnValue(state([thread({ id: 'T2', resolved: true })]));
    render(<UnresolvedThreadsView prId="pr1" />);
    expect(screen.getByText('Resolved')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /unresolve/i }),
    ).toBeInTheDocument();
  });

  it('shows Unknown and no action control when state_known is false', () => {
    mockThreads.mockReturnValue(
      state([thread({ id: 'T3', state_known: false })]),
    );
    render(<UnresolvedThreadsView prId="pr1" />);
    expect(screen.getByText('Unknown')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /resolve/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/state unconfirmed/i)).toBeInTheDocument();
  });

  it('dispatches a resolve_thread payload on click via the sendAction prop', () => {
    const sendAction = vi.fn();
    mockThreads.mockReturnValue(state([thread({})]));
    render(<UnresolvedThreadsView prId="pr1" sendAction={sendAction} />);
    fireEvent.click(screen.getByRole('button', { name: /resolve/i }));
    expect(sendAction).toHaveBeenCalledWith({
      action: 'resolve_thread',
      pr_id: 'pr1',
      thread_id: 'T1',
    });
  });

  it('dispatches an unresolve_thread payload for a resolved thread', () => {
    const sendAction = vi.fn();
    mockThreads.mockReturnValue(state([thread({ id: 'T2', resolved: true })]));
    render(<UnresolvedThreadsView prId="pr1" sendAction={sendAction} />);
    fireEvent.click(screen.getByRole('button', { name: /unresolve/i }));
    expect(sendAction).toHaveBeenCalledWith({
      action: 'unresolve_thread',
      pr_id: 'pr1',
      thread_id: 'T2',
    });
  });

  it('renders an empty state when there are no threads', () => {
    mockThreads.mockReturnValue(state([]));
    render(<UnresolvedThreadsView prId="pr1" />);
    expect(screen.getByText(/no review threads/i)).toBeInTheDocument();
  });

  it('renders a loading note while fetching', () => {
    mockThreads.mockReturnValue(state([], { loading: true }));
    render(<UnresolvedThreadsView prId="pr1" />);
    expect(screen.getByText(/loading review threads/i)).toBeInTheDocument();
  });

  it('renders an error note with a retry control', () => {
    mockThreads.mockReturnValue(state([], { error: 'boom' }));
    render(<UnresolvedThreadsView prId="pr1" />);
    expect(
      screen.getByText(/couldn’t load review threads/i),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
  });
});

function comment(over: Record<string, unknown> = {}) {
  return {
    id: 'C1',
    pr_id: 'pr1',
    author: 'alice',
    body: 'Please fix this nil check.',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    line: 42,
    file: 'src/foo.ts',
    thread_id: 'T1',
    ...over,
  };
}

describe('UnresolvedThreadsView — inline comments', () => {
  it('renders the comment body and author under the matching thread', () => {
    mockThreads.mockReturnValue(state([thread({ id: 'T1' })]));
    render(
      <UnresolvedThreadsView
        prId="pr1"
        comments={[comment({ thread_id: 'T1' })]}
      />,
    );
    expect(screen.getByText('Please fix this nil check.')).toBeInTheDocument();
    expect(screen.getByText('@alice')).toBeInTheDocument();
  });

  it('joins comments to threads by thread_id', () => {
    mockThreads.mockReturnValue(
      state([
        thread({ id: 'T1' }),
        thread({ id: 'T2', file_path: 'src/bar.ts' }),
      ]),
    );
    render(
      <UnresolvedThreadsView
        prId="pr1"
        comments={[
          comment({ id: 'C1', thread_id: 'T1', body: 'belongs to T1' }),
          comment({ id: 'C2', thread_id: 'T2', body: 'belongs to T2' }),
        ]}
      />,
    );
    expect(screen.getByText('belongs to T1')).toBeInTheDocument();
    expect(screen.getByText('belongs to T2')).toBeInTheDocument();
  });

  it('shows a no-text fallback when a thread has no comments', () => {
    mockThreads.mockReturnValue(state([thread({ id: 'T1' })]));
    render(<UnresolvedThreadsView prId="pr1" comments={[]} />);
    expect(screen.getByText(/no comment text available/i)).toBeInTheDocument();
  });

  it('ignores comments whose thread_id does not match any thread', () => {
    mockThreads.mockReturnValue(state([thread({ id: 'T1' })]));
    render(
      <UnresolvedThreadsView
        prId="pr1"
        comments={[comment({ id: 'C9', thread_id: 'T-other', body: 'orphan' })]}
      />,
    );
    expect(screen.queryByText('orphan')).not.toBeInTheDocument();
    expect(screen.getByText(/no comment text available/i)).toBeInTheDocument();
  });

  it('renders diff_hunk in a pre block when present', () => {
    const hunk = '@@ -10,6 +10,8 @@ func Foo() {\n-\treturn nil\n+\treturn err';
    mockThreads.mockReturnValue(state([thread({ id: 'T1' })]));
    render(
      <UnresolvedThreadsView
        prId="pr1"
        comments={[comment({ thread_id: 'T1', diff_hunk: hunk })]}
      />,
    );
    const pre = screen.getByText(
      (_, el) => el?.tagName === 'PRE' && el.textContent === hunk,
    );
    expect(pre).toBeInTheDocument();
  });

  it('does not render a diff_hunk block when diff_hunk is absent', () => {
    mockThreads.mockReturnValue(state([thread({ id: 'T1' })]));
    render(
      <UnresolvedThreadsView
        prId="pr1"
        comments={[comment({ thread_id: 'T1' })]}
      />,
    );
    // No <pre> elements should exist for this comment (no diff_hunk).
    const pres = document.querySelectorAll('pre');
    expect(pres.length).toBe(0);
  });
});
