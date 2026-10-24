import { describe, expect, it } from 'vitest';
import { splitUnifiedDiff } from '../components/diff/diffPathUtils';
import type { TriageResult } from '../types';
import { filterDiffToPaths, triageSummary } from './sinceLastReview';

const NOW = new Date('2026-10-23T12:00:00Z');

function triage(overrides: Partial<TriageResult>): TriageResult {
  return {
    pr_id: 'github:o/r:1',
    state: 'ok',
    reason: '',
    from_patchset: 3,
    to_patchset: 5,
    baseline: null,
    files: [],
    changed_count: 4,
    total_count: 37,
    ...overrides,
  };
}

describe('triageSummary', () => {
  it('names the review baseline and its age', () => {
    const result = triage({
      baseline: {
        source: 'github_review',
        reviewed_at: '2026-10-21T09:00:00Z',
      },
    });
    expect(triageSummary(result, NOW)).toBe(
      'since P3 (your review, 2d ago): 4 of 37 files changed',
    );
  });

  it('names an all-viewed baseline', () => {
    const result = triage({
      baseline: { source: 'all_viewed', reviewed_at: '2026-10-23T09:00:00Z' },
    });
    expect(triageSummary(result, NOW)).toBe(
      'since P3 (all viewed, 3h ago): 4 of 37 files changed',
    );
  });

  it('labels an explicit pair', () => {
    expect(triageSummary(triage({}), NOW)).toBe(
      'P3 → P5: 4 of 37 files changed',
    );
  });

  it('shows the engine reason when triage is unavailable', () => {
    const result = triage({
      state: 'no_baseline',
      reason: 'no prior review recorded',
    });
    expect(triageSummary(result, NOW)).toBe('no prior review recorded');
  });
});

describe('filterDiffToPaths', () => {
  it('keeps only the listed files', () => {
    const diff = [
      'diff --git a/a.go b/a.go',
      '--- a/a.go',
      '+++ b/a.go',
      '@@ -1 +1 @@',
      '-x',
      '+y',
      'diff --git a/b.go b/b.go',
      '--- a/b.go',
      '+++ b/b.go',
      '@@ -1 +1 @@',
      '-p',
      '+q',
      '',
    ].join('\n');

    const filtered = filterDiffToPaths(diff, new Set(['b.go']));

    expect(splitUnifiedDiff(filtered).map((file) => file.path)).toEqual([
      'b.go',
    ]);
  });
});
