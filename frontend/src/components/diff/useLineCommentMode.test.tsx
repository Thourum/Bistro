// @vitest-environment jsdom
import { act, useEffect } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  invokeAction,
  makeCtx,
} from '../../../tests/unit/keybindingTestHelpers';
import { OverlayProvider, useOverlays } from '../../overlays/OverlayContext';
import { appStore } from '../../store';
import { initialUIState } from '../../store/ui';
import type { ReviewDraft } from '../../types';
import { parseCodeViewFiles } from './diffCodeViewItems';
import { splitUnifiedDiff } from './diffPathUtils';
import { useLineCommentMode } from './useLineCommentMode';

const PATCH = `diff --git a/a.ts b/a.ts
--- a/a.ts
+++ b/a.ts
@@ -10,2 +10,2 @@
-old
-gone
+new
+added
`;
const PARSED_FILES = parseCodeViewFiles(splitUnifiedDiff(PATCH)).parsedFiles;
const DRAFT: ReviewDraft = {
  id: 'D1',
  pr_id: 'pr-1',
  path: 'a.ts',
  side: 'RIGHT',
  start_line: 0,
  line: 11,
  body: 'Please explain this',
  commit_sha: 'old-head',
  created_at: '2026-10-23T00:00:00Z',
};

interface ProbeProps {
  drafts: readonly ReviewDraft[];
  prId: string | undefined;
}

function Probe({ drafts, prId }: ProbeProps): null {
  const overlays = useOverlays();
  useEffect(() => {
    const cancel = (): void =>
      invokeAction('global.cancel', makeCtx(appStore, 'diff', overlays));
    document.addEventListener('test:cancel-comment', cancel);
    return () => document.removeEventListener('test:cancel-comment', cancel);
  }, [overlays]);
  useLineCommentMode({
    prId,
    headSha: 'new-head',
    drafts,
    parsedFiles: PARSED_FILES,
    activeFileId: null,
    layout: 'split',
    codeViewRef: { current: null },
  });
  return null;
}

describe('useLineCommentMode draft editing', () => {
  let root: Root;
  let container: HTMLDivElement;

  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    container = document.createElement('div');
    root = createRoot(container);
    appStore.setState(() => ({
      ...initialUIState,
      activeInlineCommentThreadId: 'draft:D1',
    }));
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    appStore.setState(() => initialUIState);
  });

  it.each([
    { side: 'RIGHT' as const, start_line: 0, anchor: 11 },
    { side: 'LEFT' as const, start_line: 10, anchor: 10 },
  ])(
    'opens the saved $side draft with its body and range',
    async ({ side, start_line, anchor }) => {
      await act(async () =>
        root.render(
          <OverlayProvider>
            <Probe drafts={[{ ...DRAFT, side, start_line }]} prId="pr-1" />
          </OverlayProvider>,
        ),
      );
      await act(async () => {
        document.dispatchEvent(new CustomEvent('bistro:diff:edit-draft'));
      });
      expect(appStore.state.lineComment).toMatchObject({
        prId: 'pr-1',
        fileId: PARSED_FILES[0]?.file.id,
        path: 'a.ts',
        headSha: 'old-head',
        layout: 'split',
        cursor: { side, anchor, head: 11 },
        composer: { draftId: 'D1', initialBody: 'Please explain this' },
      });
    },
  );

  it('clears the open draft composer when the central cancel action closes its overlay', async () => {
    await act(async () =>
      root.render(
        <OverlayProvider>
          <Probe drafts={[DRAFT]} prId="pr-1" />
        </OverlayProvider>,
      ),
    );
    await act(async () =>
      document.dispatchEvent(new CustomEvent('bistro:diff:edit-draft')),
    );
    expect(appStore.state.lineComment?.composer?.draftId).toBe('D1');
    await act(async () =>
      document.dispatchEvent(new CustomEvent('test:cancel-comment')),
    );
    expect(appStore.state.lineComment).toBeNull();
  });

  it.each([
    { drafts: [], prId: 'pr-1' },
    { drafts: [{ ...DRAFT, path: 'missing.ts' }], prId: 'pr-1' },
    { drafts: [DRAFT], prId: undefined },
  ])(
    'ignores unavailable draft context: $prId $drafts',
    async ({ drafts, prId }) => {
      await act(async () =>
        root.render(
          <OverlayProvider>
            <Probe drafts={drafts} prId={prId} />
          </OverlayProvider>,
        ),
      );
      await act(async () => {
        document.dispatchEvent(new CustomEvent('bistro:diff:edit-draft'));
      });
      expect(appStore.state.lineComment).toBeNull();
    },
  );
});
