# Big PRs That Open and Scroll Smoothly Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open and fast-scroll 1,000+ file PRs (Marlin #28644, 2,598 files) with no blank files and no main-thread task over 100 ms in a production build.

**Architecture:** Four independent fixes. Go drops the 4 MB per-PR patch budget so no file is blanked for budget reasons. The renderer stops whole-tree re-renders: a memoized line count in the status bar, and a virtualized, memoized file sidebar. Shiki highlighting moves to @pierre/diffs' worker pool through a Vite `?worker` import.

**Tech Stack:** Go 1.26 + go-git; Electron + Vite 8.3.4 + React 19; `@pierre/diffs` 1.5.2; `@tanstack/react-virtual` ^3.14.15; `@thourum/fault`; Vitest; Biome.

**Spec:** `docs/superpowers/specs/2026-10-23-big-pr-scroll-design.md`

## Global Constraints

- Keep the 512 KB per-file cap (`DefaultMaxFilePatchBytes`). **Spec correction:** files over that cap get `diff_status = 'truncated'` (see `renderDiffFile`), not `'too_large'`. `'too_large'` came only from the PR budget that this plan removes. Older synced PRs can still have `'too_large'` rows in SQLite until they resync, so the renderer placeholder must handle **both** `'truncated'` and `'too_large'`. Keep `'too_large'` in the zod enum and in the DB migration comments.
- Placeholder copy: `File too large (N KB)`, with a button `o — open on GitHub`. It uses the existing `o` binding (`OPEN_FILE_BROWSER_EVENT` in `DiffView.tsx`).
- Electron TS: use `@thourum/fault` for errors. **No `try`/`catch`.** Wrap promises with `ResultAsync.fromPromise` (pattern: `utils/openUrl.ts`).
- Register no hotkeys outside `hooks/useAppHotkeys.ts`. Do not add new bindings.
- No React component tests (project memory). The tests are the Go regression test plus one utility test in `diffCodeViewItems.test.ts`.
- Use design tokens and utility classes that already exist in the touched files. No raw hex.
- pnpm commands run from `electron/`. Go commands run from `go/`.
- Verify before you declare a task done:
  - Go: `cd go && go vet ./... && go test -race ./...`
  - Electron: `cd electron && pnpm type-check && pnpm lint && pnpm test`
- Commit format: `<type>: <description>` (feat, fix, refactor, docs, test, chore, perf).

---

### Task 1: Remove the per-PR patch budget (Go)

**Files:**
- Modify: `go/internal/git/types.go` (status consts, budget consts, `DiffOptions`, `DefaultDiffOptions`, `DiffProfile.TooLargeFiles`)
- Modify: `go/internal/git/diff.go:113-180` (`renderDiffFiles`, `renderDiffFile`)
- Modify: `go/internal/git/diff_test.go` (replace `TestGenerateDiff_PRBudget`, update `TestGenerateDiff_PerFileBudget` / `TestGenerateDiff_NoBudget` option literals)
- Modify: `go/internal/store/migrations/00025_add_pr_diffs_diff_status.sql` is **not** touched (migrations are immutable). `go/internal/store/diff_store.go:30` comment stays, because legacy rows exist.

**Interfaces:**
- Produces: `DiffOptions{ MaxFilePatchBytes int64 }` (one field). `DefaultDiffOptions()` returns `{MaxFilePatchBytes: DefaultMaxFilePatchBytes}`. `DiffStatusTooLarge` and `DiffProfile.TooLargeFiles` are deleted.

- [ ] **Step 1: Write the failing regression test**

In `go/internal/git/diff_test.go`, delete `TestGenerateDiff_PRBudget` (lines ~342-383). Add this test in its place (`fmt` and `strings` are already imported, so check the import block):

