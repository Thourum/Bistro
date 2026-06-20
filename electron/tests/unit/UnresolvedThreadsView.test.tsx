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
