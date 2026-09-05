# CodeView Verification Spike Notes

**Date:** 2026-07-18

**Dependency checked:** `@pierre/diffs` 1.2.11

**Decision:** **GO — human-verified in an inspectable renderer.**

## Recommendation

Begin Task 9. The installed 1.2.11 API supports every needed migration primitive: stable mixed diff items, `DiffLineAnnotation` rendering, live unified/split options, one native scroll surface, and imperative item and line scroll targets. A disposable jsdom mount confirmed that both additions- and deletions-side annotations render in both layouts, the custom per-file header hook renders, and a 1,000-item mount initially produced only two custom headers.

Human-driven verification then exercised the temporary 1,000-file fixture in the Vite browser renderer and Electron shell. Keyboard shortcuts worked; the item-900 target landed visibly; the item-500 line-14 target and both annotations were exact; twenty rapid line targets settled cleanly at line 33; unified/split retained the rough viewport and annotation anchors; and free scrolling was smooth. The existing benchmark remains parser-only, so it cannot compare the current `PatchDiff` rendering path with CodeView rendering. It is a stable parsing baseline rather than a renderer-performance result.

## Sources inspected

- Library docs: <https://diffs.com/docs#codeview>
- Installed React declarations: `node_modules/@pierre/diffs/dist/react/CodeView.d.ts`
- Installed core declarations: `node_modules/@pierre/diffs/dist/components/CodeView.d.ts` and `node_modules/@pierre/diffs/dist/types.d.ts`
- Worker-pool declarations: `node_modules/@pierre/diffs/dist/react/WorkerPoolContext.d.ts`
- Patch parser: `node_modules/@pierre/diffs/dist/utils/parsePatchFiles.d.ts`
- Current surface: `electron/src/renderer/components/PierreDiffView.tsx`
- Current benchmark: `electron/scripts/bench-large-diff.mjs`

## Verified behavior

### Empirical scratch probe

A temporary, untracked jsdom + React mount used the installed runtime, parsed a real unified patch with `parsePatchFiles`, and rendered a controlled `CodeView` item with two annotations:

- addition-side annotation at line 3;
- deletion-side annotation at line 6;
- `renderAnnotation` output tagged by thread ID;
- `renderCustomHeader` output standing in for the current per-file expertise chrome;
- `options.diffStyle` changed from `unified` to `split` without replacing the item data;
- `CodeViewHandle.scrollTo` called with an item target and an additions-side line target;
- a 1,000-item controlled view using the same parsed diff.

| Probe assertion | Result |
| --- | --- |
| Parsed fixture is usable as `CodeViewDiffItem.fileDiff` | Pass: `src/large-demo.ts`, 40 unified / 20 split rows |
| Additions annotation renders in unified layout | Pass |
| Deletions annotation renders in unified layout | Pass |
| Both annotations survive live change to split layout | Pass |
| `renderCustomHeader` renders in both layouts | Pass |
| `scrollTo({ type: 'item', id, ... })` | API invocation completed without exception; final item position unverified in jsdom |
| `scrollTo({ type: 'line', id, lineNumber, side: 'additions', ... })` | API invocation completed without exception; final line position unverified in jsdom |
| Imperative handle | Includes `scrollTo`, `getItem`, `updateItem`, selection methods, and `updateItemId` |
| Native CodeView container | Pass: one scroll container |
| 1,000-item virtualized mount | Pass: two custom headers mounted after initial render |

The probe used jsdom geometry stubs, `disableWorkerPool`, and a non-layout engine. It proves the React API wiring and rendering callbacks, not pixel-perfect browser layout, worker throughput, or actual scroll position geometry.

### Read from installed 1.2.11 types and docs

The following was confirmed from the pinned runtime/types rather than browser interaction:

