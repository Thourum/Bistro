// @vitest-environment jsdom
//
// Unit tests for PierreDiffView — covers:
//  1. PatchDiff receives lineAnnotations filtered to the current file.
//  2. renderAnnotation is wired when annotations exist for a file.
//  3. No annotations are passed for files with no matching threads.
//  4. annotation has correct side, lineNumber, metadata.
//  5. j/k keyboard navigation changes the active annotation.
//  6. Enter opens reply UI for the active annotation.
//  7. Escape closes the reply UI.
//  8. Shortcuts are ignored when textarea/input has focus.
//  9. j/k/Enter are safe with zero annotations.

import * as matchers from '@testing-library/jest-dom/matchers';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PierreDiffView } from '../../src/renderer/components/PierreDiffView';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';
import type { Comment, ReviewThread } from '../../src/renderer/types';
import type { KeyActionContext } from '../../src/renderer/utils/keybindings';
import { DEFAULT_KEYBINDINGS } from '../../src/renderer/utils/keybindings';

expect.extend(matchers);

// ---------------------------------------------------------------------------
// scrollIntoView / scrollTo are not implemented in jsdom — stub them out so
// tests don't print "Not implemented" warnings for those paths.
// ---------------------------------------------------------------------------
Element.prototype.scrollIntoView = vi.fn();
Element.prototype.scrollBy = vi.fn();

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Capture what PatchDiff is called with per invocation.
const patchDiffCalls: Array<{
  lineAnnotations: unknown;
  renderAnnotation: unknown;
}> = [];

type MockAnnotation = {
  side: string;
  lineNumber: number;
  metadata: ReviewThread | undefined;
};

vi.mock('@pierre/diffs/react', () => ({
  PatchDiff: ({
    lineAnnotations,
    renderAnnotation,
  }: {
    patch: string;
    options?: unknown;
    disableWorkerPool?: boolean;
    className?: string;
    lineAnnotations?: MockAnnotation[];
    renderAnnotation?: (a: MockAnnotation) => React.ReactNode;
  }) => {
    patchDiffCalls.push({ lineAnnotations, renderAnnotation });
    return (
      <div
        data-testid="patch-diff"
        data-has-annotations={lineAnnotations !== undefined ? 'true' : 'false'}
        data-has-render-annotation={
          renderAnnotation !== undefined ? 'true' : 'false'
        }
      >
        {/* Render annotation cards so keyboard-nav tests can inspect them. */}
        {lineAnnotations?.map((ann) => (
          <div key={`${ann.lineNumber}-${ann.metadata?.id ?? 'x'}`}>
            {renderAnnotation?.(ann)}
          </div>
        ))}
      </div>
    );
  },
}));

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: () => ({
    getVirtualItems: () => [{ key: 'f0', index: 0, start: 0, end: 100 }],
    getTotalSize: () => 100,
    scrollToIndex: vi.fn(),
    measureElement: vi.fn(),
    scrollOffset: 0,
  }),
}));