```go
// Regression: a 4 MB per-PR budget used to blank every file after the first
// ~4 MB of patch, so large PRs rendered empty bodies past that point.
func TestGenerateDiff_LargePRKeepsEveryFile(t *testing.T) {
	const fileCount = 1000
	// ~5 KB patch per file → ~5 MB total, above the old 4 MB PR budget.
	body := strings.Repeat("// changed line\n", 320)
	base := make(map[string][]byte, fileCount)
	head := make(map[string][]byte, fileCount)
	for i := range fileCount {
		name := fmt.Sprintf("src/file_%04d.go", i)
		base[name] = []byte("package main\n")
		head[name] = []byte("package main\n" + body)
	}
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t, base, head)

	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, DefaultDiffOptions())
	if err != nil {
		t.Fatalf("GenerateDiff: %v", err)
	}
	if len(result.Files) != fileCount {
		t.Fatalf("expected %d files, got %d", fileCount, len(result.Files))
	}
	if result.Size <= 4*1024*1024 {
		t.Fatalf("fixture too small to cover the old budget: %d bytes", result.Size)
	}
	for _, f := range result.Files {
		if f.DiffStatus != DiffStatusOK || f.Patch == "" {
			t.Fatalf("%s: diff_status=%q patch=%d bytes, want ok with patch", f.Path, f.DiffStatus, len(f.Patch))
		}
	}
}
```

- [ ] **Step 2: Run it and confirm it fails on the old budget**

Run: `cd go && go test ./internal/git/ -run TestGenerateDiff_LargePRKeepsEveryFile -v`
Expected: FAIL with `diff_status="too_large"` for a file near the end.

- [ ] **Step 3: Remove the budget**

`go/internal/git/types.go`: delete the `DiffStatusTooLarge` const, `DefaultMaxTotalPatchBytes`, the `MaxTotalPatchBytes` field, its line in `DefaultDiffOptions()`, and `DiffProfile.TooLargeFiles`. Update the comments. The result looks like this:

```go
// Diff status values — mirrors the diff_status column in pr_diffs. Rows
// written before the per-PR budget was removed may still hold "too_large".
const (
	DiffStatusOK        = "ok"        // full patch stored
	DiffStatusTruncated = "truncated" // per-file budget exceeded; patch empty
)

// DefaultMaxFilePatchBytes caps one file's stored patch (safe for all SQLite page sizes).
const DefaultMaxFilePatchBytes int64 = 512 * 1024

// DiffOptions controls budget limits for GenerateDiff. A zero value means
// "no limit"; the sync path should always pass DefaultDiffOptions().
type DiffOptions struct {
	MaxFilePatchBytes int64 // 0 = unlimited
}

// DefaultDiffOptions returns the production-safe budget defaults.
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{MaxFilePatchBytes: DefaultMaxFilePatchBytes}
}
```

Also change the `DiffFile.DiffStatus` field comment to `// "ok" or "truncated"`.

`go/internal/git/diff.go`:
- `renderDiffFiles`: delete the `case diffFile.DiffStatus == DiffStatusTooLarge:` arm. Call `renderDiffFile(filePatch, opts)` (no `result.Size` argument).
- `renderDiffFile`: drop the `storedBytes int64` parameter and the `opts.MaxTotalPatchBytes` case. Set `Truncated: diffStatus == DiffStatusTruncated`.

Fix the remaining literals in `diff_test.go`:
- `DiffOptions{MaxFilePatchBytes: 50, MaxTotalPatchBytes: 0}` → `DiffOptions{MaxFilePatchBytes: 50}`

Then grep for stragglers, which must return nothing outside comments and migrations:

```bash
cd go && grep -rn 'MaxTotalPatchBytes\|DiffStatusTooLarge\|TooLargeFiles' .
```

- [ ] **Step 4: Run the tests**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS. The new test is the slowest in the package, because it commits 1,000 files. That is fine.

- [ ] **Step 5: Commit**

```bash
git add go/internal/git/
git commit -m "fix(engine): drop the 4 MB per-PR patch budget so large PRs keep every file"
```

---

### Task 2: Show a placeholder for oversized files (renderer)