- Controlled ownership uses `items`; imperative ownership uses `initialItems`. They are intentionally mutually exclusive.
- A diff item is `{ id, type: 'diff', fileDiff, annotations?, version?, collapsed? }`. Its annotations are `DiffLineAnnotation<T>[]`.
- `DiffLineAnnotation<T>` supplies `side`, `lineNumber`, and generic `metadata`. Valid diff sides are `additions` and `deletions`.
- Stable IDs are the identity for reconciliation, selection, `getItem`, `updateItem`, and all scroll targets. With a matching ID and unchanged `version`, CodeView may retain the prior rendered record even when the caller supplies a new object; therefore derive `version` deterministically from all rendered file/annotation/card state (including reply/resolution state) so every visible change advances it.
- `options.diffStyle` accepts `unified` and `split`; `stickyHeaders`, `layout`, `itemMetrics`, and `pointerEventsOnScroll` are CodeView options.
- `renderAnnotation`, `renderCustomHeader`, `renderHeaderPrefix`, `renderHeaderMetadata`, and `renderGutterUtility` receive the complete `CodeViewItem` and can branch on `item.type`.
- `CodeViewHandle.scrollTo` accepts item, line, range, and position targets. An item target requires `id`; a line target requires `id` and `lineNumber`, with optional diff `side`, alignment, offset, and behavior.
- CodeView is designed as one virtualized scroll region. The docs describe item/line-size estimation and `__devOnlyValidateItemHeights` for development validation.
- `WorkerPoolContextProvider` is optional. When used, it requires `poolOptions` (including a `workerFactory`) and `highlighterOptions`; worker-owned render options override per-component values for theme, `lineDiffType`, `tokenizeMaxLineLength`, and `useTokenTransformer`.

## Current feature to CodeView mapping

| Current feature | Current location / behavior | CodeView mechanism and Task 9 guidance |
| --- | --- | --- |
| Multi-file virtualized diff surface | `PierreDiffView.tsx` uses TanStack Virtual around mounted `PatchDiff` blocks | Build controlled `CodeViewDiffItem<ReviewThread>[]`, one stable item per parsed file. Use `CodeView` as the sole diff canvas; remove the outer TanStack virtualizer. |
| Per-file patch parsing | Current parser returns file-level raw patch segments | Use `parsePatchFiles(file.patch)` and flatten each returned `ParsedPatch.files` result into diff items. Preserve the existing parsed-file model as the routing/sidebar authority; do not derive canonical keys from CodeView internals. |
| Thread-to-line mapping | Current thread with a current anchor maps to `{ side: 'additions', lineNumber: thread.line, metadata: thread }` | Preserve that exact eligibility/mapping as `CodeViewDiffItem.annotations`. Use `renderAnnotation(annotation, item)` to render `ThreadAnnotationCard`; its generic annotation metadata should be `ReviewThread`. |
| Thread card and inline composer | `ThreadAnnotationCard` needs its active/replying state, callbacks, and stable `data-annotation-key` DOM attribute | Reuse the card component unchanged under `renderAnnotation`. Keep the DOM attribute for active-card focus/scroll and retain its `Mod+Enter`, Escape, blur, and propagation handling. |
| Annotation keyboard navigation (`j` / `k`) | Existing flattened list selects a thread, scrolls file, then uses DOM `scrollIntoView` for the card | Keep the ordered thread list and store contract. First call `scrollTo({ type: 'item', id: fileId, align: 'nearest' })`; then retain DOM scrolling of `[data-annotation-key]` after the CodeView render frame. Both actual position and card timing need real-Electron validation because annotations affect measured height. |
| Generic diff `j` / `k` fallback | Keybinding scrolls `[data-diff-viewport]` by 40px | Preserve `data-diff-viewport` on CodeView's scroll root via `containerRef`, or update the binding to a CodeView-backed scroll adapter. Confirm the referenced root supports the current `scrollBy` bindings, including vertical, horizontal, page, Home, and End behavior; do not leave it targeting the removed TanStack container. |
| File navigation (`n` / `p`, sidebar, `?file`) | Canonical key resolves to an existing parsed-file `id`; current virtualizer scrolls the item | Use `handle.scrollTo({ type: 'item', id: fileId, align: 'start', behavior: 'instant' })`. The target type is accepted by the API; final position must be confirmed on a real PR. Keep existing parsed-file order and `onFilesResolved` canonical keys. |
| Active-file sync while manual scrolling | Current virtualizer finds the first visible row and updates `selectedFileIndex` | Use CodeView's `onScroll(scrollTop, viewer)` plus rendered item snapshot / viewer lookup to determine the first visible item. Confirm this in Electron: no scroll-jacking, and `n` / `p` start from the visible file. |
| Unified / split setting | Not yet shipped; Task 10 will add it | Pass the persisted setting as `options={{ ..., diffStyle }}`. The scratch mount verified annotations and custom headers remain rendered across a unified-to-split re-render. |
| Per-file header and expertise `formatWhy` content | Current wrapper header and `FileExpertiseLine` sit above each `PatchDiff` | Use `renderCustomHeader(item)` to render the current header plus `FileExpertiseLine`, looking up expertise from the stable item ID/path mapping. This hook rendered in the probe. `renderHeaderPrefix`/`renderHeaderMetadata` are alternatives if the standard library header is desired. |
| Sticky headers / spacing | Hand-built sticky wrapper header per virtual file | Start with `options.stickyHeaders: true` and `layout: { paddingTop: 0, paddingBottom: 0, gap: 0 }` so the host owns the behavior. Verify contrast, top offset, and spacing in Electron before styling changes. Note that `stickyHeaders` is CodeView-level; `stickyHeader` is a shared per-file/diff option. |
| Mark viewed (`v`) | `DiffPage` owns viewed state and updates `selectedFileIndex` | Leave in `DiffPage`. Preserve parsed-file order and selected-index synchronization; CodeView does not own GitHub/viewed mutations. |
| Since-last-review / range diff (`s`) and `DegradedRangeDiffIndicator` | `DiffPage` swaps in `RangeDiffView` rather than `PierreDiffView` | Leave untouched. CodeView only replaces the ordinary PR diff host. Confirm the range branch still renders its own `data-diff-viewport`. |
| Worker highlighting | Current view deliberately passes `disableWorkerPool` | Do not introduce a worker pool in Task 9 unless the renderer can provide a bundler-safe `workerFactory`. Keep `disableWorkerPool` initially to preserve behavior. If adding it later, centralize worker-owned render options in the provider. |

