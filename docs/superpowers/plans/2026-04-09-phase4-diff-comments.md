# Phase 4: Diff Viewer & Comments — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-quality diff viewer with multi-file support, syntax highlighting, inline comment threads, and comment posting — all wired to the Go sync engine via WebSocket.

**Architecture:** DiffViewer already renders single-file diffs with line numbers and coloring. This plan extends it to handle multi-file diffs, adds syntax highlighting via highlight.js, creates CommentThread/CommentInput components for inline comments, and wires the `c`/`r` keyboard bindings to open comment boxes. PRDetailPage already fetches diffs via WebSocket REQUEST_DIFF.

**Tech Stack:** React 18, highlight.js, TanStack Store, TanStack Query, WebSocket, Vitest

---

## Current State

### Already done:
- `diff-parser.ts` (82 lines) — Parses single-file unified diffs: file headers, hunk headers, added/removed/context lines with line numbers. Handles "no newline at EOF" markers.
- `DiffViewer.tsx` (88 lines) — Renders parsed diff with line numbers (old/new), colored +/- markers, hunk headers. Has `data-diff-viewport` attribute for keyboard scroll. Uses ScrollArea.
- `diff-parser.test.ts` (89 lines) — 8 tests: file headers, hunk headers, added/removed/context lines, line numbers, empty input, no-newline marker.
- `DiffViewer.test.tsx` (66 lines) — 8 tests: renders content, file paths, context/added/removed lines, hunk headers, scrollable container, empty state.
- `PRDetailPage.tsx` (89 lines) — Fetches diff via `useWebSocket().request('REQUEST_DIFF', ...)`, renders DiffViewer.
- `useWebSocketComments.ts` (93 lines) — `useRequestComments(prId)` and `usePostComment(prId, body, line?)` hooks.
- `useWebSocket.ts` (290 lines) — Full WebSocket client with send/request/reconnect, handles SYNC_COMPLETE, PR_APPROVED, COMMENT_POSTED events.
- Keyboard bindings: `n`/`p` (next/prev file), `hjkl` (scroll), `c` (comment), `r` (reply), `0`/`$` (top/bottom) all defined in keyboard.ts.

