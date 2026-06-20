// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MergeReadinessBadge } from '../../src/renderer/components/MergeReadinessBadge';
import type { ReviewThread } from '../../src/renderer/types';

expect.extend(matchers);

vi.mock('../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: vi.fn(),
}));

import { useReviewThreads } from '../../src/renderer/hooks/useReviewThreads';

const mockThreads = vi.mocked(useReviewThreads);

function thread(over: Partial<ReviewThread>): ReviewThread {
  return {
    id: 'T1',
    pr_id: 'pr1',
    file_path: 'src/foo.ts',
    line: 1,
    resolved: false,
    outdated: false,
    state_known: true,
    ...over,
  };
}

function state(threads: ReviewThread[], over = {}) {
  return { threads, loading: false, error: null, refetch: vi.fn(), ...over };
}

describe('MergeReadinessBadge', () => {
  it('shows ready when no open threads remain', () => {
    mockThreads.mockReturnValue(state([thread({ resolved: true })]));
    render(<MergeReadinessBadge prId="pr1" />);
    expect(screen.getByText(/ready to merge/i)).toBeInTheDocument();
  });

  it('shows a singular open-thread count', () => {
    mockThreads.mockReturnValue(state([thread({})]));
    render(<MergeReadinessBadge prId="pr1" />);
    expect(screen.getByText('1 open thread')).toBeInTheDocument();
  });

  it('shows a plural open-thread count', () => {
    mockThreads.mockReturnValue(
      state([thread({ id: 'T1' }), thread({ id: 'T2' })]),
    );
    render(<MergeReadinessBadge prId="pr1" />);
    expect(screen.getByText('2 open threads')).toBeInTheDocument();
  });

  it('does not count outdated or resolved threads as blocking', () => {
    mockThreads.mockReturnValue(
      state([
        thread({ id: 'T1', resolved: true }),
        thread({ id: 'T2', outdated: true }),
      ]),
    );
    render(<MergeReadinessBadge prId="pr1" />);
    expect(screen.getByText(/ready to merge/i)).toBeInTheDocument();
  });

  it('surfaces unknown-state threads separately without blocking merge', () => {
    mockThreads.mockReturnValue(
      state([thread({ id: 'T1', state_known: false })]),
    );
    render(<MergeReadinessBadge prId="pr1" />);
    expect(screen.getByText(/ready to merge/i)).toBeInTheDocument();
    expect(screen.getByText(/1 unknown/i)).toBeInTheDocument();
  });

  it('renders nothing while loading', () => {
    mockThreads.mockReturnValue(state([], { loading: true }));
    const { container } = render(<MergeReadinessBadge prId="pr1" />);
    expect(container.firstChild).toBeNull();
  });
});