## Performance results

### Existing baseline: parser only

`pnpm bench:large-diff` was run before the spike. It warms once, parses seven samples per synthetic scenario, and reports the median and p95 of `parseMultiFileDiff`. It does not mount React, `PatchDiff`, CodeView, syntax highlighting, annotations, or a browser virtualizer.

| Scenario | Sections | Input size | Median | P95 |
| --- | ---: | ---: | ---: | ---: |
| 100 files × 20 changed lines | 100 | 130,799 B / 4,500 lines | 1.99 ms | 2.47 ms |
| 1,000 files × 10 changed lines | 1,000 | 727,999 B / 25,000 lines | 9.99 ms | 14.32 ms |
| 3,000 files × 4 changed lines | 3,000 | 1,202,999 B / 39,000 lines | 15.20 ms | 17.78 ms |
| 1 file × 10,000 changed lines | 1 | 655,756 B / 20,005 lines | 5.42 ms | 6.23 ms |

No CodeView “after” renderer number was collected: this script cannot produce one without changing the workload from parsing to renderer instrumentation. Because the eventual migration should preserve the parser path, its future parser figure is expected to remain within normal run-to-run variance, but that is a hypothesis rather than an observed comparison. The script is useful only for parser-regression detection, not for the Task 8 rendering-architecture gate.

### CodeView-oriented observation

The disposable mount rendered a 1,000-item CodeView from a parsed diff fixture in **72.98 ms** in jsdom, with **2 of 1,000** `renderCustomHeader` nodes mounted at the initial viewport. This is directional evidence of item virtualization only. It is not comparable to browser rendering time, and must not be treated as a performance budget or as a baseline versus the old path.

CodeView's documentation states that it owns a single virtual code surface, renders visible lines in hunk-sized batches, estimates/corrects item heights, and is generally more performant than lower-level flexible layouts where top-level file/diff containers stay mounted. This supports the migration design, but is not a renderer-performance result and cannot satisfy the Task 8 gate.

## Risks and mitigations