vi.mock('../../src/renderer/components/Markdown', () => ({
  Markdown: ({ body }: { body: string }) => (
    <span data-testid="md">{body}</span>
  ),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const MINIMAL_PATCH = `diff --git a/src/foo.ts b/src/foo.ts
--- a/src/foo.ts
+++ b/src/foo.ts
@@ -1,2 +1,3 @@
 unchanged
+added line
-removed line
`;

const TWO_FILE_PATCH = `diff --git a/src/foo.ts b/src/foo.ts
--- a/src/foo.ts
+++ b/src/foo.ts
@@ -1,2 +1,2 @@
-old
+new
diff --git a/src/bar.ts b/src/bar.ts
--- a/src/bar.ts
+++ b/src/bar.ts
@@ -1,2 +1,2 @@
-old
+new
`;

const THREAD_FOO: ReviewThread = {
  id: 'thread-1',
  pr_id: 'pr-1',
  file_path: 'src/foo.ts',
  line: 2,
  resolved: false,
  outdated: false,
  state_known: true,
};

const THREAD_FOO_2: ReviewThread = {
  id: 'thread-2',
  pr_id: 'pr-1',
  file_path: 'src/foo.ts',
  line: 5,
  resolved: false,
  outdated: false,
  state_known: true,
};

const THREAD_BAR: ReviewThread = {
  id: 'thread-bar',
  pr_id: 'pr-1',
  file_path: 'src/bar.ts',
  line: 3,
  resolved: false,
  outdated: false,
  state_known: true,
};

const COMMENT_FOO: Comment = {
  id: 'comment-1',
  pr_id: 'pr-1',
  author: 'alice',
  body: 'looks good',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  thread_id: 'thread-1',
};

function byId(actionId: string) {
  const binding = DEFAULT_KEYBINDINGS.find((b) => b.actionId === actionId);
  if (binding === undefined) throw new Error(`missing binding ${actionId}`);
  return binding;
}

function hotkeyCtx(): KeyActionContext {
  return {
    store: appStore,
    routeBase: 'diff',
    activeOverlay: null,
    overlays: {
      activeOverlay: null,
      helpSourceRoute: 'list',
      openCommand: vi.fn(),
      openHelp: vi.fn(),
      openSearch: vi.fn(),
      openFilter: vi.fn(),
      toggleFilter: vi.fn(),
      openRepoPicker: vi.fn(),
      openThreads: vi.fn(),
      closeOverlay: vi.fn(),
      closeSearch: vi.fn(),
    },
    location: { pathname: '/prs/pr-1/diff' },
    prId: 'pr-1',
  };
}

function pressJ(): void {
  byId('detail.scrollDown').action(hotkeyCtx());
}

function pressK(): void {
  byId('detail.scrollUp').action(hotkeyCtx());
}

function pressEnter(): void {
  byId('detail.inlineReply').action(hotkeyCtx());
}

function pressEscape(): void {
  byId('global.escape').action(hotkeyCtx());
}

beforeEach(() => {
  appStore.setState(() => initialUIState);
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PierreDiffView annotation wiring', () => {
  beforeEach(() => {
    patchDiffCalls.length = 0;
    appStore.setState(() => initialUIState);
  });

  it('passes lineAnnotations and renderAnnotation to PatchDiff when thread matches file', () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[COMMENT_FOO]}
      />,
    );

    const patchDiff = screen.getAllByTestId('patch-diff')[0];
    expect(patchDiff).toBeDefined();
    expect(patchDiff!.getAttribute('data-has-annotations')).toBe('true');
    expect(patchDiff!.getAttribute('data-has-render-annotation')).toBe('true');
  });

  it('does NOT pass lineAnnotations when no thread matches the file', () => {
    const threadOtherFile: ReviewThread = {
      ...THREAD_FOO,
      id: 'thread-2',
      file_path: 'src/other.ts',
    };
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[threadOtherFile]}
        comments={[]}
      />,
    );

    const patchDiff = screen.getAllByTestId('patch-diff')[0];
    expect(patchDiff!.getAttribute('data-has-annotations')).toBe('false');
    expect(patchDiff!.getAttribute('data-has-render-annotation')).toBe('false');
  });

  it('does NOT publish invalid or outdated thread anchors as inline annotations', () => {
    const invalidThreads: ReviewThread[] = [
      { ...THREAD_FOO, id: 'line-zero', line: 0 },
      { ...THREAD_FOO, id: 'outdated', outdated: true },
      { ...THREAD_FOO, id: 'unknown', state_known: false },
    ];

    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={invalidThreads}
        comments={[]}
      />,
    );

    const patchDiff = screen.getAllByTestId('patch-diff')[0];
    expect(patchDiff!.getAttribute('data-has-annotations')).toBe('false');
    expect(appStore.state.inlineCommentThreadIds).toEqual([]);
  });

  it('does NOT pass lineAnnotations when no threads provided', () => {
    render(<PierreDiffView diff={MINIMAL_PATCH} theme="light" />);

    const patchDiff = screen.getAllByTestId('patch-diff')[0];
    expect(patchDiff!.getAttribute('data-has-annotations')).toBe('false');
  });

  it('annotation has correct side and lineNumber for the thread', () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    const call = patchDiffCalls[0];
    expect(call).toBeDefined();
    const annotations = call!.lineAnnotations as MockAnnotation[];
    expect(annotations).toHaveLength(1);
    expect(annotations[0]!.side).toBe('additions');
    expect(annotations[0]!.lineNumber).toBe(2);
    expect(annotations[0]!.metadata?.id).toBe('thread-1');
  });
});

// ---------------------------------------------------------------------------
// Keyboard navigation: j/k
// ---------------------------------------------------------------------------

