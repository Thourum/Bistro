import { describe, expect, it, vi } from 'vitest';
import type { UnrenderableDiffFile } from './diffCodeViewItems';
import { requestSelectedFileScroll } from './diffCodeViewNavigation';

const RENDERABLE_FILE_ID = 'renderable-file';
const UNRENDERABLE_FILE_ID = 'unrenderable-file';

function createUnrenderableFiles(): UnrenderableDiffFile[] {
  return [
    {
      file: {
        id: UNRENDERABLE_FILE_ID,
        path: 'src/unrenderable.ts',
        oldFile: 'a/src/unrenderable.ts',
        newFile: 'b/src/unrenderable.ts',
        canonicalKey: 'src/unrenderable.ts',
        lang: 'typescript',
        additions: 1,
        deletions: 1,
        patch: 'invalid patch',
        lineCount: 2,
      },
      reason: 'parse-failed',
    },
  ];
}

function createHandle() {
  const scrollTo = vi.fn();
  return {
    handle: { scrollTo } as never,
    scrollTo,
  };
}

describe('requestSelectedFileScroll', () => {
  it('scrolls an available CodeView item immediately', () => {
    const { handle, scrollTo } = createHandle();
    const pendingScrollFileIdRef = { current: null };

    requestSelectedFileScroll(
      handle,
      pendingScrollFileIdRef,
      RENDERABLE_FILE_ID,
      createUnrenderableFiles(),
    );

    expect(scrollTo).toHaveBeenCalledWith({
      type: 'item',
      id: RENDERABLE_FILE_ID,
      align: 'start',
      behavior: 'instant',
    });
    expect(pendingScrollFileIdRef.current).toBeNull();
  });

  it('queues a renderable selection until CodeView remounts', () => {
    const pendingScrollFileIdRef = { current: null };

    requestSelectedFileScroll(
      null,
      pendingScrollFileIdRef,
      RENDERABLE_FILE_ID,
      createUnrenderableFiles(),
    );

    expect(pendingScrollFileIdRef.current).toBe(RENDERABLE_FILE_ID);
  });

  it('clears a queued scroll when a newer selection scrolls immediately', () => {
    const { handle, scrollTo } = createHandle();
    const pendingScrollFileIdRef = { current: RENDERABLE_FILE_ID };

    requestSelectedFileScroll(
      handle,
      pendingScrollFileIdRef,
      'newer-renderable-file',
      createUnrenderableFiles(),
    );

    expect(pendingScrollFileIdRef.current).toBeNull();
    expect(scrollTo).toHaveBeenCalledWith({
      type: 'item',
      id: 'newer-renderable-file',
      align: 'start',
      behavior: 'instant',
    });
  });

  it('does not queue a parse-failure selection', () => {
    const pendingScrollFileIdRef = { current: RENDERABLE_FILE_ID };

    requestSelectedFileScroll(
      null,
      pendingScrollFileIdRef,
      UNRENDERABLE_FILE_ID,
      createUnrenderableFiles(),
    );

    expect(pendingScrollFileIdRef.current).toBeNull();
  });
});