Today an oversized file arrives as a header-only patch (`diff --git a/x b/x\n`, added by `GetUnifiedDiff` in `go/internal/store/diff_store.go`). It renders as a blank body. This task classifies such a file as unrenderable, the same way binary files are handled. Selecting the file (sidebar or `Shift+J`/`Shift+K`) then shows a panel instead of CodeView.

**Files:**
- Modify: `electron/src/renderer/components/diff/diffCodeViewItems.ts` (`UnrenderableDiffFile`, `parseCodeViewFiles`)
- Modify: `electron/src/renderer/components/diff/diffParseFailurePanel.tsx`
- Modify: `electron/src/renderer/components/diff/DiffView.tsx` (new prop, pass to `useDiffViewData`)
- Modify: `electron/src/renderer/pages/DiffPage.tsx` (build the map from `files`)
- Test: `electron/src/renderer/components/diff/diffCodeViewItems.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export type UnrenderableDiffFile =
    | { file: ParsedDiffFile; reason: 'parse-failed' | 'binary' }
    | { file: ParsedDiffFile; reason: 'too-large'; patchBytes: number };
  export function parseCodeViewFiles(
    files: readonly ParsedDiffFile[],
    oversizedPatchBytes?: ReadonlyMap<string, number>,
  ): CodeViewFileParseResult;
  ```
  The key of `oversizedPatchBytes` is the file path (`DiffFile.path` === `ParsedDiffFile.path`). The value is `patch_bytes`.
- `DiffViewProps` gains `oversizedPatchBytes?: ReadonlyMap<string, number>`.

- [ ] **Step 1: Write the failing test**

Append this to the `describe('parseCodeViewFiles', ...)` block in `diffCodeViewItems.test.ts`:

```ts
  it('classifies an oversized file as too-large with its original size', () => {
    const headerOnly = `diff --git a/huge.c b/huge.c
diff --git a/src/first.ts b/src/first.ts
--- a/src/first.ts
+++ b/src/first.ts
@@ -1 +1 @@
-old
+new
`;
    const { parsedFiles, unrenderableFiles } = parseCodeViewFiles(
      splitUnifiedDiff(headerOnly),
      new Map([['huge.c', 600 * 1024]]),
    );

    expect(unrenderableFiles).toEqual([
      expect.objectContaining({ reason: 'too-large', patchBytes: 600 * 1024 }),
    ]);
    expect(unrenderableFiles[0]?.file.path).toBe('huge.c');
    expect(parsedFiles.map(({ file }) => file.path)).toEqual(['src/first.ts']);
  });
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `cd electron && pnpm exec vitest run src/renderer/components/diff/diffCodeViewItems.test.ts`
Expected: FAIL, because `unrenderableFiles` is empty or has the wrong reason.

- [ ] **Step 3: Implement the classification**

In `diffCodeViewItems.ts`, replace the `UnrenderableDiffFile` interface with the union above, then update `parseCodeViewFiles`:

```ts
const NO_OVERSIZED_FILES: ReadonlyMap<string, number> = new Map();

