// @vitest-environment jsdom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { cancellable } from '../test/cancellable';
import type { CommitsResult } from '../types';
import {
  COMMIT_TOGGLE_VIEWED_EVENT,
  COMMIT_VIEWED_AND_NEXT_EVENT,
} from '../utils/commitNavigation';
import { useCommitData } from './useCommitData';
import { useCommitsData } from './useCommitsData';

const { List, Get, SetViewed, navigate } = vi.hoisted(() => ({
  List: vi.fn(),
  Get: vi.fn(),
  SetViewed: vi.fn(),
  navigate: vi.fn(),
}));
vi.mock('@bindings/services', () => ({ Commits: { List, Get, SetViewed } }));
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }));
const initialState = appStore.state;
const COMMIT_LIST: CommitsResult = {
  pr_id: 'pr-1',
  available: true,
  reason: '',
  truncated: false,
  commits: ['first', 'last'].map((sha) => ({
    sha,
    parent_shas: [],
    subject: sha,
    body: '',
    author_name: 'Author',
    authored_at: '2026-11-02T00:00:00Z',
    is_merge: false,
    additions: 1,
    deletions: 0,
    file_count: 1,
    viewed: false,
  })),
};

afterEach(() => {
  appStore.setState(() => initialState);
  vi.resetAllMocks();
});

interface CommitProbeProps {
  sha: string;
  onData: (data: ReturnType<typeof useCommitData>) => void;
}
function CommitProbe({ sha, onData }: CommitProbeProps): null {
  onData(useCommitData('pr-1', sha));
  return null;
}

it('optimistically marks, rolls back on failure, and advances only after the ack', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  List.mockImplementation(() => cancellable(Promise.resolve(COMMIT_LIST)));
  Get.mockImplementation(() =>
    cancellable(
      Promise.resolve({
        pr_id: 'pr-1',
        sha: 'first',
        available: true,
        reason: '',
        diff: '',
        additions: 1,
        deletions: 0,
      }),
    ),
  );
  const root = createRoot(document.createElement('div'));
  const seen: ReturnType<typeof useCommitData>[] = [];
  await act(async () =>
    root.render(<CommitProbe sha="first" onData={(data) => seen.push(data)} />),
  );
  expect(appStore.state.commitShas).toEqual(['first', 'last']);
  expect(appStore.state.selectedCommitIndex).toBe(0);

  let decline: (reason: unknown) => void = () => undefined;
  SetViewed.mockImplementationOnce(() =>
    cancellable(
      new Promise((_resolve, reject) => {
        decline = reject;
      }),
    ),
  );
  await act(async () =>
    document.dispatchEvent(new Event(COMMIT_VIEWED_AND_NEXT_EVENT)),
  );
  expect(seen.at(-1)?.commit?.viewed).toBe(true);
  expect(navigate).not.toHaveBeenCalled();
  await act(async () =>
    decline(
      new Error('runtime failure', {
        cause: {
          code: 'invalid',
          reason: 'commit_not_in_pr',
          message: 'not in PR',
        },
      }),
    ),
  );
  expect(seen.at(-1)?.commit?.viewed).toBe(false);
  expect(navigate).not.toHaveBeenCalled();
  expect(appStore.state.toasts.at(-1)?.message).toBe(
    'Could not update viewed: commit_not_in_pr',
  );

  SetViewed.mockImplementationOnce(() =>
    cancellable(Promise.resolve({ pr_id: 'pr-1', sha: 'first', viewed: true })),
  );
  await act(async () =>
    document.dispatchEvent(new Event(COMMIT_VIEWED_AND_NEXT_EVENT)),
  );
  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ params: { prId: 'pr-1', sha: 'last' } }),
  );
  expect(seen.at(-1)?.commit?.viewed).toBe(true);
  navigate.mockClear();
  await act(async () =>
    document.dispatchEvent(new Event(COMMIT_VIEWED_AND_NEXT_EVENT)),
  );
  expect(navigate).toHaveBeenCalledOnce();
  expect(SetViewed).toHaveBeenCalledTimes(2);

  await act(async () =>
    root.render(<CommitProbe sha="last" onData={(data) => seen.push(data)} />),
  );
  expect(appStore.state.selectedCommitIndex).toBe(1);
  navigate.mockClear();
  SetViewed.mockImplementationOnce(() =>
    cancellable(Promise.resolve({ pr_id: 'pr-1', sha: 'last', viewed: true })),
  );
  await act(async () =>
    document.dispatchEvent(new Event(COMMIT_VIEWED_AND_NEXT_EVENT)),
  );
  expect(seen.at(-1)?.commit?.viewed).toBe(true);
  expect(navigate).not.toHaveBeenCalled();
  await act(async () => root.unmount());
});

it('handles stale SHA faults separately from transport errors', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  List.mockImplementation(() => cancellable(Promise.resolve(COMMIT_LIST)));
  Get.mockImplementation(() =>
    cancellable(
      Promise.reject(
        new Error('runtime failure', {
          cause: {
            code: 'invalid',
            reason: 'commit_not_in_pr',
            message: 'not in PR',
          },
        }),
      ),
    ),
  );
  const seen: ReturnType<typeof useCommitData>[] = [];
  const root = createRoot(document.createElement('div'));
  await act(async () =>
    root.render(<CommitProbe sha="stale" onData={(data) => seen.push(data)} />),
  );
  expect(seen.at(-1)).toMatchObject({
    notInPr: true,
    error: null,
    position: null,
  });
  await act(async () => root.unmount());
});

interface ListProbeProps {
  prId: string;
  onData: (data: ReturnType<typeof useCommitsData>) => void;
}
function ListProbe({ prId, onData }: ListProbeProps): null {
  onData(useCommitsData(prId));
  return null;
}

it('discards late PR lists, clamps the cursor, and toggles the selected row', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  let finishOld: (value: CommitsResult) => void = () => undefined;
  List.mockImplementationOnce(() =>
    cancellable(
      new Promise<CommitsResult>((resolve) => {
        finishOld = resolve;
      }),
    ),
  );
  List.mockImplementationOnce(() => cancellable(Promise.resolve(COMMIT_LIST)));
  appStore.setState((state) => ({
    ...state,
    selectedCommitIndex: 99,
    commitShas: ['stale'],
  }));
  const seen: ReturnType<typeof useCommitsData>[] = [];
  const root = createRoot(document.createElement('div'));
  const onData = (data: ReturnType<typeof useCommitsData>): void => {
    seen.push(data);
  };
  await act(async () => root.render(<ListProbe prId="old" onData={onData} />));
  expect(appStore.state.commitShas).toEqual([]);
  await act(async () => root.render(<ListProbe prId="pr-1" onData={onData} />));
  expect(appStore.state.selectedCommitIndex).toBe(1);
  await act(async () => finishOld({ ...COMMIT_LIST, commits: null }));
  expect(appStore.state.commitShas).toEqual(['first', 'last']);
  SetViewed.mockImplementationOnce(() =>
    cancellable(Promise.resolve({ pr_id: 'pr-1', sha: 'last', viewed: true })),
  );
  await act(async () =>
    document.dispatchEvent(new Event(COMMIT_TOGGLE_VIEWED_EVENT)),
  );
  expect(seen.at(-1)?.result?.commits?.map((commit) => commit.viewed)).toEqual([
    false,
    true,
  ]);
  await act(async () => root.unmount());
});