| Risk | Impact | Mitigation / acceptance condition |
| --- | --- | --- |
| Existing file IDs are canonical UI IDs while CodeView needs stable unique IDs | Broken file scrolls or key collisions | Use the current `ParsedDiffFile.id` as the `CodeViewDiffItem.id`, retaining existing `buildFileKeys` behavior and duplicate-path protection. |
| Existing parser produces raw file patches while CodeView consumes `FileDiffMetadata` | Migration cannot construct items or can regress quoted paths, renames, deletions, additions, `/dev/null`, or malformed patches | The ordinary-patch probe proves only the shape conversion. Characterize every existing path case and representative input during Task 9 before treating the migration as complete. |
| Inline cards change item height after CodeView measures it | Jumps, clipping, or missed active-card scroll | Use CodeView-owned item version updates when annotations change; retain the next-frame card DOM scroll. Validate comments, reply composer open/close, and resolved-card height changes against a large real PR. |
| `onScroll` active-file derivation is not yet empirically characterized | `n` / `p` cursor may drift or scroll-jack | Prototype with CodeView rendered-item metadata in Task 9 and manually check manual scroll, URL navigation, sidebar click, and `n` / `p`. Keep state updates immutable. |
| Expertise currently sits between bespoke header and diff body | Semantics/styling could move | Use verified `renderCustomHeader` rather than forcing expertise into line annotations. Preserve existing `FileExpertiseLine` UI and error state. |
| Worker pool initialization requires bundler-specific workers | Startup/highlighting breakage | Keep the current `disableWorkerPool` setting for the migration. Add worker pooling separately only with an Electron/Vite worker-factory spike. |
| Parser benchmark does not measure render performance | False confidence from unchanged numbers | Re-run it to prevent parsing regression, then manually test a large synced PR for initial load, scrolling, unified/split toggle, and annotation insertion. |

## Real-app verification attempt

A temporary, uncommitted dev-only renderer entry mounted a fixture-backed controlled `CodeView` with 1,000 diff items, one additions-side and one deletions-side annotation at item 500, custom headers, `stickyHeaders`, `disableWorkerPool`, and a programmatic sequence designed to record:

1. a far item target (`file-0999`);
2. forty immediate item targets (the hold-key / repeated `j`/`k` analogue);
3. an annotated additions-side line target (`file-0500`, line 14);
4. a unified-to-split toggle plus the post-toggle annotation count and scroll delta.

The scratch component was type-checked successfully, Electron Forge started successfully, the Bistro main window was created, and the Go engine reached ready state and connected to the app. The temporary main-entry wiring and probe file were then removed before this note was updated; no shipped route or component changed.

**Blocked observations:** there is no configured Playwright/Puppeteer/Chromium-driver setup, no DevTools Protocol endpoint exposed by this Forge launch, and accessibility/AppleScript inspection timed out in this environment. The Forge process did not forward the renderer `console.info` emitted by the probe. Consequently no live result values or visual observations were available, so the sequence cannot be counted as empirical confirmation of scrolling, toggling, annotation anchoring, or responsiveness.

## Human renderer verification

The gate was cleared by a human-driven check after the temporary fixture probe was corrected and run in both the Vite browser renderer and Electron shell. The browser was useful to expose the initial host-sizing defect; Electron is the relevant desktop runtime confirmation.

| Check | Human-verified result |
| --- | --- |
| Keyboard controls (`1`, `2`, `3`, `T`) | Pass after probe autofocus/event handling correction |
| Far item target | Pass: item 900 lands visibly in the expected region |
| Exact line target and annotations | Pass: item 500, line 14, and both additions/deletions annotations are correctly anchored |
| Rapid programmatic scrolling | Pass: twenty sequential line targets settle cleanly at line 33 without blanking or stutter |
| Unified/split toggle | Pass: rough viewport position is retained and annotations remain anchored |
| Free scrolling / virtualization | Pass: smooth scrolling across the 1,000-file fixture in Electron |

The temporary component, hash gate, and dev command-palette entry were removed after verification. No shipped route or component remains changed by the probe.

### Findings that Task 9 must preserve

