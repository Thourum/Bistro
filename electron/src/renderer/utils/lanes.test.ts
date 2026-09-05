import { describe, expect, it } from 'vitest';
import type { PR } from '../types';
import { laneForPR, laneSections, sectionContaining } from './lanes';

const VIEWER_LOGIN = 'me';

const makePR = (overrides: Partial<PR> = {}): PR => ({
  id: 'pr-1',
  provider: 'github',
  owner: 'acme',
  repo: 'api',
  number: 1,
  title: 'Test PR',
  body: null,
  author: 'alice',
  state: 'open',
  created_at: '2026-07-18T00:00:00Z',
  updated_at: '2026-07-18T00:00:00Z',
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
  touches_you: false,
  mentioned: false,
  viewer_commented: false,
  unread: false,
  reviewed_head_stale: false,
  hydrated: 1,
  ...overrides,
});

describe('laneForPR', () => {
  it('places changes requested on your open PR in needs action', () => {
    const pr = makePR({
      author: VIEWER_LOGIN,
      review_status: 'changes_requested',
    });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(1);
  });

  it('places an open PR requesting your review in needs action', () => {
    const pr = makePR({ requested_reviewers: [VIEWER_LOGIN] });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(1);
  });

  it('places a review with new author pushes in needs action', () => {
    const pr = makePR({
      changes_requested_by: VIEWER_LOGIN,
      reviewed_head_stale: true,
    });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(1);
  });

  it('does not treat a closed review request as needs action', () => {
    const pr = makePR({
      state: 'closed',
      requested_reviewers: [VIEWER_LOGIN],
    });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(6);
  });

  it('places your merged PR in your PRs before needs-action signals', () => {
    const pr = makePR({
      author: VIEWER_LOGIN,
      state: 'merged',
      review_status: 'changes_requested',
    });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(3);
  });

  it('places your completed review on an open PR in waiting on author', () => {
    const pr = makePR({ approved_by: VIEWER_LOGIN });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(2);
  });

  it('places your comment on someone else’s open PR in waiting on author', () => {
    const pr = makePR({ viewer_commented: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(2);
  });

  it('does not place a closed commented PR in waiting on author', () => {
    const pr = makePR({ state: 'closed', viewer_commented: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(6);
  });

  it('prioritizes a re-requested review over waiting on author', () => {
    const pr = makePR({
      approved_by: VIEWER_LOGIN,
      requested_reviewers: [VIEWER_LOGIN],
    });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(1);
  });

  it('never places your own commented PR in waiting on author', () => {
    const pr = makePR({ author: VIEWER_LOGIN, viewer_commented: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(3);
  });

  it('prioritizes your PR over code ownership', () => {
    const pr = makePR({ author: VIEWER_LOGIN, touches_you: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(3);
  });

  it('places a PR touching your code in your code', () => {
    const pr = makePR({ touches_you: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(4);
  });

  it('prioritizes code ownership over a mention', () => {
    const pr = makePR({ touches_you: true, mentioned: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(4);
  });

  it('places a mentioned PR in mentioned', () => {
    const pr = makePR({ mentioned: true });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(5);
  });

  it('prioritizes an open review request over code ownership', () => {
    const pr = makePR({
      requested_reviewers: [VIEWER_LOGIN],
      touches_you: true,
    });

    expect(laneForPR(pr, VIEWER_LOGIN)).toBe(1);
  });

  it('places a PR without matching signals in FYI', () => {
    expect(laneForPR(makePR(), VIEWER_LOGIN)).toBe(6);
  });

  it('disables viewer-dependent rules for an empty viewer login', () => {
    const reviewRequest = makePR({ requested_reviewers: [VIEWER_LOGIN] });
    const codeOwnership = makePR({ touches_you: true });
    const mention = makePR({ mentioned: true });

    expect(laneForPR(reviewRequest, '')).toBe(6);
    expect(laneForPR(codeOwnership, '')).toBe(4);
    expect(laneForPR(mention, '')).toBe(5);
  });
});

describe('sectionContaining', () => {
  const sections = [
    { lane: 1 as const, startIndex: 0, count: 2 },
    { lane: 2 as const, startIndex: 2, count: 0 },
    { lane: 3 as const, startIndex: 2, count: 3 },
  ];

  it('returns the populated section containing a cursor index', () => {
    expect(sectionContaining(sections, 3)).toEqual(sections[2]);
  });

  it('returns null for an empty lane boundary or out-of-range index', () => {
    expect(sectionContaining(sections, 2)).toEqual(sections[2]);
    expect(sectionContaining(sections, 5)).toBeNull();
    expect(sectionContaining(sections, -1)).toBeNull();
  });
});

describe('laneSections', () => {
  it('assigns leading empty lanes the next populated lane start index', () => {
    const prs = [makePR({ id: 'your-pr', author: VIEWER_LOGIN })];

    expect(laneSections(prs, VIEWER_LOGIN)).toEqual([
      { lane: 1, startIndex: 0, count: 0 },
      { lane: 2, startIndex: 0, count: 0 },
      { lane: 3, startIndex: 0, count: 1 },
      { lane: 4, startIndex: 1, count: 0 },
      { lane: 5, startIndex: 1, count: 0 },
      { lane: 6, startIndex: 1, count: 0 },
    ]);
  });

  it('assigns middle and trailing empty lanes their following lane start index', () => {
    const prs = [
      makePR({ id: 'needs-action', requested_reviewers: [VIEWER_LOGIN] }),
      makePR({ id: 'mentioned', mentioned: true }),
    ];

    expect(laneSections(prs, VIEWER_LOGIN)).toEqual([
      { lane: 1, startIndex: 0, count: 1 },
      { lane: 2, startIndex: 1, count: 0 },
      { lane: 3, startIndex: 1, count: 0 },
      { lane: 4, startIndex: 1, count: 0 },
      { lane: 5, startIndex: 1, count: 1 },
      { lane: 6, startIndex: 2, count: 0 },
    ]);
  });

  it('reports the full list in a single lane', () => {
    const prs = [
      makePR({ id: 'one', touches_you: true }),
      makePR({ id: 'two', touches_you: true }),
    ];

    expect(laneSections(prs, VIEWER_LOGIN)).toEqual([
      { lane: 1, startIndex: 0, count: 0 },
      { lane: 2, startIndex: 0, count: 0 },
      { lane: 3, startIndex: 0, count: 0 },
      { lane: 4, startIndex: 0, count: 2 },
      { lane: 5, startIndex: 2, count: 0 },
      { lane: 6, startIndex: 2, count: 0 },
    ]);
  });

  it('returns contiguous counts for a normal lane distribution', () => {
    const prs = [
      makePR({ id: 'needs-action', requested_reviewers: [VIEWER_LOGIN] }),
      makePR({ id: 'waiting', approved_by: VIEWER_LOGIN }),
      makePR({ id: 'your-pr', author: VIEWER_LOGIN }),
      makePR({ id: 'your-code', touches_you: true }),
      makePR({ id: 'mentioned', mentioned: true }),
      makePR({ id: 'fyi' }),
    ];

    expect(laneSections(prs, VIEWER_LOGIN)).toEqual([
      { lane: 1, startIndex: 0, count: 1 },
      { lane: 2, startIndex: 1, count: 1 },
      { lane: 3, startIndex: 2, count: 1 },
      { lane: 4, startIndex: 3, count: 1 },
      { lane: 5, startIndex: 4, count: 1 },
      { lane: 6, startIndex: 5, count: 1 },
    ]);
  });
});
