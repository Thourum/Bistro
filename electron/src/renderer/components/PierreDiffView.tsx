import { PatchDiff } from "@pierre/diffs/react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, useState } from "react";
import type { AppSettings } from "../../types/settings";
import { useStore } from "../hooks/useStore";
import { appStore } from "../store";

export interface PierreDiffViewProps {
  /** Full unified-diff string (`diff --git …` separated). */
  diff: string;
  /** Current theme; currently unused but we thread it through so the
   *  component can re-render when the user flips modes. */
  theme: AppSettings["appearance"]["theme"];
  /** Optional find query used to narrow the file list. Diff-canvas
   *  find-in-content is deferred to a later pass. */
  findQuery?: string;
}

interface ParsedFile {
  /** A stable key for React. */
  id: string;
  /** `a/foo.ts` → `foo.ts` (best-effort). */
  path: string;
  /** Extracted language tag, e.g. `TS`, `RS`. */
  lang: string;
  /** Added LOC count. */
  additions: number;
  /** Removed LOC count. */
  deletions: number;
  /** The per-file unified-diff slice. */
  patch: string;
  /** Total lines in the patch — used to estimate virtual row heights. */
  lineCount: number;
}

/**
 * `PierreDiffView` — wraps `@pierre/diffs/react` to render a multi-file
 * unified-diff payload using the 25/75 split layout from
 * `diff_view_minimal_light/code.html`.
 *
 * Inline comments are intentionally NOT wired this pass (see `renderAnnotation`
 * omitted below) — we'll re-enable them once we have `actionId`-based keybinds.
 */
