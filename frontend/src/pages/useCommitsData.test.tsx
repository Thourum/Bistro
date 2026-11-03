// @vitest-environment jsdom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import { cachedCommitList } from '../services/commits';
import { appStore } from '../store';
import { cancellable } from '../test/cancellable';
import { deferred } from '../test/deferred';
import type { CommitsResult } from '../types';
import { useCommitsData } from './useCommitsData';

const { List, SetViewed } = vi.hoisted(() => ({
  List: vi.fn(),
  SetViewed: vi.fn(),
}));
vi.mock('@bindings/services', () => ({ Commits: { List, SetViewed } }));
const initialState = appStore.state;
afterEach(() => {
  appStore.setState(() => initialState);
  vi.resetAllMocks();
});

interface ProbeProps {
  onData: (data: ReturnType<typeof useCommitsData>) => void;
}
function Probe({ onData }: ProbeProps): null {
  onData(useCommitsData('remount-rollback-pr'));
  return null;
}

it('adopts a failed old viewed write rollback after remount and refresh', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const list: CommitsResult = {
    pr_id: 'remount-rollback-pr',
    viewer_login: initialState.viewerLogin,
    available: true,
    reason: '',
    truncated: false,
    commits: [
      {
        sha: 'first',
        parent_shas: [],
        subject: 'first',
        body: '',
        author_name: '',
        authored_at: '',
        is_merge: false,
        additions: 0,
        deletions: 0,
        file_count: 0,
        viewed: false,
      },
    ],
  };
  List.mockImplementationOnce(() => cancellable(Promise.resolve(list)));
  const oldWrite = deferred<unknown>();
  SetViewed.mockImplementationOnce(() => cancellable(oldWrite.promise));
  const seen: ReturnType<typeof useCommitsData>[] = [];
  const onData = (data: ReturnType<typeof useCommitsData>): void => {
    seen.push(data);
  };
  const root = createRoot(document.createElement('div'));
  await act(async () => root.render(<Probe onData={onData} />));
  const mounted = seen.at(-1);
  expect(mounted).toBeDefined();
  await act(async () => {
    void mounted?.toggleViewed('first');
  });
  expect(seen.at(-1)?.result?.commits?.[0]?.viewed).toBe(true);
  await act(async () => root.unmount());

  const refresh = deferred<CommitsResult>();
  List.mockImplementationOnce(() => cancellable(refresh.promise));
  const remounted = createRoot(document.createElement('div'));
  await act(async () => remounted.render(<Probe onData={onData} />));
  await act(async () => refresh.resolve(list));
  expect(seen.at(-1)?.result?.commits?.[0]?.viewed).toBe(true);
  await act(async () => oldWrite.reject(new Error('offline')));
  expect(cachedCommitList('remount-rollback-pr')?.commits?.[0]?.viewed).toBe(
    false,
  );
  expect(seen.at(-1)?.result?.commits?.[0]?.viewed).toBe(false);
  await act(async () => remounted.unmount());
});
