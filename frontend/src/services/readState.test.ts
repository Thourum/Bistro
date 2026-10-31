import { Viewed } from '@bindings/services';
import { afterEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { cancellable } from '../test/cancellable';
import { markFileViewed } from './readState';

vi.mock('@bindings/services', () => ({ Viewed: { MarkFile: vi.fn() } }));
const initialState = appStore.state;
afterEach(() => {
  appStore.setState(() => initialState);
  vi.clearAllMocks();
});

it('warns once for a stale hash and never warns for a successful ack', async () => {
  vi.mocked(Viewed.MarkFile).mockReturnValueOnce(
    cancellable(
      Promise.reject(
        new Error('runtime failure', {
          cause: {
            code: 'invalid',
            message: 'stale hash',
            reason: 'stale_hash',
          },
        }),
      ),
    ),
  );
  expect(await markFileViewed('pr-1', 'a.go', 'old')).toBeNull();
  expect(appStore.state.toasts).toHaveLength(1);
  expect(appStore.state.toasts[0]).toMatchObject({
    type: 'warning',
    message: 'Could not mark viewed: stale_hash',
  });
  appStore.setState(() => initialState);
  const ack = { pr_id: 'pr-1', file_path: 'a.go', content_hash: 'current' };
  vi.mocked(Viewed.MarkFile).mockReturnValueOnce(
    cancellable(Promise.resolve(ack)),
  );
  expect(await markFileViewed('pr-1', 'a.go', 'current')).toEqual(ack);
  expect(appStore.state.toasts).toHaveLength(0);
});