export function parseCodeViewFiles(
  files: readonly ParsedDiffFile[],
  oversizedPatchBytes: ReadonlyMap<string, number> = NO_OVERSIZED_FILES,
): CodeViewFileParseResult {
  const parsedFiles: ParsedCodeViewFile[] = [];
  const unrenderableFiles: UnrenderableDiffFile[] = [];
  for (const file of files) {
    // The engine stores no patch for files over its per-file cap.
    const patchBytes = oversizedPatchBytes.get(file.path);
    if (patchBytes !== undefined) {
      unrenderableFiles.push({ file, reason: 'too-large', patchBytes });
      continue;
    }
    // Binary patches parse "successfully" into zero hunks, so classify first.
    const isBinary = isBinaryPatch(file.patch);
    // ...rest unchanged
```

- [ ] **Step 4: Run the test to confirm it passes**

Run: `cd electron && pnpm exec vitest run src/renderer/components/diff/diffCodeViewItems.test.ts`
Expected: PASS (all cases).

- [ ] **Step 5: Render the panel and wire the data**

`diffParseFailurePanel.tsx`: add a `too-large` branch. It uses the neutral tone that binary files use, and its button calls the same `onOpenInBrowser`:

```tsx
const BYTES_PER_KB = 1024;

export function DiffParseFailurePanel({ file, onOpenInBrowser }: { ... }): React.ReactElement {
  const isNeutral = file.reason !== 'parse-failed';
  const tone = isNeutral
    ? 'border-outline-variant/30 bg-surface-container text-outline'
    : 'border-error/30 bg-error/10 text-error';
  return (
    <section className={`border-b px-4 py-3 font-mono text-[10px] ${tone}`} data-unrenderable-diff-file={file.file.id}>
      <p className="uppercase tracking-widest">{panelTitle(file)}</p>
      <p className="mt-1 text-outline">
        {isNeutral ? file.file.path : 'The patch could not be parsed by the diff renderer.'}
      </p>
      {isNeutral && onOpenInBrowser !== undefined ? (
        <button type="button" onClick={onOpenInBrowser} className="mt-2 uppercase tracking-widest text-primary hover:underline">
          {file.reason === 'too-large' ? 'o — open on GitHub' : 'o — open in browser'}
        </button>
      ) : null}
    </section>
  );
}

function panelTitle(file: UnrenderableDiffFile): string {
  if (file.reason === 'too-large')
    return `File too large (${Math.round(file.patchBytes / BYTES_PER_KB)} KB)`;
  return file.reason === 'binary' ? 'Binary file' : `Unable to render ${file.file.path}`;
}
```

Update the doc comment so it lists oversized files as well.

`DiffView.tsx`:
- Add `oversizedPatchBytes?: ReadonlyMap<string, number>` to `DiffViewProps`, with the doc comment `/** Engine-capped files (no stored patch), keyed by path → original patch bytes. */`. Default it to a module const `EMPTY_OVERSIZED: ReadonlyMap<string, number> = new Map()`.
- Pass it into `useDiffViewData` (add it to `DiffViewDataOptions`). Change the memo to `parseCodeViewFiles(files, oversizedPatchBytes)` with deps `[files, oversizedPatchBytes]`.

`DiffPage.tsx`, next to `viewedPaths`:

```tsx
  // 'too_large' only appears on rows synced before the per-PR budget was removed.
  const oversizedPatchBytes = useMemo(
    () =>
      new Map(
        files
          .filter((f) => !f.is_binary && f.diff_status !== 'ok')
          .map((f) => [f.path, f.patch_bytes]),
      ),
    [files],
  );
```

Pass `oversizedPatchBytes={oversizedPatchBytes}` to `<DiffView>`.

- [ ] **Step 6: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. If `pnpm lint` reports formatting only, run `pnpm exec biome check --write src` and re-run.

- [ ] **Step 7: Commit**

```bash
git add electron/src/renderer/components/diff/ electron/src/renderer/pages/DiffPage.tsx
git commit -m "feat(diff): show a placeholder for files over the engine's per-file cap"
```

---

### Task 3: Stop whole-tree re-renders (status bar + sidebar)

**Files:**
- Modify: `electron/src/renderer/pages/DiffPageChrome.tsx:81-96` (`DiffPageStatusBar`)
- Modify: `electron/src/renderer/components/diff/diffFileSidebar.tsx` (full rewrite of the body)
- Modify: `electron/src/renderer/components/diff/diffViewNavigation.ts:208-222` (stable `selectFile`)

**Interfaces:**
- Consumes: `DiffFileSidebar` props are unchanged (`files`, `activeId`, `onSelect`, `totalCount`, `viewedPaths`).
- Produces: `selectFile` from `useDiffFileNavigation` has a stable identity while `files` and `unrenderableFiles` are unchanged. Row memoization depends on this.

- [ ] **Step 1: Memoize the status-bar line count**

In `DiffPageStatusBar` (`DiffPageChrome.tsx`), add `useMemo` to the `react` import. Then:

```tsx
  // The full diff can be ~19 MB; split it once per diff, never per render.
  const diffLineCount = useMemo(() => diff.split('\n').length, [diff]);
  const rangeLineCount = useMemo(
    () => (rangeResult === null ? 0 : rangeResult.raw.split('\n').length),
    [rangeResult],
  );
```

Render `` `${showRangeDiff && rangeResult !== null ? rangeLineCount : diffLineCount} lines` ``. Check how `rangeResult` is typed in `DiffPageStatusBarProps` first. If it is `RangeDiffResult | null`, the code above is correct as written.

- [ ] **Step 2: Make `selectFile` stable**

In `diffViewNavigation.ts`, add `useCallback` to the react import and wrap `selectFile`:

```ts
  const selectFile = useCallback(
    (id: string): void => {
      setActiveId(id);
      requestSelectedFileScroll(
        codeViewRef.current,
        pendingScrollFileIdRef,
        id,
        unrenderableFiles,
      );
      updateSelectedFileIndex(files, id);
    },
    [files, unrenderableFiles],
  );
```

`codeViewRef`/`pendingScrollFileIdRef` are refs and `setActiveId` is a state setter, so all three are stable. If Biome's `useExhaustiveDependencies` asks for them, add them. Check that `updateSelectedFileIndex` and `requestSelectedFileScroll` are module-level functions, not closures. If either is a closure, add it to the deps.

- [ ] **Step 3: Virtualize and memoize the sidebar**

Rewrite `diffFileSidebar.tsx`. The header moves out of the scroll element, so the virtualizer measures offsets from the top of the list (no `scrollMargin` needed). Each row has a fixed height, so `estimateSize` is exact:

```tsx
import { useVirtualizer } from '@tanstack/react-virtual';
import { memo, useEffect, useRef } from 'react';
import type { ParsedDiffFile } from './diffPathUtils';

interface DiffFileSidebarProps {
  files: readonly ParsedDiffFile[];
  activeId: string | null;
  onSelect: (id: string) => void;
  totalCount: number;
  /** Paths marked viewed (`v`) for the current user. Empty when untracked. */
  viewedPaths?: ReadonlySet<string>;
}

// 28px button (py-1.5 + 16px line) + 4px gap; rows must stay fixed-height.
const ROW_HEIGHT_PX = 32;
// Rows rendered beyond the viewport so fast scrolls don't flash blank.
const OVERSCAN_ROWS = 20;

export function DiffFileSidebar({
  files,
  activeId,
  onSelect,
  totalCount,
  viewedPaths = EMPTY_VIEWED_PATHS,
}: DiffFileSidebarProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: files.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: OVERSCAN_ROWS,
  });

  // Keyboard file navigation (Shift+J/K) and scroll-driven activation move
  // activeId; keep its row in view.
  useEffect(() => {
    const index = files.findIndex((file) => file.id === activeId);
    if (index >= 0) virtualizer.scrollToIndex(index, { align: 'auto' });
  }, [activeId, files, virtualizer]);

  return (
    <aside className="w-1/4 min-w-[220px] bg-surface-container-low border-r border-outline-variant/20 flex flex-col min-h-0">
      <h3 className="px-4 pt-4 font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-4">
        Files Changed ({totalCount.toString().padStart(2, '0')})
      </h3>
      <div ref={scrollRef} className="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
        <nav className="relative" style={{ height: virtualizer.getTotalSize() }}>
          {virtualizer.getVirtualItems().map((row) => {
            const file = files[row.index];
            if (file === undefined) return null;
            return (
              <div
                key={file.id}
                className="absolute left-0 right-0 pb-1"
                style={{ top: row.start, height: ROW_HEIGHT_PX }}
              >
                <DiffFileRow
                  file={file}
                  isActive={file.id === activeId}
                  isViewed={viewedPaths.has(file.path)}
                  onSelect={onSelect}
                />
              </div>
            );
          })}
        </nav>
      </div>
    </aside>
  );
}

