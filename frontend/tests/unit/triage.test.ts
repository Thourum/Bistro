import { describe, expect, it } from 'vitest';
import type { PR } from '../../src/types';
import {
  classifyPR,
  countTriageLanes,
  prBrowserUrl,
} from '../../src/utils/triage';

function buildPR(overrides: Partial<PR> = {}): PR {
  return {
    id: 'github:acme/api:1',
    provider: 'github',
    owner: 'acme',
    repo: 'api',
    number: 1,
    title: 'Test PR',
    body: null,
    author: 'someone-else',
    state: 'open',
    created_at: '2026-06-01T10:00:00Z',
    updated_at: '2026-06-01T10:00:00Z',
    merged_at: null,
    base_branch: 'main',
    head_branch: 'feature',
    base_sha: 'abc',
    head_sha: 'def',
    comment_count: 0,
    ci_status: null,
    draft: 0,
    review_status: null,
    approved_by: null,
    changes_requested_by: null,
    requested_reviewers: [],
    touches_you: false,
    mentioned: false,
    viewer_commented: false,
    unread: false,
    reviewed_head_stale: false,
    hydrated: 1,
    ...overrides,
  };
}

describe('classifyPR', () => {
  it('classifies review-requested PRs as needs_action', () => {
    const pr = buildPR({ requested_reviewers: ['me', 'other'] });
    expect(classifyPR(pr, 'me')).toBe('needs_action');
  });

  it('classifies own PRs with changes requested as needs_action', () => {
    const pr = buildPR({ author: 'me', review_status: 'changes_requested' });
    expect(classifyPR(pr, 'me')).toBe('needs_action');
  });

  it('classifies approved PRs as waiting_author', () => {
    const pr = buildPR({ review_status: 'approved' });
    expect(classifyPR(pr, 'me')).toBe('waiting_author');
  });

  it('classifies drafts as fyi even when review is requested', () => {
    const pr = buildPR({ draft: 1, requested_reviewers: ['me'] });
    expect(classifyPR(pr, 'me')).toBe('fyi');
  });

  it('classifies unrelated PRs as fyi', () => {
    expect(classifyPR(buildPR(), 'me')).toBe('fyi');
  });

  it('never classifies as needs_action without a username', () => {
    const pr = buildPR({ requested_reviewers: ['me'] });
    expect(classifyPR(pr, '')).toBe('fyi');
  });

  it("someone else's changes-requested PR is not my action", () => {
    const pr = buildPR({
      author: 'someone-else',
      review_status: 'changes_requested',
    });
    expect(classifyPR(pr, 'me')).toBe('fyi');
  });

  it('handles missing requested_reviewers defensively', () => {
    const pr = buildPR();
    // Simulate older cached rows where the field is absent.
    const legacy = { ...pr, requested_reviewers: undefined } as unknown as PR;
    expect(classifyPR(legacy, 'me')).toBe('fyi');
  });
});

describe('countTriageLanes', () => {
  it('counts each lane', () => {
    const prs = [
      buildPR({ id: 'a', requested_reviewers: ['me'] }),
      buildPR({ id: 'b', review_status: 'approved' }),
      buildPR({ id: 'c' }),
      buildPR({ id: 'd', draft: 1 }),
    ];
    expect(countTriageLanes(prs, 'me')).toEqual({
      needsAction: 1,
      waitingAuthor: 1,
      fyi: 2,
    });
  });

  it('returns zeros for an empty list', () => {
    expect(countTriageLanes([], 'me')).toEqual({
      needsAction: 0,
      waitingAuthor: 0,
      fyi: 0,
    });
  });
});

describe('prBrowserUrl', () => {
  it('builds GitHub PR browser URLs from local PR metadata', () => {
    expect(
      prBrowserUrl(buildPR({ owner: 'paperclip', repo: 'bistro', number: 33 })),
    ).toBe('https://github.com/paperclip/bistro/pull/33');
  });

  it('returns null for unsupported providers', () => {
    expect(prBrowserUrl(buildPR({ provider: 'gitlab' }))).toBeNull();
  });
});