describe('PierreDiffView keyboard navigation (j/k)', () => {
  beforeEach(() => {
    patchDiffCalls.length = 0;
    appStore.setState(() => initialUIState);
  });

  it('j moves to first annotation when none is active', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    // Initially no annotation is active.
    const cards = screen.getAllByTestId('thread-annotation-card');
    expect(cards[0]!.getAttribute('data-active')).toBe('false');

    act(() => pressJ());

    await waitFor(() => {
      const updated = screen.getAllByTestId('thread-annotation-card');
      expect(updated[0]!.getAttribute('data-active')).toBe('true');
    });
  });

  it('k moves to last annotation when none is active', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO, THREAD_FOO_2]}
        comments={[]}
      />,
    );

    act(() => pressK());

    await waitFor(() => {
      const cards = screen.getAllByTestId('thread-annotation-card');
      // Last annotation (THREAD_FOO_2, line 5) becomes active.
      const activeCard = cards.find(
        (c) => c.getAttribute('data-active') === 'true',
      );
      expect(activeCard).toBeDefined();
      expect(activeCard!.getAttribute('data-thread-id')).toBe('thread-2');
    });
  });

  it('j then j moves forward through annotations', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO, THREAD_FOO_2]}
        comments={[]}
      />,
    );

    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });

    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-2');
    });
  });

  it('k moves backward after j', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO, THREAD_FOO_2]}
        comments={[]}
      />,
    );

    // Move to first annotation.
    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });

    // Move to second annotation (separate act so state is committed first).
    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-2');
    });

    // Back to first.
    act(() => pressK());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });
  });

  it('j does not go past the last annotation', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    // Navigate to the only annotation.
    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });

    // j again — should stay on the same annotation.
    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });
  });

  it('k does not go before the first annotation', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO, THREAD_FOO_2]}
        comments={[]}
      />,
    );

    // Navigate to first annotation.
    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });

    // k — should stay on first.
    act(() => pressK());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });
  });
});

// ---------------------------------------------------------------------------
// Reply UI: Enter / Escape / Cancel
// ---------------------------------------------------------------------------

