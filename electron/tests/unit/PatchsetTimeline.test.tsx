// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PatchsetTimeline } from '../../src/renderer/components/PatchsetTimeline';
import type { Patchset } from '../../src/renderer/types';

expect.extend(matchers);

function ps(n: number): Patchset {
  return {
    pr_id: 'pr-1',
    number: n,
    head_sha: String(n).repeat(40).slice(0, 40),
    base_sha: 'b'.repeat(40),
    pushed_at: `2026-01-0${n}T00:00:00Z`,
  };
}

const three = [ps(1), ps(2), ps(3)];

describe('PatchsetTimeline', () => {
  it('renders a chip for each observed patchset', () => {
    render(<PatchsetTimeline patchsets={three} onSelectPair={vi.fn()} />);
    // P1 appears in both the TO rail and the FROM rail; P3 in the TO rail.
    expect(screen.getAllByText('P1').length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText('P3').length).toBeGreaterThanOrEqual(1);
  });

  it('uses honest snapshot wording, never "every push"', () => {
    render(<PatchsetTimeline patchsets={three} onSelectPair={vi.fn()} />);
    expect(screen.getByText(/snapshots observed/i)).toBeInTheDocument();
  });

  it('fires the default (P_{n-1}, P_n) pair on mount', () => {
    const spy = vi.fn();
    render(<PatchsetTimeline patchsets={three} onSelectPair={spy} />);
    expect(spy).toHaveBeenCalledWith(three[1], three[2]);
  });

  it('selecting a later TO chip emits the new pair', () => {
    const spy = vi.fn();
    render(<PatchsetTimeline patchsets={three} onSelectPair={spy} />);
    spy.mockClear();
    // Choose P2 as the TO end via its accessible label.
    fireEvent.click(screen.getByLabelText(/Compare up to patchset 2/i));
    const last = spy.mock.calls[spy.mock.calls.length - 1];
    expect(last[1].number).toBe(2);
  });

  it('renders an empty state with no patchsets and never crashes', () => {
    render(<PatchsetTimeline patchsets={[]} onSelectPair={vi.fn()} />);
    expect(screen.getByText(/no snapshots yet/i)).toBeInTheDocument();
  });

  it('hides the FROM rail when only one snapshot exists', () => {
    render(<PatchsetTimeline patchsets={[ps(1)]} onSelectPair={vi.fn()} />);
    expect(screen.queryByText(/^From$/i)).toBeNull();
  });
});