export function PierreDiffView({
  diff,
  theme,
  findQuery = "",
}: PierreDiffViewProps) {
  const files = useMemo(() => splitUnifiedDiff(diff), [diff]);
  const [activeId, setActiveId] = useState<string | null>(files[0]?.id ?? null);
  // Registered by DiffCanvas so nav clicks can jump the virtualized list.
  const scrollToFileRef = useRef<(id: string) => void>(() => undefined);
  const selectedFileIndex = useStore(appStore, (s) => s.selectedFileIndex);

  useEffect(() => {
    if (files.length === 0) {
      setActiveId(null);
      return;
    }
    if (activeId === null || !files.some((f) => f.id === activeId)) {
      setActiveId(files[0]?.id ?? null);
    }
  }, [files, activeId]);

  // The n/p keybindings drive store.selectedFileIndex; follow it here.
  useEffect(() => {
    if (files.length === 0) return;
    const clamped = Math.min(Math.max(selectedFileIndex, 0), files.length - 1);
    if (clamped !== selectedFileIndex) {
      appStore.setState((s) => ({ ...s, selectedFileIndex: clamped }));
      return;
    }
    const file = files[clamped];
    if (file !== undefined && file.id !== activeId) {
      setActiveId(file.id);
      scrollToFileRef.current(file.id);
    }
    // activeId intentionally omitted: scroll-driven highlight changes must
    // not yank the viewport back to the store-selected file.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedFileIndex, files]);

  // Keep the store index in sync when scrolling changes the active file, so
  // the next n/p press moves relative to what the user actually sees.
  const handleActiveChange = (id: string): void => {
    setActiveId(id);
    const idx = files.findIndex((f) => f.id === id);
    if (idx >= 0 && appStore.state.selectedFileIndex !== idx) {
      appStore.setState((s) => ({ ...s, selectedFileIndex: idx }));
    }
  };

  const filtered = useMemo(() => {
    const q = findQuery.trim().toLowerCase();
    if (q === "") return files;
    return files.filter((f) => f.path.toLowerCase().includes(q));
  }, [files, findQuery]);

  if (files.length === 0) {
    return (
      <div className="w-full h-full flex items-center justify-center text-outline font-mono text-xs uppercase tracking-widest">
        No changes
      </div>
    );
  }

  return (
    <div className="w-full h-full flex overflow-hidden">
      <FileNav
        files={filtered}
        activeId={activeId}
        onSelect={(id) => {
          setActiveId(id);
          scrollToFileRef.current(id);
        }}
        totalCount={files.length}
      />
      <DiffCanvas
        files={files}
        activeId={activeId}
        theme={theme}
        onActiveChange={handleActiveChange}
        registerScrollTo={(fn) => {
          scrollToFileRef.current = fn;
        }}
      />
    </div>
  );
}

interface FileNavProps {
  files: ParsedFile[];
  activeId: string | null;
  onSelect: (id: string) => void;
  totalCount: number;
}

function FileNav({ files, activeId, onSelect, totalCount }: FileNavProps) {
  return (
    <aside className="w-1/4 min-w-[220px] bg-surface-container-low border-r border-outline-variant/20 overflow-y-auto">
      <div className="p-4">
        <h3 className="font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-4">
          Files Changed ({totalCount.toString().padStart(2, "0")})
        </h3>
        <nav className="space-y-1">
          {files.map((f) => {
            const isActive = f.id === activeId;
            return (
              <button
                key={f.id}
                type="button"
                onClick={() => onSelect(f.id)}
                className={`w-full flex items-center gap-2 px-2 py-1.5 text-left transition-colors ${
                  isActive
                    ? "bg-surface-container-lowest text-primary border-l-2 border-primary"
                    : "hover:bg-surface-container-highest/50 text-on-surface border-l-2 border-transparent"
                }`}
              >
                <span
                  className={`material-symbols-outlined text-[16px] ${
                    isActive ? "text-primary" : "text-outline"
                  }`}
                >
                  description
                </span>
                <span className="font-mono text-[11px] truncate flex-1">
                  {f.path}
                </span>
                <span className="font-mono text-[9px] text-outline shrink-0">
                  +{f.additions}/-{f.deletions}
                </span>
              </button>
            );
          })}
        </nav>
      </div>
    </aside>
  );
}

interface DiffCanvasProps {
  files: ParsedFile[];
  activeId: string | null;
  theme: AppSettings["appearance"]["theme"];
  onActiveChange: (id: string) => void;
  registerScrollTo: (fn: (id: string) => void) => void;
}

/** Estimated rendered height of one diff line, used until measurement. */
const ESTIMATED_LINE_HEIGHT = 20;
/** Per-file chrome: sticky header plus borders. */
const FILE_HEADER_HEIGHT = 40;

/**
 * Virtualized diff canvas: only the file blocks near the viewport are
 * mounted, so 100+ file diffs don't pay the DOM cost of every `PatchDiff`.
 * Rows are positioned with `top` (not `transform`) so the per-file sticky
 * headers keep sticking to the scroll viewport.
 */
function DiffCanvas({
  files,
  activeId,
  theme: _theme,
  onActiveChange,
  registerScrollTo,
}: DiffCanvasProps) {
  const scrollRef = useRef<HTMLElement>(null);

  const virtualizer = useVirtualizer({
    count: files.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => {
      const file = files[index];
      if (file === undefined) return FILE_HEADER_HEIGHT;
      return FILE_HEADER_HEIGHT + file.lineCount * ESTIMATED_LINE_HEIGHT;
    },
    overscan: 2,
    getItemKey: (index) => files[index]?.id ?? index,
  });

  useEffect(() => {
    registerScrollTo((id: string) => {
      const idx = files.findIndex((f) => f.id === id);
      if (idx >= 0) {
        virtualizer.scrollToIndex(idx, { align: "start" });
      }
    });
  }, [files, virtualizer, registerScrollTo]);

  const virtualItems = virtualizer.getVirtualItems();

  // The topmost visible file drives the nav highlight. Scrolling never
  // scrolls back (no scroll-jacking) — only nav clicks call scrollToIndex.
  const scrollOffset = virtualizer.scrollOffset ?? 0;
  const firstVisibleIndex =
    virtualItems.find((v) => v.end > scrollOffset + 1)?.index ?? -1;

  useEffect(() => {
    if (firstVisibleIndex < 0) return;
    const file = files[firstVisibleIndex];
    if (file !== undefined && file.id !== activeId) {
      onActiveChange(file.id);
    }
  }, [firstVisibleIndex, files, activeId, onActiveChange]);

  return (
    <section
      ref={scrollRef}
      data-diff-viewport
      className="flex-1 bg-surface-container-lowest overflow-auto relative"
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualItems.map((virtualRow) => {
          const file = files[virtualRow.index];
          if (file === undefined) return null;
          return (
            <div
              key={virtualRow.key}
              ref={virtualizer.measureElement}
              data-index={virtualRow.index}
              data-file-id={file.id}
              className="absolute left-0 w-full border-b border-outline-variant/10"
              style={{ top: virtualRow.start }}
            >
              <div className="sticky top-0 z-10 bg-surface-container-lowest border-b border-outline-variant/10 px-4 py-2 flex items-center justify-between">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-outline font-mono text-[11px] truncate">
                    {file.path}
                  </span>
                  <span className="bg-surface-container px-2 py-0.5 text-[10px] text-outline uppercase font-mono shrink-0">
                    {file.lang}
                  </span>
                </div>
                <div className="flex gap-4 font-mono text-[11px] font-bold shrink-0">
                  <span className="text-primary">+{file.additions}</span>
                  <span className="text-error">-{file.deletions}</span>
                </div>
              </div>
              <PatchDiff
                options={{
                  theme: `pierre-${_theme}`,
                }}
                patch={file.patch}
                disableWorkerPool
                className="block w-full"
              />
            </div>
          );
        })}
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Unified-diff splitter
// ---------------------------------------------------------------------------

/**
 * Split a multi-file unified diff into per-file entries keyed by path.
 *
 * Handles the standard `diff --git a/foo b/foo` preamble. If the payload does
 * not have `diff --git` headers (e.g. the server returned a plain patch), we
 * fall back to `^diff ` or `^---` separators so the library can still try to
 * render something sensible.
 */
function splitUnifiedDiff(raw: string): ParsedFile[] {
  if (raw.trim().length === 0) return [];

  const GIT_HEADER = /^diff --git /m;
  if (!GIT_HEADER.test(raw)) {
    const { additions, deletions } = countChanges(raw);
    return [
      {
        id: "patch-0",
        path: "patch",
        lang: "patch",
        additions,
        deletions,
        patch: raw,
        lineCount: countLines(raw),
      },
    ];
  }

  const lines = raw.split("\n");
  const chunks: string[] = [];
  let buf: string[] = [];
  for (const line of lines) {
    if (line.startsWith("diff --git ")) {
      if (buf.length > 0) chunks.push(buf.join("\n"));
      buf = [line];
    } else {
      buf.push(line);
    }
  }
  if (buf.length > 0) chunks.push(buf.join("\n"));

  return chunks.map((patch, i) => {
    const path = extractFilePath(patch) ?? `file-${i}`;
    const lang = path.split(".").pop()?.toUpperCase() ?? "";
    const { additions, deletions } = countChanges(patch);
    return {
      id: `${path}::${i}`,
      path,
      lang,
      additions,
      deletions,
      patch,
      lineCount: countLines(patch),
    };
  });
}

function countLines(patch: string): number {
  let count = 1;
  for (let i = 0; i < patch.length; i++) {
    if (patch.charCodeAt(i) === 10) count++;
  }
  return count;
}

function extractFilePath(patch: string): string | null {
  const match = /^diff --git a\/(\S+) b\/(\S+)/m.exec(patch);
  if (match !== null) return match[2] ?? match[1] ?? null;
  const plus = /^\+\+\+ b\/(\S+)/m.exec(patch);
  if (plus !== null) return plus[1] ?? null;
  return null;
}

function countChanges(patch: string): {
  additions: number;
  deletions: number;
} {
  let additions = 0;
  let deletions = 0;
  for (const line of patch.split("\n")) {
    if (line.startsWith("+++") || line.startsWith("---")) continue;
    if (line.startsWith("+")) additions += 1;
    else if (line.startsWith("-")) deletions += 1;
  }
  return { additions, deletions };
}