### What needs building:
1. **Multi-file diff parsing** — `parseDiff` returns a single `DiffSection`. Need `parseMultiFileDiff` returning `DiffSection[]`.
2. **Multi-file DiffViewer** — File tabs/headers, file navigation (n/p keys use `selectedFileIndex` store field).
3. **Syntax highlighting** — Install highlight.js, create highlighter utility, integrate into DiffViewer.
4. **CommentThread component** — Render inline comments at specific lines, collapse/expand, reply box.
5. **CommentInput component** — Inline comment input that appears when `c` is pressed on a diff line.
6. **Wire comments to PRDetailPage** — Fetch comments via WebSocket, render alongside diff lines.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/renderer/utils/diff-parser.ts` | Modify | Add `parseMultiFileDiff()` returning `DiffSection[]` |
| `src/renderer/utils/highlighter.ts` | Create | highlight.js wrapper: detect language from extension, highlight code |
| `src/renderer/components/DiffViewer.tsx` | Rewrite | Multi-file support, file tabs, syntax highlighting, comment anchors |
| `src/renderer/components/DiffFileHeader.tsx` | Create | File header with name, +/- stats, expand/collapse |
| `src/renderer/components/CommentThread.tsx` | Create | Inline comment thread: author, body, timestamp, collapse/expand, reply |
| `src/renderer/components/CommentInput.tsx` | Create | Comment input box: appears at line, Cmd+Enter send, Esc cancel |
| `src/renderer/pages/PRDetailPage.tsx` | Modify | Fetch comments, pass to DiffViewer, wire comment posting |
| `tests/unit/diff-parser.test.ts` | Modify | Add multi-file diff tests |
| `tests/unit/DiffViewer.test.tsx` | Modify | Add multi-file, syntax highlighting, comment anchor tests |
| `tests/unit/CommentThread.test.tsx` | Create | Comment rendering, collapse/expand, reply tests |
| `tests/unit/CommentInput.test.tsx` | Create | Input rendering, keyboard shortcuts, submit tests |

---

## Task 1: Multi-file diff parser

**Files:**
- Modify: `src/renderer/utils/diff-parser.ts`
- Test: `tests/unit/diff-parser.test.ts`

- [ ] **Step 1: Read current diff-parser.ts**

Already read above. Current `parseDiff()` handles single file only. The `diff --git` header that separates files is not handled.

- [ ] **Step 2: Add `parseMultiFileDiff()` function**

Add a new function that splits on `diff --git` boundaries and calls `parseDiff` for each section:

```typescript
export function parseMultiFileDiff(diffText: string): DiffSection[] {
  if (!diffText || diffText.trim() === '') return [];

  // Split on "diff --git" boundaries
  const parts = diffText.split(/^(?=diff --git )/m);
  const sections: DiffSection[] = [];

  for (const part of parts) {
    const trimmed = part.trim();
    if (trimmed === '') continue;

    // Strip the "diff --git a/... b/..." line before passing to parseDiff
    const withoutGitHeader = trimmed.replace(/^diff --git [^\n]+\n?/, '');
    if (withoutGitHeader.trim() === '') continue;

    // Handle index lines (e.g., "index abc123..def456 100644")
    const withoutIndex = withoutGitHeader.replace(/^index [^\n]+\n?/m, '');

    const section = parseDiff(withoutIndex);
    // If parseDiff found file headers, use them; otherwise extract from git header
    if (!section.oldFile && !section.newFile) {
      const gitMatch = trimmed.match(/^diff --git a\/(.+) b\/(.+)/);
      if (gitMatch) {
        section.oldFile = `a/${gitMatch[1]}`;
        section.newFile = `b/${gitMatch[2]}`;
      }
    }
    sections.push(section);
  }

  return sections;
}
```

- [ ] **Step 3: Add multi-file diff tests**

```typescript
describe('parseMultiFileDiff', () => {
  it('parses multiple files from a single diff', () => {
    const diff = `diff --git a/src/foo.ts b/src/foo.ts
--- a/src/foo.ts
+++ b/src/foo.ts
@@ -1,3 +1,3 @@
 line 1
-old line 2
+new line 2
 line 3
diff --git a/src/bar.ts b/src/bar.ts
--- a/src/bar.ts
+++ b/src/bar.ts
@@ -1,2 +1,3 @@
 line 1
+added line
 line 2`;
    const sections = parseMultiFileDiff(diff);
    expect(sections).toHaveLength(2);
    expect(sections[0].newFile).toContain('foo.ts');
    expect(sections[1].newFile).toContain('bar.ts');
  });

  it('returns empty array for empty input', () => {
    expect(parseMultiFileDiff('')).toHaveLength(0);
  });

  it('handles single-file diff (no diff --git header)', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -1 +1 @@
-old
+new`;
    const sections = parseMultiFileDiff(diff);
    expect(sections).toHaveLength(1);
  });

  it('handles binary file markers', () => {
    const diff = `diff --git a/image.png b/image.png
Binary files differ
diff --git a/src/code.ts b/src/code.ts
--- a/src/code.ts
+++ b/src/code.ts
@@ -1 +1 @@
-old
+new`;
    const sections = parseMultiFileDiff(diff);
    // Binary file may produce empty section; at least the code file should parse
    expect(sections.length).toBeGreaterThanOrEqual(1);
  });
});
```

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/diff-parser.test.ts`
Expected: All pass (old + new tests)

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/utils/diff-parser.ts electron/tests/unit/diff-parser.test.ts
git commit -m "feat: add parseMultiFileDiff for multi-file unified diff support"
```

---

## Task 2: Install highlight.js and create highlighter utility

**Files:**
- Create: `src/renderer/utils/highlighter.ts`
- Create: `tests/unit/highlighter.test.ts`

- [ ] **Step 1: Install highlight.js**

```bash
cd electron && pnpm add highlight.js
```

- [ ] **Step 2: Create highlighter.ts**

```typescript
import hljs from 'highlight.js/lib/core';
// Register common languages
import typescript from 'highlight.js/lib/languages/typescript';
import javascript from 'highlight.js/lib/languages/javascript';
import go from 'highlight.js/lib/languages/go';
import python from 'highlight.js/lib/languages/python';
import json from 'highlight.js/lib/languages/json';
import css from 'highlight.js/lib/languages/css';
import xml from 'highlight.js/lib/languages/xml';
import bash from 'highlight.js/lib/languages/bash';
import yaml from 'highlight.js/lib/languages/yaml';
import sql from 'highlight.js/lib/languages/sql';
import markdown from 'highlight.js/lib/languages/markdown';
import rust from 'highlight.js/lib/languages/rust';

hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('go', go);
hljs.registerLanguage('python', python);
hljs.registerLanguage('json', json);
hljs.registerLanguage('css', css);
hljs.registerLanguage('xml', xml);
hljs.registerLanguage('bash', bash);
hljs.registerLanguage('yaml', yaml);
hljs.registerLanguage('sql', sql);
hljs.registerLanguage('markdown', markdown);
hljs.registerLanguage('rust', rust);

const EXTENSION_MAP: Record<string, string> = {
  '.ts': 'typescript',
  '.tsx': 'typescript',
  '.js': 'javascript',
  '.jsx': 'javascript',
  '.mjs': 'javascript',
  '.go': 'go',
  '.py': 'python',
  '.json': 'json',
  '.css': 'css',
  '.html': 'xml',
  '.xml': 'xml',
  '.svg': 'xml',
  '.sh': 'bash',
  '.bash': 'bash',
  '.zsh': 'bash',
  '.yml': 'yaml',
  '.yaml': 'yaml',
  '.sql': 'sql',
  '.md': 'markdown',
  '.rs': 'rust',
  '.toml': 'yaml', // close enough
};

export function detectLanguage(filePath: string): string | null {
  const ext = filePath.slice(filePath.lastIndexOf('.'));
  return EXTENSION_MAP[ext] ?? null;
}

export function highlightCode(code: string, language: string | null): string {
  if (language === null) return escapeHtml(code);

  try {
    const result = hljs.highlight(code, { language });
    return result.value;
  } catch {
    return escapeHtml(code);
  }
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
```

- [ ] **Step 3: Create highlighter tests**

```typescript
import { detectLanguage, highlightCode } from '../../src/renderer/utils/highlighter';

describe('detectLanguage', () => {
  it('detects TypeScript from .ts extension', () => {
    expect(detectLanguage('src/auth.ts')).toBe('typescript');
  });

  it('detects Go from .go extension', () => {
    expect(detectLanguage('main.go')).toBe('go');
  });

  it('returns null for unknown extensions', () => {
    expect(detectLanguage('file.xyz')).toBeNull();
  });
});

describe('highlightCode', () => {
  it('returns highlighted HTML for known language', () => {
    const result = highlightCode('const x = 1;', 'typescript');
    expect(result).toContain('hljs-');
  });

  it('returns escaped HTML for null language', () => {
    const result = highlightCode('<div>test</div>', null);
    expect(result).toContain('&lt;div&gt;');
  });

  it('handles empty code', () => {
    const result = highlightCode('', 'typescript');
    expect(result).toBe('');
  });
});
```

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/highlighter.test.ts`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/utils/highlighter.ts electron/tests/unit/highlighter.test.ts electron/package.json electron/pnpm-lock.yaml
git commit -m "feat: add syntax highlighting via highlight.js with language detection"
```

---

## Task 3: Create DiffFileHeader component

**Files:**
- Create: `src/renderer/components/DiffFileHeader.tsx`

- [ ] **Step 1: Create DiffFileHeader.tsx**

A file header showing the filename, +/- line counts, and expand/collapse toggle:

```typescript
import React from 'react';
import type { DiffSection } from '../utils/diff-parser';

interface DiffFileHeaderProps {
  section: DiffSection;
  index: number;
  isSelected: boolean;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  onClick: () => void;
}

function countChanges(section: DiffSection): { additions: number; deletions: number } {
  let additions = 0;
  let deletions = 0;
  for (const hunk of section.hunks) {
    for (const line of hunk.lines) {
      if (line.type === 'added') additions++;
      if (line.type === 'removed') deletions++;
    }
  }
  return { additions, deletions };
}

function extractFileName(filePath: string): string {
  // Strip "a/" or "b/" prefix
  return filePath.replace(/^[ab]\//, '');
}

export function DiffFileHeader({
  section,
  index,
  isSelected,
  isCollapsed,
  onToggleCollapse,
  onClick,
}: DiffFileHeaderProps): React.ReactElement {
  const fileName = extractFileName(section.newFile || section.oldFile);
  const { additions, deletions } = countChanges(section);

  return (
    <div
      onClick={onClick}
      className={`flex items-center justify-between px-4 py-2 border-b border-outline-variant cursor-pointer transition-colors ${
        isSelected
          ? 'bg-primary/10 border-l-2 border-l-primary'
          : 'bg-surface-container hover:bg-surface-container-low'
      }`}
      role="button"
      tabIndex={0}
      data-file-index={index}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
    >
      <div className="flex items-center gap-2 min-w-0">
        <button
          onClick={(e) => { e.stopPropagation(); onToggleCollapse(); }}
          className="text-on-surface-variant hover:text-on-surface text-xs w-4"
          aria-label={isCollapsed ? 'Expand' : 'Collapse'}
        >
          {isCollapsed ? '\u25B6' : '\u25BC'}
        </button>
        <span className="font-mono text-sm text-on-surface truncate">{fileName}</span>
      </div>
      <div className="flex items-center gap-2 flex-shrink-0 text-xs font-mono">
        {additions > 0 && (
          <span className="text-green-600 dark:text-green-400">+{additions}</span>
        )}
        {deletions > 0 && (
          <span className="text-red-600 dark:text-red-400">-{deletions}</span>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add electron/src/renderer/components/DiffFileHeader.tsx
git commit -m "feat: create DiffFileHeader component with +/- stats and collapse"
```

---

## Task 4: Rewrite DiffViewer for multi-file and syntax highlighting

**Files:**
- Rewrite: `src/renderer/components/DiffViewer.tsx`
- Modify: `tests/unit/DiffViewer.test.tsx`

- [ ] **Step 1: Rewrite DiffViewer.tsx**

Update to support multi-file diffs, file navigation via `selectedFileIndex`, syntax highlighting, and comment anchor points:

```typescript
import React, { useMemo } from 'react';
import { ScrollArea } from './ui/scroll-area';
import { parseMultiFileDiff } from '../utils/diff-parser';
import { detectLanguage, highlightCode } from '../utils/highlighter';
import { DiffFileHeader } from './DiffFileHeader';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { DiffLine, DiffSection } from '../utils/diff-parser';
import type { Comment } from '../types';

interface DiffViewerProps {
  diff: string;
  className?: string;
  comments?: Comment[];
  onLineClick?: (fileIndex: number, lineNumber: number) => void;
}

function getLineClasses(type: DiffLine['type']): { bg: string; text: string } {
  switch (type) {
    case 'added':
      return { bg: 'bg-green-50 dark:bg-green-900/20', text: 'text-green-900 dark:text-green-100' };
    case 'removed':
      return { bg: 'bg-red-50 dark:bg-red-900/20', text: 'text-red-900 dark:text-red-100' };
    default:
      return { bg: 'bg-surface-container-lowest', text: 'text-on-surface' };
  }
}

function getLinePrefix(type: DiffLine['type']): string {
  if (type === 'added') return '+';
  if (type === 'removed') return '-';
  return ' ';
}

function extractFileName(section: DiffSection): string {
  const path = section.newFile || section.oldFile;
  return path.replace(/^[ab]\//, '');
}

function renderHighlightedContent(content: string, language: string | null): React.ReactElement {
  const html = highlightCode(content, language);
  return <span dangerouslySetInnerHTML={{ __html: html }} />;
}

export function DiffViewer({ diff, className = '', comments = [], onLineClick }: DiffViewerProps) {
  const selectedFileIndex = useStore(appStore, (s) => s.selectedFileIndex);
  const [collapsedFiles, setCollapsedFiles] = React.useState<Set<number>>(new Set());

  const sections = useMemo(() => parseMultiFileDiff(diff), [diff]);

  if (!diff || diff.trim() === '' || sections.length === 0) {
    return (
      <div className="p-4 text-center text-on-surface-variant">
        No changes
      </div>
    );
  }

  const toggleCollapse = (index: number) => {
    setCollapsedFiles((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  const handleFileClick = (index: number) => {
    appStore.setState((s) => ({ ...s, selectedFileIndex: index }));
  };

  // Clamp selectedFileIndex
  const clampedIndex = Math.max(0, Math.min(selectedFileIndex, sections.length - 1));

  return (
    <ScrollArea data-diff-viewport className={`w-full h-full border border-outline-variant overflow-auto ${className}`}>
      <div className="font-mono text-sm bg-surface-container-lowest">
        {sections.map((section, fileIdx) => {
          const fileName = extractFileName(section);
          const language = detectLanguage(fileName);
          const isCollapsed = collapsedFiles.has(fileIdx);
          const isSelected = fileIdx === clampedIndex;

          // Find comments for this file
          const fileComments = comments.filter((c) => c.file === fileName);

          return (
            <div key={`file-${fileName}-${fileIdx}`} id={`diff-file-${fileIdx}`}>
              <DiffFileHeader
                section={section}
                index={fileIdx}
                isSelected={isSelected}
                isCollapsed={isCollapsed}
                onToggleCollapse={() => toggleCollapse(fileIdx)}
                onClick={() => handleFileClick(fileIdx)}
              />

              {!isCollapsed && section.hunks.map((hunk, hunkIdx) => (
                <div key={`hunk-${hunk.header}-${hunkIdx}`}>
                  <div className="bg-surface-container px-4 py-1 text-on-surface-variant text-xs border-b border-outline-variant">
                    {hunk.header}
                  </div>

                  {hunk.lines.map((line, lineIdx) => {
                    const { bg, text } = getLineClasses(line.type);
                    const prefix = getLinePrefix(line.type);
                    const lineNum = line.newLineNumber ?? line.oldLineNumber;

                    // Check for inline comments on this line
                    const lineComments = lineNum !== undefined
                      ? fileComments.filter((c) => c.line === lineNum)
                      : [];

                    return (
                      <React.Fragment key={`line-${line.type}-${lineIdx}`}>
                        <div
                          className={`diff-${line.type} flex border-b border-outline-variant/30 ${bg} ${
                            onLineClick ? 'cursor-pointer hover:brightness-95' : ''
                          }`}
                          onClick={() => onLineClick && lineNum !== undefined && onLineClick(fileIdx, lineNum)}
                          data-line-number={lineNum}
                          data-file-index={fileIdx}
                        >
                          <div className="w-12 flex-shrink-0 px-2 py-1 text-right text-on-surface-variant border-r border-outline-variant/50 select-none bg-surface-container-low text-[11px]">
                            {line.oldLineNumber ?? '\u00a0'}
                          </div>
                          <div className="w-12 flex-shrink-0 px-2 py-1 text-right text-on-surface-variant border-r border-outline-variant/50 select-none bg-surface-container-low text-[11px]">
                            {line.newLineNumber ?? '\u00a0'}
                          </div>
                          <div className="w-4 flex-shrink-0 px-1 py-1 select-none text-on-surface-variant text-center">
                            {prefix}
                          </div>
                          <div className={`flex-1 px-3 py-1 overflow-auto ${text} whitespace-pre-wrap break-words`}>
                            {renderHighlightedContent(line.content, language)}
                          </div>
                        </div>

                        {lineComments.map((comment) => (
                          <div
                            key={comment.id}
                            className="flex border-b border-outline-variant/30 bg-surface-container-low"
                          >
                            <div className="w-28 flex-shrink-0" />
                            <div className="flex-1 px-4 py-2 border-l-2 border-l-primary/50">
                              <div className="flex items-center gap-2 mb-1">
                                <span className="font-bold text-xs text-on-surface">@{comment.author}</span>
                                <span className="text-[10px] text-on-surface-variant">
                                  {new Date(comment.created_at).toLocaleDateString()}
                                </span>
                              </div>
                              <div className="text-xs text-on-surface whitespace-pre-wrap">{comment.body}</div>
                            </div>
                          </div>
                        ))}
                      </React.Fragment>
                    );
                  })}
                </div>
              ))}
            </div>
          );
        })}
      </div>
    </ScrollArea>
  );
}
```

- [ ] **Step 2: Import highlight.js theme CSS**

Add to `src/renderer/styles/globals.css` or import in DiffViewer:

```typescript
import 'highlight.js/styles/github-dark.css';
```

Or conditionally based on dark mode. Add the import at the top of `DiffViewer.tsx`.

- [ ] **Step 3: Update DiffViewer tests**

Update existing tests and add new ones for multi-file support:

```typescript
// Add to existing DiffViewer tests:
it('renders multiple file headers for multi-file diff', () => {
  const multiDiff = `diff --git a/foo.ts b/foo.ts
--- a/foo.ts
+++ b/foo.ts
@@ -1 +1 @@
-old
+new
diff --git a/bar.ts b/bar.ts
--- a/bar.ts
+++ b/bar.ts
@@ -1 +1 @@
-old
+new`;
  render(<DiffViewer diff={multiDiff} />);
  expect(screen.getByText(/foo\.ts/)).toBeInTheDocument();
  expect(screen.getByText(/bar\.ts/)).toBeInTheDocument();
});

it('renders inline comments at correct lines', () => {
  const comments = [{
    id: '1', pr_id: 'pr1', author: 'alice', body: 'Looks good',
    created_at: '2026-01-01', updated_at: '2026-01-01', file: 'src/auth.ts', line: 10,
  }];
  render(<DiffViewer diff={sampleDiff} comments={comments} />);
  expect(screen.getByText(/@alice/)).toBeInTheDocument();
  expect(screen.getByText(/Looks good/)).toBeInTheDocument();
});
```

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/DiffViewer.test.tsx tests/unit/diff-parser.test.ts`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/DiffViewer.tsx electron/src/renderer/components/DiffFileHeader.tsx electron/tests/unit/DiffViewer.test.tsx
git commit -m "feat: DiffViewer with multi-file support, syntax highlighting, inline comments"
```

---

## Task 5: Create CommentThread component

**Files:**
- Create: `src/renderer/components/CommentThread.tsx`
- Create: `tests/unit/CommentThread.test.tsx`

- [ ] **Step 1: Create CommentThread.tsx**

```typescript
import React, { useState } from 'react';
import type { Comment } from '../types';

interface CommentThreadProps {
  comments: Comment[];
  onReply?: (body: string) => void;
  onEdit?: (commentId: string, body: string) => void;
  onDelete?: (commentId: string) => void;
  currentUser?: string;
}

export function CommentThread({
  comments,
  onReply,
  onEdit,
  onDelete,
  currentUser,
}: CommentThreadProps): React.ReactElement {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [replyText, setReplyText] = useState('');
  const [showReply, setShowReply] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editText, setEditText] = useState('');

  if (comments.length === 0) return <></>;

  const rootComment = comments[0];
  const replies = comments.slice(1);

  const handleReplySubmit = () => {
    if (replyText.trim() && onReply) {
      onReply(replyText.trim());
      setReplyText('');
      setShowReply(false);
    }
  };

  const handleEditSubmit = (commentId: string) => {
    if (editText.trim() && onEdit) {
      onEdit(commentId, editText.trim());
      setEditingId(null);
      setEditText('');
    }
  };

  const handleReplyKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      handleReplySubmit();
    } else if (e.key === 'Escape') {
      setShowReply(false);
      setReplyText('');
    }
  };

  const handleEditKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>, commentId: string) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      handleEditSubmit(commentId);
    } else if (e.key === 'Escape') {
      setEditingId(null);
      setEditText('');
    }
  };

  const renderComment = (comment: Comment) => {
    const isAuthor = currentUser !== undefined && comment.author === currentUser;
    const isEditing = editingId === comment.id;

    return (
      <div key={comment.id} className="px-4 py-2 border-b border-outline-variant/30">
        <div className="flex items-center justify-between mb-1">
          <div className="flex items-center gap-2">
            <span className="font-bold text-xs text-on-surface">@{comment.author}</span>
            <span className="text-[10px] text-on-surface-variant">
              {new Date(comment.created_at).toLocaleDateString()}
            </span>
          </div>
          {isAuthor && !isEditing && (
            <div className="flex items-center gap-1">
              <button
                onClick={() => { setEditingId(comment.id); setEditText(comment.body); }}
                className="text-[10px] text-on-surface-variant hover:text-on-surface"
              >
                edit
              </button>
              {onDelete && (
                <button
                  onClick={() => onDelete(comment.id)}
                  className="text-[10px] text-error hover:text-error/80"
                >
                  delete
                </button>
              )}
            </div>
          )}
        </div>

        {isEditing ? (
          <div>
            <textarea
              value={editText}
              onChange={(e) => setEditText(e.target.value)}
              onKeyDown={(e) => handleEditKeyDown(e, comment.id)}
              className="w-full p-2 text-xs bg-surface-container border border-outline-variant rounded font-mono resize-none"
              rows={3}
              autoFocus
            />
            <div className="flex gap-2 mt-1">
              <button
                onClick={() => handleEditSubmit(comment.id)}
                className="text-[10px] text-primary hover:underline"
              >
                Save (Cmd+Enter)
              </button>
              <button
                onClick={() => { setEditingId(null); setEditText(''); }}
                className="text-[10px] text-on-surface-variant hover:underline"
              >
                Cancel (Esc)
              </button>
            </div>
          </div>
        ) : (
          <div className="text-xs text-on-surface whitespace-pre-wrap">{comment.body}</div>
        )}
      </div>
    );
  };

  return (
    <div className="border-l-2 border-l-primary/50 bg-surface-container-low">
      <div
        className="flex items-center justify-between px-4 py-1 cursor-pointer hover:bg-surface-container-low/80"
        onClick={() => setIsCollapsed(!isCollapsed)}
        role="button"
        tabIndex={0}
      >
        <span className="text-[10px] text-on-surface-variant font-mono">
          {comments.length} comment{comments.length !== 1 ? 's' : ''}
        </span>
        <span className="text-[10px] text-on-surface-variant">
          {isCollapsed ? '\u25B6' : '\u25BC'}
        </span>
      </div>

      {!isCollapsed && (
        <>
          {renderComment(rootComment)}
          {replies.map((reply) => (
            <div key={reply.id} className="ml-4">
              {renderComment(reply)}
            </div>
          ))}

          {onReply && !showReply && (
            <button
              onClick={() => setShowReply(true)}
              className="px-4 py-2 text-[10px] text-primary hover:underline w-full text-left"
            >
              Reply...
            </button>
          )}

          {showReply && (
            <div className="px-4 py-2">
              <textarea
                value={replyText}
                onChange={(e) => setReplyText(e.target.value)}
                onKeyDown={handleReplyKeyDown}
                placeholder="Reply... (Cmd+Enter to send, Esc to cancel)"
                className="w-full p-2 text-xs bg-surface-container border border-outline-variant rounded font-mono resize-none"
                rows={3}
                autoFocus
              />
              <div className="flex gap-2 mt-1">
                <button
                  onClick={handleReplySubmit}
                  className="text-[10px] text-primary hover:underline"
                >
                  Send (Cmd+Enter)
                </button>
                <button
                  onClick={() => { setShowReply(false); setReplyText(''); }}
                  className="text-[10px] text-on-surface-variant hover:underline"
                >
                  Cancel (Esc)
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Create CommentThread tests**

```typescript
// @vitest-environment jsdom
import { render, screen, fireEvent } from '@testing-library/react';
import { CommentThread } from '../../src/renderer/components/CommentThread';
import type { Comment } from '../../src/renderer/types';

const mockComment: Comment = {
  id: '1',
  pr_id: 'pr1',
  author: 'alice',
  body: 'This looks good!',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  line: 10,
  file: 'src/auth.ts',
};

const mockReply: Comment = {
  id: '2',
  pr_id: 'pr1',
  author: 'bob',
  body: 'Thanks!',
  created_at: '2026-01-02T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
  is_reply_to: '1',
};

describe('CommentThread', () => {
  it('renders comment author and body', () => {
    render(<CommentThread comments={[mockComment]} />);
    expect(screen.getByText(/@alice/)).toBeInTheDocument();
    expect(screen.getByText(/This looks good/)).toBeInTheDocument();
  });

  it('shows comment count', () => {
    render(<CommentThread comments={[mockComment, mockReply]} />);
    expect(screen.getByText(/2 comments/)).toBeInTheDocument();
  });

  it('renders nothing for empty comments', () => {
    const { container } = render(<CommentThread comments={[]} />);
    expect(container.innerHTML).toBe('');
  });

  it('shows reply button when onReply provided', () => {
    render(<CommentThread comments={[mockComment]} onReply={() => {}} />);
    expect(screen.getByText(/Reply\.\.\./)).toBeInTheDocument();
  });

  it('shows edit/delete for comment author', () => {
    render(<CommentThread comments={[mockComment]} currentUser="alice" onDelete={() => {}} />);
    expect(screen.getByText('edit')).toBeInTheDocument();
    expect(screen.getByText('delete')).toBeInTheDocument();
  });

  it('hides edit/delete for non-author', () => {
    render(<CommentThread comments={[mockComment]} currentUser="bob" />);
    expect(screen.queryByText('edit')).not.toBeInTheDocument();
  });

  it('can collapse and expand', () => {
    render(<CommentThread comments={[mockComment]} />);
    // Body is initially visible
    expect(screen.getByText(/This looks good/)).toBeInTheDocument();
    // Click to collapse
    fireEvent.click(screen.getByText(/1 comment/));
    expect(screen.queryByText(/This looks good/)).not.toBeInTheDocument();
    // Click to expand
    fireEvent.click(screen.getByText(/1 comment/));
    expect(screen.getByText(/This looks good/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/CommentThread.test.tsx`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/CommentThread.tsx electron/tests/unit/CommentThread.test.tsx
git commit -m "feat: CommentThread component with collapse/expand, reply, edit/delete"
```

---

## Task 6: Create CommentInput component

**Files:**
- Create: `src/renderer/components/CommentInput.tsx`
- Create: `tests/unit/CommentInput.test.tsx`

- [ ] **Step 1: Create CommentInput.tsx**

An inline comment input that appears when `c` key is pressed:

```typescript
import React, { useState, useRef, useEffect } from 'react';

interface CommentInputProps {
  fileIndex: number;
  lineNumber: number;
  onSubmit: (body: string) => void;
  onCancel: () => void;
}

export function CommentInput({
  fileIndex,
  lineNumber,
  onSubmit,
  onCancel,
}: CommentInputProps): React.ReactElement {
  const [text, setText] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      if (text.trim()) {
        onSubmit(text.trim());
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onCancel();
    }
  };

  return (
    <div
      className="flex border-b border-outline-variant/30 bg-surface-container"
      data-comment-input
      data-file-index={fileIndex}
      data-line-number={lineNumber}
    >
      <div className="w-28 flex-shrink-0" />
      <div className="flex-1 p-3">
        <div className="text-[10px] text-on-surface-variant mb-1 font-mono">
          Comment on line {lineNumber}
        </div>
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Add comment (Cmd+Enter to send, Esc to cancel)"
          className="w-full p-2 text-xs bg-surface-container-lowest border border-outline-variant rounded font-mono resize-none focus:border-primary focus:outline-none"
          rows={3}
        />
        <div className="flex items-center justify-between mt-1">
          <span className="text-[10px] text-on-surface-variant">
            Cmd+Enter to send
          </span>
          <div className="flex gap-2">
            <button
              onClick={onCancel}
              className="text-[10px] text-on-surface-variant hover:text-on-surface"
            >
              Cancel
            </button>
            <button
              onClick={() => text.trim() && onSubmit(text.trim())}
              disabled={!text.trim()}
              className="text-[10px] text-primary hover:underline disabled:opacity-50"
            >
              Send
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create CommentInput tests**

```typescript
// @vitest-environment jsdom
import { render, screen, fireEvent } from '@testing-library/react';
import { vi } from 'vitest';
import { CommentInput } from '../../src/renderer/components/CommentInput';

describe('CommentInput', () => {
  it('renders with line number', () => {
    render(<CommentInput fileIndex={0} lineNumber={42} onSubmit={() => {}} onCancel={() => {}} />);
    expect(screen.getByText(/Comment on line 42/)).toBeInTheDocument();
  });

  it('renders placeholder text', () => {
    render(<CommentInput fileIndex={0} lineNumber={1} onSubmit={() => {}} onCancel={() => {}} />);
    expect(screen.getByPlaceholderText(/Add comment/)).toBeInTheDocument();
  });

  it('calls onCancel when Cancel is clicked', () => {
    const onCancel = vi.fn();
    render(<CommentInput fileIndex={0} lineNumber={1} onSubmit={() => {}} onCancel={onCancel} />);
    fireEvent.click(screen.getByText('Cancel'));
    expect(onCancel).toHaveBeenCalled();
  });

  it('calls onSubmit with trimmed text when Send is clicked', () => {
    const onSubmit = vi.fn();
    render(<CommentInput fileIndex={0} lineNumber={1} onSubmit={onSubmit} onCancel={() => {}} />);
    const textarea = screen.getByPlaceholderText(/Add comment/);
    fireEvent.change(textarea, { target: { value: '  Great work!  ' } });
    fireEvent.click(screen.getByText('Send'));
    expect(onSubmit).toHaveBeenCalledWith('Great work!');
  });

  it('disables Send button when text is empty', () => {
    render(<CommentInput fileIndex={0} lineNumber={1} onSubmit={() => {}} onCancel={() => {}} />);
    const sendButton = screen.getByText('Send');
    expect(sendButton).toHaveAttribute('disabled');
  });

  it('auto-focuses the textarea', () => {
    render(<CommentInput fileIndex={0} lineNumber={1} onSubmit={() => {}} onCancel={() => {}} />);
    const textarea = screen.getByPlaceholderText(/Add comment/);
    expect(document.activeElement).toBe(textarea);
  });
});
```

- [ ] **Step 3: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/CommentInput.test.tsx`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/CommentInput.tsx electron/tests/unit/CommentInput.test.tsx
git commit -m "feat: CommentInput component with Cmd+Enter send, Esc cancel, auto-focus"
```

---

## Task 7: Wire comments into PRDetailPage

**Files:**
- Modify: `src/renderer/pages/PRDetailPage.tsx`

- [ ] **Step 1: Read current PRDetailPage.tsx**

Already read. Currently fetches diff but not comments.

- [ ] **Step 2: Update PRDetailPage to fetch and display comments**

Add comment fetching, comment input state, and wire to DiffViewer:

```typescript
import React, { useState, useEffect, useCallback } from 'react';
import { MainLayout } from '../layouts/MainLayout';
import { DiffViewer } from '../components/DiffViewer';
import { CommentInput } from '../components/CommentInput';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { usePRList } from '../queries/usePRList';
import { useWebSocket } from '../hooks/useWebSocket';
import type { Comment } from '../types';

export function PRDetailPage() {
  const selectedIndex = useStore(appStore, (s) => s.selectedPRIndex);
  const filterState = useStore(appStore, (s) => s.filterState);
  const confirmingAction = useStore(appStore, (s) => s.confirmingAction);
  const [diff, setDiff] = useState<string>('');
  const [diffLoading, setDiffLoading] = useState(false);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  const [commentTarget, setCommentTarget] = useState<{ fileIndex: number; lineNumber: number } | null>(null);

  const { data: prs = [], isLoading, error } = usePRList(filterState);
  const pr = prs[selectedIndex];
  const { request, send } = useWebSocket();

  // Fetch diff
  useEffect(() => {
    if (pr === undefined) return;

    setDiffLoading(true);
    setDiffError(null);

    request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .then((data) => {
        const payload = data as { diff?: string };
        setDiff(payload.diff ?? '');
        setDiffLoading(false);
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? err.message : 'Unknown error';
        setDiffError(message);
        setDiffLoading(false);
      });
  }, [selectedIndex, request, pr]);

  // Fetch comments
  useEffect(() => {
    if (pr === undefined) return;

    request('REQUEST_COMMENTS', { pr_id: pr.id })
      .then((data) => {
        const payload = data as { comments?: Comment[] };
        setComments(payload.comments ?? []);
      })
      .catch((err: unknown) => {
        console.error('Failed to fetch comments:', err);
      });
  }, [selectedIndex, request, pr]);

  // Handle 'c' key (comment on line) — watch confirmingAction
  useEffect(() => {
    if (confirmingAction === 'comment') {
      // Open comment input at selected file/line 0 (user clicks a line to set target)
      setCommentTarget({ fileIndex: 0, lineNumber: 1 });
      appStore.setState((s) => ({ ...s, confirmingAction: null }));
    }
  }, [confirmingAction]);

  const handleLineClick = useCallback((fileIndex: number, lineNumber: number) => {
    setCommentTarget({ fileIndex, lineNumber });
  }, []);

  const handleCommentSubmit = useCallback((body: string) => {
    if (pr === undefined || commentTarget === null) return;

    send('POST_COMMENT', {
      pr_id: pr.id,
      body,
      line: commentTarget.lineNumber,
    });

    // Optimistic update
    const optimisticComment: Comment = {
      id: `optimistic-${Date.now()}`,
      pr_id: pr.id,
      author: 'you',
      body,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      line: commentTarget.lineNumber,
    };
    setComments((prev) => [...prev, optimisticComment]);
    setCommentTarget(null);
  }, [pr, commentTarget, send]);

  const handleCommentCancel = useCallback(() => {
    setCommentTarget(null);
  }, []);

  return (
    <MainLayout
      toolbar={
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm text-on-surface-variant">
            PR #{pr?.number ?? selectedIndex + 1}
          </span>
          {pr !== undefined && (
            <span className="text-sm text-on-surface truncate max-w-xs">
              {pr.title}
            </span>
          )}
          {pr !== undefined && (
            <span className="text-xs text-on-surface-variant font-mono ml-auto">
              {pr.author} · {pr.state}
              {pr.review_status && ` · ${pr.review_status}`}
            </span>
          )}
        </div>
      }
    >
      <div className="flex flex-col h-full">
        {isLoading && (
          <p className="text-sm text-on-surface-variant p-4">Loading PR details...</p>
        )}
        {error !== null && (
          <p className="text-sm text-error p-4">
            Failed to load PR: {(error as Error).message}
          </p>
        )}
        {diffLoading && (
          <p className="text-sm text-on-surface-variant p-4">Loading diff...</p>
        )}
        {diffError !== null && (
          <p className="text-sm text-error p-4">Failed to load diff: {diffError}</p>
        )}

        <div className="flex-1 min-h-0">
          <DiffViewer
            diff={diff}
            className="h-full"
            comments={comments}
            onLineClick={handleLineClick}
          />
        </div>

        {commentTarget !== null && (
          <CommentInput
            fileIndex={commentTarget.fileIndex}
            lineNumber={commentTarget.lineNumber}
            onSubmit={handleCommentSubmit}
            onCancel={handleCommentCancel}
          />
        )}
      </div>
    </MainLayout>
  );
}
```

- [ ] **Step 3: Update PRDetailPage test if needed**

Read and fix any broken tests due to the new props/structure.

- [ ] **Step 4: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/pages/PRDetailPage.tsx
git commit -m "feat: PRDetailPage fetches comments, inline comment input, optimistic updates"
```

---

## Task 8: Full build and test verification

**Files:**
- All modified files from Tasks 1-7

- [ ] **Step 1: Run TypeScript build**

Run: `cd electron && pnpm build`
Expected: Clean build, no errors

- [ ] **Step 2: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All tests pass (177+ existing + new tests)

- [ ] **Step 3: Verify Go build still passes**

Run: `cd /Users/samuelasvanyi/dev/bistro/go && go build ./...`
Expected: Clean

- [ ] **Step 4: Fix any issues found**

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: Phase 4 diff viewer & comments complete"
```

---

## Reference Files

| File | Purpose |
|------|---------|
| `design/ui-ux-principles.md` | UI design spec (pages 11-12 cover diff viewer & comments) |
| `design/data-flow.md` | Data flow including diff rendering |
| `design/api-contracts.md` | WebSocket message types (REQUEST_DIFF, REQUEST_COMMENTS, POST_COMMENT) |
| `docs/IMPLEMENTATION_CHECKLIST.md` | Phase 4 checklist items |
| `src/renderer/utils/diff-parser.ts` | Diff parser (extend for multi-file) |
| `src/renderer/components/DiffViewer.tsx` | Existing diff viewer (rewrite) |
| `src/renderer/pages/PRDetailPage.tsx` | Detail page (wire comments) |
| `src/renderer/hooks/useWebSocket.ts` | WebSocket client (send/request) |
| `src/renderer/hooks/useWebSocketComments.ts` | Comment hooks (already exist) |
| `src/renderer/types/index.ts` | Comment type definition |

## Important Constraints

1. **Immutability**: ALL store/state updates must be immutable
2. **highlight.js**: Use tree-shaken imports (`highlight.js/lib/core` + individual languages) to keep bundle small
3. **Optimistic updates**: Comments appear immediately in UI, rollback on error
4. **dangerouslySetInnerHTML**: Only used for highlight.js output (which escapes user input)
5. **All 177 existing tests must pass** after each task
6. **Build must compile** with zero TypeScript errors
7. **Commit after each task** with conventional commit message