interface DiffFileRowProps {
  file: ParsedDiffFile;
  isActive: boolean;
  isViewed: boolean;
  onSelect: (id: string) => void;
}

const DiffFileRow = memo(function DiffFileRow({
  file,
  isActive,
  isViewed,
  onSelect,
}: DiffFileRowProps) {
  return (
    <button
      type="button"
      onClick={() => onSelect(file.id)}
      className={`w-full h-full flex items-center gap-2 px-2 text-left cursor-crosshair transition-colors duration-150 ${
        isActive
          ? 'bg-surface-container-lowest text-primary border-l-2 border-primary'
          : 'hover:bg-surface-container-highest/50 text-on-surface border-l-2 border-transparent hover:border-primary/40'
      }`}
    >
      {/* icon + path + stats spans: copy verbatim from the current file */}
    </button>
  );
});

const EMPTY_VIEWED_PATHS: ReadonlySet<string> = new Set();
```

Copy the three inner `<span>`s (the icon, the path, `+a/-d`) **verbatim** from the current `diffFileSidebar.tsx`. Only the wrapper changes: the outer `button` replaces `py-1.5` with `h-full`, because the wrapper fixes the height. Biome may warn that `virtualizer` is unstable in the effect deps. `useVirtualizer` returns a stable instance, so keep the dep.

- [ ] **Step 4: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

Manual check in `cd electron && pnpm start`. Open any PR diff:
- (a) The sidebar scrolls, and every row looks the same as before (icon, path, stats, active border).
- (b) `Shift+J` held down moves the active row, and the sidebar follows it.
- (c) Clicking a row scrolls the diff to that file.
- (d) Scrolling the diff updates the active row.
- (e) Typing in the toolbar find box filters the sidebar.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/pages/DiffPageChrome.tsx electron/src/renderer/components/diff/
git commit -m "perf(diff): virtualize the file sidebar and memoize the status-bar line count"
```

