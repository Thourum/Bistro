import type { CodeViewHandle } from '@pierre/diffs/react';
import type { DiffAnnotationMeta } from './diffAnnotations';
import {
  findUnrenderableDiffFile,
  type UnrenderableDiffFile,
} from './diffCodeViewItems';

export interface PendingScrollFileId {
  current: string | null;
}

export function scrollToCodeViewFile(
  handle: CodeViewHandle<DiffAnnotationMeta, unknown> | null,
  id: string,
  align: 'start' | 'nearest' = 'start',
): void {
  handle?.scrollTo({ type: 'item', id, align, behavior: 'instant' });
}

export function requestSelectedFileScroll(
  handle: CodeViewHandle<DiffAnnotationMeta, unknown> | null,
  pendingScrollFileIdRef: PendingScrollFileId,
  id: string,
  unrenderableFiles: readonly UnrenderableDiffFile[],
): void {
  if (findUnrenderableDiffFile(unrenderableFiles, id) !== undefined) {
    pendingScrollFileIdRef.current = null;
    return;
  }
  if (handle === null) {
    pendingScrollFileIdRef.current = id;
    return;
  }
  pendingScrollFileIdRef.current = null;
  scrollToCodeViewFile(handle, id);
}
