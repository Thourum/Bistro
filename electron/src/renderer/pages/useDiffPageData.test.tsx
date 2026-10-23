// @vitest-environment jsdom
import { okAsync } from '@thourum/fault';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, onTestFinished, vi } from 'vitest';
import type { DiffFile, PR } from '../types';
import { useDiffPageData } from './useDiffPageData';

const request = vi.hoisted(() => vi.fn());
vi.mock('../services/ws/connection', () => ({ wsClient: { request } }));

const PR_FIXTURE: PR = {
  id: 'pr-1',
  provider: 'github',
  owner: 'owner',
  repo: 'repo',
  number: 1,
  title: 'Update files',
  body: null,
  author: 'author',
  state: 'open',
  created_at: '2026-10-23T00:00:00Z',
  updated_at: '2026-10-23T00:00:00Z',
  merged_at: null,
  base_branch: 'master',
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
};

const CAPPED_FILE: DiffFile = {
  path: 'huge.c',
  status: 'modified',
  is_binary: false,
  is_truncated: true,
  diff_status: 'truncated',
  patch_bytes: 614400,
  hunk_count: 0,
  additions: 1,
  deletions: 1,
  content_hash: 'hash',
  viewed: false,
};

interface ProbeProps {
  pr: PR;
  onData: (data: ReturnType<typeof useDiffPageData>) => void;
}

function Probe({ pr, onData }: ProbeProps): null {
  onData(
    useDiffPageData({ pr, isAwaitingHydration: false, commentsRevision: 0 }),
  );
  return null;
}

it('preserves cap-map identity across viewed updates and refreshes changed metadata', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const seenData: ReturnType<typeof useDiffPageData>[] = [];
  const onData = (data: ReturnType<typeof useDiffPageData>): void => {
    seenData.push(data);
  };
  const files: DiffFile[] = [
    CAPPED_FILE,
    { ...CAPPED_FILE, path: 'legacy.c', diff_status: 'too_large' },
    { ...CAPPED_FILE, path: 'small.ts', diff_status: 'ok' },
    { ...CAPPED_FILE, path: 'logo.png', status: 'binary', is_binary: true },
  ];
  request.mockImplementation((type: string) =>
    okAsync(type === 'REQUEST_DIFF' ? { diff: '', files } : { comments: [] }),
  );
  const container = document.createElement('div');
  const root = createRoot(container);
  onTestFinished(async () => {
    await act(async () => root.unmount());
    request.mockReset();
  });
  await act(async () => root.render(<Probe pr={PR_FIXTURE} onData={onData} />));
  const before = seenData.at(-1);
  expect(before).toBeDefined();
  if (before === undefined) return;

  await act(async () => {
    before.setFiles((previous) =>
      previous.map((file) => ({ ...file, viewed: true })),
    );
  });
  const afterViewed = seenData.at(-1);
  expect(afterViewed?.files.every((file) => file.viewed)).toBe(true);
  expect(afterViewed?.oversizedPatchBytes).toBe(before.oversizedPatchBytes);
  expect(afterViewed?.oversizedPatchBytes).toEqual(
    new Map([
      ['huge.c', 614400],
      ['legacy.c', 614400],
    ]),
  );

  request.mockImplementation((type: string) =>
    okAsync(
      type === 'REQUEST_DIFF'
        ? { diff: '', files: [{ ...CAPPED_FILE, patch_bytes: 716800 }] }
        : { comments: [] },
    ),
  );
  await act(async () =>
    root.render(
      <Probe pr={{ ...PR_FIXTURE, head_sha: 'updated' }} onData={onData} />,
    ),
  );
  const afterRefresh = seenData.at(-1);
  expect(afterRefresh?.oversizedPatchBytes).not.toBe(
    before.oversizedPatchBytes,
  );
  expect(afterRefresh?.oversizedPatchBytes).toEqual(
    new Map([['huge.c', 716800]]),
  );
});