---

### Task 4: Highlight in @pierre/diffs' worker pool

**Background:** `@pierre/diffs` 1.5.2 exports `WorkerPoolContextProvider` and `useWorkerPool` from `@pierre/diffs/react`. It ships the worker entry at `@pierre/diffs/worker/worker.js`, which is an ES module with a dynamic `import()` of shiki's wasm. The pool is a singleton (`getOrCreateWorkerPoolSingleton`), so `highlighterOptions` apply only once, at creation. A theme change must therefore go through `pool.setRenderOptions({ theme })`. Worker-owned options (`theme`, `lineDiffType`, `tokenizeMaxLineLength`) override per-component options. If workers fail to start, the pool stops them and highlights on the main thread, so a worker failure degrades performance but does not break rendering. Type references: `node_modules/@pierre/diffs/dist/react/WorkerPoolContext.d.ts` and `node_modules/@pierre/diffs/dist/worker/types.d.ts` (in the repo-root `node_modules`).

**Files:**
- Modify: `electron/vite.renderer.config.ts` (add `worker.format`)
- Modify: `electron/src/renderer/components/diff/DiffView.tsx` (provider, theme sync, remove `disableWorkerPool`, `tokenizeMaxLineLength`)
- Modify: `docs/superpowers/plans/2026-07-18-codeview-spike-notes.md` (lines 85, 117, 160 describe the deferral)

**Interfaces:**
- Consumes: none from earlier tasks. It touches the same `DiffView.tsx` as Task 2, so re-read the file before you edit.

- [ ] **Step 1: Emit ES-module workers**

The worker contains a dynamic `import()`. Vite's default `iife` worker format cannot code-split, so the build would fail. In `electron/vite.renderer.config.ts`, add this to the `defineConfig` object:

```ts
  // @pierre/diffs' highlighter worker lazy-imports shiki's wasm; only ES
  // workers can code-split.
  worker: { format: 'es' },
```

- [ ] **Step 2: Provide the pool in DiffView**

At the top of `DiffView.tsx`:

```tsx
import {
  CodeView,
  type CodeViewHandle,
  useWorkerPool,
  WorkerPoolContextProvider,
} from '@pierre/diffs/react';
import DiffsHighlighterWorker from '@pierre/diffs/worker/worker.js?worker';
import { Fault, ResultAsync } from '@thourum/fault';
```

