import { PatchDiff } from '@pierre/diffs/react';
import { useEffect, useMemo, useRef, useState } from 'react';
import type { AppSettings } from '../../types/settings';

export interface PierreDiffViewProps {
  /** Full unified-diff string (`diff --git …` separated). */
  diff: string;
  /** Current theme; currently unused but we thread it through so the
   *  component can re-render when the user flips modes. */
  theme: AppSettings['appearance']['theme'];
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
  findQuery = '',
}: PierreDiffViewProps) {
  const files = useMemo(() => splitUnifiedDiff(diff), [diff]);
  const [activeId, setActiveId] = useState<string | null>(files[0]?.id ?? null);

  useEffect(() => {
    if (files.length === 0) {
      setActiveId(null);
      return;
    }
    if (activeId === null || !files.some((f) => f.id === activeId)) {
      setActiveId(files[0]?.id ?? null);
    }
  }, [files, activeId]);

  const filtered = useMemo(() => {
    const q = findQuery.trim().toLowerCase();
    if (q === '') return files;
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
        onSelect={setActiveId}
        totalCount={files.length}
      />
      <DiffCanvas
        files={files}
        activeId={activeId}
        theme={theme}
        onActiveChange={setActiveId}
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
          Files Changed ({totalCount.toString().padStart(2, '0')})
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
                    ? 'bg-surface-container-lowest text-primary border-l-2 border-primary'
                    : 'hover:bg-surface-container-highest/50 text-on-surface border-l-2 border-transparent'
                }`}
              >
                <span
                  className={`material-symbols-outlined text-[16px] ${
                    isActive ? 'text-primary' : 'text-outline'
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
  theme: AppSettings['appearance']['theme'];
  onActiveChange: (id: string) => void;
}

function DiffCanvas({
  files,
  activeId,
  theme: _theme,
  onActiveChange,
}: DiffCanvasProps) {
  const scrollRef = useRef<HTMLElement>(null);
  const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map());

  useEffect(() => {
    if (activeId === null) return;
    const el = fileRefs.current.get(activeId);
    if (el !== undefined) {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }, [activeId]);

  useEffect(() => {
    const root = scrollRef.current;
    if (root === null) return;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries.find((e) => e.isIntersecting);
        if (visible !== undefined) {
          const id = (visible.target as HTMLElement).dataset.fileId;
          if (id !== undefined && id !== activeId) onActiveChange(id);
        }
      },
      { root, threshold: 0.15 },
    );
    fileRefs.current.forEach((el) => {
      observer.observe(el);
    });
    return () => observer.disconnect();
  }, [activeId, onActiveChange]);

  return (
    <section
      ref={scrollRef}
      className="flex-1 bg-surface-container-lowest overflow-auto relative"
    >
      <div className="min-w-full">
        {files.map((file) => (
          <div
            key={file.id}
            ref={(el) => {
              if (el !== null) fileRefs.current.set(file.id, el);
              else fileRefs.current.delete(file.id);
            }}
            data-file-id={file.id}
            className="border-b border-outline-variant/10"
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
              patch={file.patch}
              disableWorkerPool
              className="block w-full"
            />
          </div>
        ))}
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
    const lang = guessLang('patch');
    const { additions, deletions } = countChanges(raw);
    return [
      {
        id: 'patch-0',
        path: 'patch',
        lang,
        additions,
        deletions,
        patch: raw,
      },
    ];
  }

  const lines = raw.split('\n');
  const chunks: string[] = [];
  let buf: string[] = [];
  for (const line of lines) {
    if (line.startsWith('diff --git ')) {
      if (buf.length > 0) chunks.push(buf.join('\n'));
      buf = [line];
    } else {
      buf.push(line);
    }
  }
  if (buf.length > 0) chunks.push(buf.join('\n'));

  return chunks.map((patch, i) => {
    const path = extractFilePath(patch) ?? `file-${i}`;
    const lang = guessLang(path);
    const { additions, deletions } = countChanges(patch);
    return {
      id: `${path}::${i}`,
      path,
      lang,
      additions,
      deletions,
      patch,
    };
  });
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
  for (const line of patch.split('\n')) {
    if (line.startsWith('+++') || line.startsWith('---')) continue;
    if (line.startsWith('+')) additions += 1;
    else if (line.startsWith('-')) deletions += 1;
  }
  return { additions, deletions };
}

function guessLang(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? '';
  const map: Record<string, string> = {
    ts: 'TS',
    tsx: 'TSX',
    js: 'JS',
    jsx: 'JSX',
    mjs: 'JS',
    cjs: 'JS',
    go: 'GO',
    rs: 'RUST',
    py: 'PY',
    rb: 'RB',
    java: 'JAVA',
    kt: 'KT',
    swift: 'SWIFT',
    cpp: 'CPP',
    c: 'C',
    h: 'H',
    hpp: 'HPP',
    md: 'MD',
    json: 'JSON',
    yml: 'YAML',
    yaml: 'YAML',
    toml: 'TOML',
    sh: 'SH',
    css: 'CSS',
    html: 'HTML',
    sql: 'SQL',
  };
  return map[ext] ?? ext.toUpperCase() ?? 'TXT';
}