1. **CodeView owns its root scroller.** The React host supplied to `CodeView` is the element that receives the library's native scroll listener and imperative `scrollTo` calls. It must be a bounded vertical scroll surface, for example `min-h-0 flex-1 overflow-y-auto`, within a `min-h-0 overflow-hidden` flex parent. The initial probe omitted `overflow-y-auto`, which left the wrong element to receive native scrolling and appeared frozen. Use `containerRef` when legacy bindings need the root element.
2. **Cold far item targets are estimated.** A far `type: 'item'` jump can initially land within about two items because CodeView uses shared default `itemMetrics` of 20px line height, 44px header height, and 8px spacing until preceding items are measured. Mounted layout self-corrects; exact line targets passed the human check. If Task 9 needs pixel-perfect far item jumps, add a post-measure correction/retry rather than assuming the first item jump is exact.

The remaining parser edge-case characterization and ordinary review-route acceptance are Task 9 implementation/acceptance work, not blockers for selecting CodeView. Word-level rendering is intentionally not a Task 8 requirement; it belongs to Task 11 and remains unverified by this probe.

## Concrete Task 9 implementation guidance

1. **Keep the current public contract first.** Extract the existing parsed-file and path-key utilities before replacing the canvas. Write the planned characterization tests for quoted paths, renames, deletions, `/dev/null`, and malformed/plain patches.
2. **Use a controlled CodeView.** Derive a memoized, immutable `CodeViewDiffItem<ReviewThread>[]` from the current parsed files, filtered current-anchor threads, and their mapped annotations. Stable ID: current `file.id`; derive a deterministic `version` from every render-relevant file, annotation, thread-card, and reply/resolution state so matching ID/version pairs never retain stale output.
3. **Parse into library metadata at the boundary.** A CodeView diff item needs `fileDiff`, not `patch`. `parsePatchFiles(file.patch)` returns `ParsedPatch[]`; flatten their `.files` values deliberately and fail visibly if a file cannot be represented. Do not silently omit a file.
4. **Start with minimal, explicit options.** Use `diffStyle: 'unified'`, `stickyHeaders: true`, `layout: { paddingTop: 0, paddingBottom: 0, gap: 0 }`, the existing theme, and `disableWorkerPool`. Do not add collapse or worker-pool behavior as part of the migration.
5. **Render headers and cards through supported callbacks.** Use `renderCustomHeader` for the existing file header and `FileExpertiseLine`; use `renderAnnotation` for `ThreadAnnotationCard`. Maintain `data-annotation-key` on the card.
6. **Make the CodeView ref the sole programmatic file navigator.** URL state, sidebar clicks, and selected-file-index effects should call the verified item target. For an active thread, use the item target followed by the existing next-frame DOM card scroll. Keep generic scroll fallback pointing at CodeView's bounded `containerRef` root. Allow for cold item-target estimation or issue a post-measure correction if exact placement is required.
7. **Treat visible-file tracking as a contained adapter.** Use CodeView's `onScroll` / viewer state to update `selectedFileIndex` only for user-driven viewport changes. Test that state updates never trigger a compensating scroll.
8. **Preserve non-host concerns in place.** Do not modify mark-viewed, range-diff branching, or the range-diff indicator merely to accommodate CodeView.
9. **Finish with real Electron acceptance.** On a large synced PR, verify: first render; fast scroll; `n`/`p`; generic `j`/`k`; thread-card `j`/`k` and Enter/Escape; reply composer; resolved/outdated behavior; sidebar selection; URL `?file`; mark viewed; range diff; unified/split; expertise normal/error states. Re-run `pnpm bench:large-diff` and compare parsing figures with this note.

## Final gate result

**GO.** The installed API, jsdom mount, and human-driven renderer verification establish that CodeView is an appropriate migration candidate. The 1,000-file fixture verified keyboard controls, visible far-item navigation, exact line navigation and both annotation sides, rapid sequential scrolling, layout-toggle retention, annotation anchoring, and smooth free scrolling in Electron. Task 9 may begin with the root-scroller and estimated-item-jump guidance above. Preserve the PatchDiff-props fallback only as the response if ordinary review-route acceptance exposes an unanticipated regression.
