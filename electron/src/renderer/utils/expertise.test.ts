import { describe, expect, it } from 'vitest';
import type { ExpertiseExpert, PR } from '../types';
import {
  expertisePayloadForPR,
  focusedExpertisePR,
  formatWhy,
  isExpertisePayloadForPR,
  touchesYouCount,
} from './expertise';

const makePR = (overrides: Partial<PR>): PR => ({
  id: overrides.id ?? 'pr-1',
  provider: 'github',
  owner: 'acme',
  repo: 'api',
  number: 1,
  title: 'Test PR',
  body: null,
  author: 'alice',
  state: 'open',
  created_at: '2026-07-15T00:00:00Z',
  updated_at: '2026-07-15T00:00:00Z',
  merged_at: null,
  base_branch: 'main',
  head_branch: 'feature',
  base_sha: 'base',
  head_sha: 'head',
  comment_count: 0,
  ci_status: null,
  draft: 0,
  review_status: null,
  approved_by: null,
  changes_requested_by: null,
  requested_reviewers: [],
  touches_you: overrides.touches_you ?? false,
  mentioned: overrides.mentioned ?? false,
  viewer_commented: overrides.viewer_commented ?? false,
  unread: overrides.unread ?? false,
  reviewed_head_stale: overrides.reviewed_head_stale ?? false,
  hydrated: overrides.hydrated ?? 1,
});

const makeExpert = (overrides: Partial<ExpertiseExpert>): ExpertiseExpert => ({
  author_email: overrides.author_email ?? 'alice@example.com',
  author_login:
    overrides.author_login !== undefined ? overrides.author_login : 'alice',
  surviving_lines: overrides.surviving_lines ?? 312,
  total_lines: overrides.total_lines ?? 480,
  commit_score: overrides.commit_score ?? 0.4,
  is_original: overrides.is_original ?? true,
  score: overrides.score ?? 86.7,
  dormant: overrides.dormant ?? false,
});

describe('expertise helpers', () => {
  it('counts PR rows that touch your code', () => {
    const prs = [
      makePR({ id: 'pr-1', touches_you: true }),
      makePR({ id: 'pr-2', touches_you: false }),
      makePR({ id: 'pr-3', touches_you: true }),
    ];

    expect(touchesYouCount(prs)).toBe(2);
  });

  it('selects the routed PR for expertise commands over the cursor PR id', () => {
    const prs = [makePR({ id: 'pr-list-row' }), makePR({ id: 'pr-focused' })];

    expect(focusedExpertisePR(prs, 'pr-focused', 'pr-list-row')?.id).toBe(
      'pr-focused',
    );
  });

  it('selects the cursor PR by its projected id instead of its row index', () => {
    const prs = [makePR({ id: 'pr-first' }), makePR({ id: 'pr-focused' })];

    expect(focusedExpertisePR(prs, undefined, 'pr-focused')?.id).toBe(
      'pr-focused',
    );
  });

  it('formats an original-author why line with rounded score', () => {
    expect(formatWhy(makeExpert({ score: 86.7 }))).toBe(
      'alice: 312/480 surviving lines, original author → 87',
    );
  });

  it('omits original-author text when false', () => {
    expect(
      formatWhy(
        makeExpert({
          author_login: null,
          author_email: 'bob@example.com',
          is_original: false,
          score: 42.2,
        }),
      ),
    ).toBe('bob@example.com: 312/480 surviving lines → 42');
  });

  it('normalizes GET_EXPERTISE payloads to the requested PR id', () => {
    expect(expertisePayloadForPR({ files: [] }, 'pr-1')).toEqual({
      pr_id: 'pr-1',
      files: [],
    });
  });

  it('checks whether an expertise payload belongs to the active PR', () => {
    expect(isExpertisePayloadForPR({ pr_id: 'pr-1', files: [] }, 'pr-1')).toBe(
      true,
    );
    expect(isExpertisePayloadForPR({ pr_id: 'pr-1', files: [] }, 'pr-2')).toBe(
      false,
    );
  });
});