describe('PierreDiffView reply UI (Enter/Escape)', () => {
  beforeEach(() => {
    patchDiffCalls.length = 0;
  });

  it('Enter opens reply composer for the active annotation', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    // Activate an annotation first.
    act(() => pressJ());
    await waitFor(() => {
      const active = screen
        .getAllByTestId('thread-annotation-card')
        .find((c) => c.getAttribute('data-active') === 'true');
      expect(active).toBeDefined();
    });

    // Now press Enter to open the reply composer.
    act(() => pressEnter());

    await waitFor(() => {
      expect(screen.getByTestId('reply-composer')).toBeInTheDocument();
    });

    // The card should be in replying state.
    const card = screen
      .getAllByTestId('thread-annotation-card')
      .find((c) => c.getAttribute('data-active') === 'true');
    expect(card?.getAttribute('data-replying')).toBe('true');
  });

  it('Enter does nothing when no annotation is active', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    act(() => pressEnter());

    await waitFor(() => {
      expect(screen.queryByTestId('reply-composer')).not.toBeInTheDocument();
    });
  });

  it('Escape closes the reply composer', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    // Open reply via j + Enter.
    act(() => pressJ());
    await waitFor(() =>
      screen
        .getAllByTestId('thread-annotation-card')
        .some((c) => c.getAttribute('data-active') === 'true'),
    );
    act(() => pressEnter());
    await waitFor(() => {
      expect(screen.getByTestId('reply-composer')).toBeInTheDocument();
    });

    // Escape should close it.
    act(() => pressEscape());
    await waitFor(() => {
      expect(screen.queryByTestId('reply-composer')).not.toBeInTheDocument();
    });
  });

  it('Cancel button closes the reply composer', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    act(() => pressJ());
    await waitFor(() =>
      screen
        .getAllByTestId('thread-annotation-card')
        .some((c) => c.getAttribute('data-active') === 'true'),
    );
    act(() => pressEnter());
    await waitFor(() => {
      expect(screen.getByTestId('reply-composer')).toBeInTheDocument();
    });

    // Click Cancel — scope query to the reply composer to avoid ambiguity.
    act(() => {
      const composer = screen.getByTestId('reply-composer');
      fireEvent.click(
        within(composer).getByRole('button', { name: /cancel/i }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByTestId('reply-composer')).not.toBeInTheDocument();
    });
  });

  it('reply composer shows textarea labelled "Reply to thread"', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    act(() => pressJ());
    await waitFor(() =>
      screen
        .getAllByTestId('thread-annotation-card')
        .some((c) => c.getAttribute('data-active') === 'true'),
    );
    act(() => pressEnter());
    await waitFor(() => {
      expect(
        screen.getByRole('textbox', { name: /reply to thread/i }),
      ).toBeInTheDocument();
    });
  });

  it('Escape inside the reply textarea closes the composer', async () => {
    render(
      <PierreDiffView
        diff={MINIMAL_PATCH}
        theme="light"
        threads={[THREAD_FOO]}
        comments={[]}
      />,
    );

    act(() => pressJ());
    await waitFor(() =>
      screen
        .getAllByTestId('thread-annotation-card')
        .some((c) => c.getAttribute('data-active') === 'true'),
    );
    act(() => pressEnter());
    const textarea = await screen.findByRole('textbox', {
      name: /reply to thread/i,
    });

    act(() => {
      textarea.focus();
      fireEvent.keyDown(textarea, { key: 'Escape' });
    });

    await waitFor(() => {
      expect(screen.queryByTestId('reply-composer')).not.toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// Text-input guard: shortcuts ignored when textarea/input has focus
// ---------------------------------------------------------------------------

describe('PierreDiffView shortcuts are ignored inside text fields', () => {
  beforeEach(() => {
    patchDiffCalls.length = 0;
  });

  it('j does not move annotation when a textarea has focus', async () => {
    render(
      <>
        <textarea data-testid="outside-textarea" />
        <PierreDiffView
          diff={MINIMAL_PATCH}
          theme="light"
          threads={[THREAD_FOO]}
          comments={[]}
        />
      </>,
    );

    const textarea = screen.getByTestId('outside-textarea');
    act(() => {
      textarea.focus();
      fireEvent.keyDown(textarea, { key: 'j' });
    });

    // No annotation should become active.
    await waitFor(() => {
      const cards = screen.getAllByTestId('thread-annotation-card');
      expect(
        cards.every((c) => c.getAttribute('data-active') === 'false'),
      ).toBe(true);
    });
  });

  it('j does not move annotation when an input has focus', async () => {
    render(
      <>
        <input type="text" data-testid="outside-input" />
        <PierreDiffView
          diff={MINIMAL_PATCH}
          theme="light"
          threads={[THREAD_FOO]}
          comments={[]}
        />
      </>,
    );

    const input = screen.getByTestId('outside-input');
    act(() => {
      input.focus();
      fireEvent.keyDown(input, { key: 'j' });
    });

    await waitFor(() => {
      const cards = screen.getAllByTestId('thread-annotation-card');
      expect(
        cards.every((c) => c.getAttribute('data-active') === 'false'),
      ).toBe(true);
    });
  });
});

// ---------------------------------------------------------------------------
// Edge cases: zero annotations
// ---------------------------------------------------------------------------

describe('PierreDiffView with zero annotations', () => {
  beforeEach(() => {
    patchDiffCalls.length = 0;
  });

  it('j/k/Enter are safe with zero annotations', async () => {
    render(<PierreDiffView diff={MINIMAL_PATCH} theme="light" />);

    // Should not throw or cause any state errors.
    expect(() => {
      act(() => {
        pressJ();
        pressK();
        pressEnter();
      });
    }).not.toThrow();

    await waitFor(() => {
      expect(screen.queryByTestId('reply-composer')).not.toBeInTheDocument();
    });
  });

  it('renders "No changes" for an empty diff', () => {
    render(<PierreDiffView diff="" theme="light" />);
    expect(screen.getByText('No changes')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Annotation ordering: file order → line number → thread id
// ---------------------------------------------------------------------------

describe('PierreDiffView annotation ordering', () => {
  beforeEach(() => {
    patchDiffCalls.length = 0;
  });

  it('annotations from earlier files come before later files', async () => {
    // TWO_FILE_PATCH has src/foo.ts first, src/bar.ts second.
    // The virtualizer mock only renders index 0 (foo.ts), so only foo
    // annotations appear in DOM. We verify that j lands on the foo thread.
    render(
      <PierreDiffView
        diff={TWO_FILE_PATCH}
        theme="light"
        threads={[THREAD_BAR, THREAD_FOO]}
        comments={[]}
      />,
    );

    act(() => pressJ());

    // First active annotation should be thread-1 (src/foo.ts line 2), not
    // thread-bar (src/bar.ts line 3), because file order determines priority.
    await waitFor(() => {
      const cards = screen.getAllByTestId('thread-annotation-card');
      const active = cards.find(
        (c) => c.getAttribute('data-active') === 'true',
      );
      expect(active?.getAttribute('data-thread-id')).toBe('thread-1');
    });
  });
});