`@thourum/fault` may already be imported. Merge the imports rather than duplicating them. Check that `WorkerPoolOptions` is exported from `@pierre/diffs/react`, which its `.d.ts` re-exports.

Then add the module consts, next to the other module consts:

```tsx
// Lines longer than this render unhighlighted, so a minified bundle can't
// pin a worker. 1,000 chars covers hand-written code with margin.
const TOKENIZE_MAX_LINE_LENGTH = 1000;
const LINE_DIFF_TYPE = 'word';

const WORKER_POOL_OPTIONS: WorkerPoolOptions = {
  workerFactory: () => new DiffsHighlighterWorker(),
  // Pool stops workers and falls back to main-thread highlighting on failure.
  onWorkerError: (event) => {
    event.preventDefault();
    log.error({ error: event instanceof ErrorEvent ? event.message : event.type }, 'diff highlighter worker failed');
  },
};
```

In `DiffView`'s non-empty return, wrap the outermost `<div className="w-full h-full min-h-0 flex overflow-hidden">` in this provider:

```tsx
    <WorkerPoolContextProvider
      poolOptions={WORKER_POOL_OPTIONS}
      highlighterOptions={{
        theme: `pierre-${theme}`,
        lineDiffType: LINE_DIFF_TYPE,
        tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH,
      }}
    >
      ...existing tree...
    </WorkerPoolContextProvider>
```

In `DiffCodeView`, which renders inside the provider, keep the singleton pool's theme in sync with the app theme:

```tsx
  const workerPool = useWorkerPool();
  useEffect(() => {
    if (workerPool === undefined) return;
    void ResultAsync.fromPromise(
      workerPool.setRenderOptions({ theme: `pierre-${theme}` }),
      (cause) =>
        new Fault(cause instanceof Error ? cause.message : String(cause))
          .withTag('DIFF_THEME_ERROR')
          .withCause(cause),
    ).orInspect((error) =>
      log.error({ error: error.message }, 'failed to switch diff theme'),
    );
  }, [workerPool, theme]);
```

On the `<CodeView>`:
- Delete the `disableWorkerPool` prop.
- In `options`, use `lineDiffType: LINE_DIFF_TYPE` and add `tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH`. This keeps the main-thread fallback path bounded as well. If the `options` type rejects `tokenizeMaxLineLength`, drop it there. The provider still sets it for workers.

`.orInspect` is the method `DiffView.tsx` already uses on `openUrlInBrowser`. If `ResultAsync.fromPromise` lacks it, use the same terminal call as `openFilesInBrowser` in this file.

If any test imports `DiffView.tsx`, it now pulls in a `?worker` import that Vitest cannot resolve. Check with `grep -rn "DiffView'" electron/src --include='*.test.*'`. It should return nothing, because no component tests exist.

- [ ] **Step 3: Verify the type-check, lint, and tests**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. `vite/client` types (set in `tsconfig.renderer.json` `types`) declare `*?worker` modules.

- [ ] **Step 4: Verify dev mode**

Run: `cd electron && pnpm start`, then open a PR diff. Check all of the following:
- (a) Code is syntax-highlighted.
- (b) DevTools → Sources → Threads lists the `DiffsHighlighterWorker` workers.
- (c) The console shows no `diff highlighter worker failed` errors.
- (d) Switch the app theme between light and dark in Settings, return to the diff, and confirm that the highlight colors follow.

- [ ] **Step 5: Verify the packaged build**

Run: `cd electron && pnpm run package`. Launch `out/Bistro-darwin-arm64/Bistro.app`, open a PR diff, and check items (a)–(c) again. Open DevTools with the app's existing shortcut, or check the renderer log.

The packaged renderer loads over `file://` (`main/window.ts` `loadFile`). If module workers fail there, the console shows `diff highlighter worker failed` and highlighting still works on the main thread. **Contingency:** change the import to `@pierre/diffs/worker/worker-portable.js?worker&inline` (a blob-URL worker), re-run `pnpm run package`, and recheck. Use the contingency only if the first approach fails, and record which one shipped in the commit message.

- [ ] **Step 6: Update the spike notes**

In `docs/superpowers/plans/2026-07-18-codeview-spike-notes.md`, add one short paragraph after the table row at line 85. Leave the historical rows as they are:

```markdown
> **Update 2026-10-23:** The worker pool shipped (`docs/superpowers/plans/2026-10-23-big-pr-scroll.md` Task 4). `DiffView` wraps CodeView in `WorkerPoolContextProvider` with a Vite `?worker` factory (`worker.format: 'es'` in `vite.renderer.config.ts`). Worker-owned options (`theme`, `lineDiffType`, `tokenizeMaxLineLength`) are set on the provider. Theme changes go through `useWorkerPool().setRenderOptions`, because the pool is a singleton.
```

- [ ] **Step 7: Commit**

```bash
git add electron/vite.renderer.config.ts electron/src/renderer/components/diff/DiffView.tsx docs/superpowers/plans/2026-07-18-codeview-spike-notes.md
git commit -m "perf(diff): highlight in @pierre/diffs' worker pool instead of the main thread"
```

---

### Task 5: Log diff-open timing, then check acceptance

**Files:**
- Modify: `electron/src/renderer/pages/DiffPage.tsx`

**Interfaces:**
- Consumes: `diff`, `files`, and `prId`, which already exist in `DiffPage`. Task 2 also edits `DiffPage.tsx`, so re-read the file before you edit.

- [ ] **Step 1: Add the log**

In `DiffPage.tsx`:
- Add `useEffect` and `useRef` to the react import.
- Import `createLogger` from `'../utils/logger'`.
- Add `const log = createLogger('DiffPage');` at module level.

Inside `DiffPage`, record when navigation to a PR started by extending the existing `useLayoutEffect` on `[prId]`:

```tsx
  // Start of the diff-open measurement; reset whenever the route's PR changes.
  const openedAtRef = useRef(performance.now());
  useLayoutEffect(() => {
    openedAtRef.current = performance.now();
    if (prId === '') return;
    appStore.setState((s) => ({ ...s, diffFileKeys: [] }));
  }, [prId]);
```

That replaces the existing effect body; keep its comment. Then add the log:

```tsx
  // One line per diff open so big-PR regressions show up in user logs.
  // rAF after commit ≈ the first file's first paint.
  useEffect(() => {
    if (diff === '') return;
    const frame = requestAnimationFrame(() =>
      log.debug(
        {
          prId,
          fileCount: files.length,
          payloadChars: diff.length,
          firstFileMs: Math.round(performance.now() - openedAtRef.current),
        },
        'diff opened',
      ),
    );
    return () => cancelAnimationFrame(frame);
  }, [prId, diff, files.length]);
```

Call the field `payloadChars` and not bytes, because `diff.length` counts UTF-16 units. That count equals the byte count for ASCII diffs, so it is close enough to spot regressions without encoding 19 MB just to log it.

- [ ] **Step 2: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. Then run `pnpm start` with `LOG_LEVEL=debug` in `electron/.env.local` (check `.env.example` for the exact key). Open a diff and confirm that exactly one `diff opened` line appears, with a non-zero `firstFileMs`.

- [ ] **Step 3: Commit**

```bash
git add electron/src/renderer/pages/DiffPage.tsx
git commit -m "feat(diff): log file count, payload size, and time-to-first-file on diff open"
```

- [ ] **Step 4: Acceptance (manual, by the user)**

This step needs a signed-in packaged app and a Chrome performance trace, so the controller does not run it. Hand it to the user:

1. `cd electron && pnpm run package`, then launch `out/Bistro-darwin-arm64/Bistro.app`.
2. Open MarlinFirmware/Marlin#28644 and resync it, because the old rows carry `too_large`. The `diff opened` log should show `firstFileMs` ≤ 1000.
3. Files near the end of the PR render normally, without blank bodies.
4. In DevTools → Performance, record a fast top-to-bottom scroll. No main-thread task exceeds 100 ms.
