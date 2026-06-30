# Markdown Rendering for PR Descriptions & Comments — Implementation Plan (Round 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Render GitHub-Flavored Markdown — incl. raw `<details>`/`<summary>`, fenced code with syntax highlighting, and ` ```diff ` blocks — in PR descriptions and comment bodies, replacing today's plain `whitespace-pre-wrap` text.

**Architecture:** One memoized `<Markdown>` component on `react-markdown` + `remark-gfm` + `rehype-raw` + `rehype-sanitize`. Raw HTML is enabled then re-narrowed by a sanitize schema (GitHub `defaultSchema` + `details`/`summary`) → XSS-safe by construction. Fenced code reuses the **already-installed-but-idle** `highlightCode` (`highlight.js`); ` ```diff ` uses the `diff` grammar. PR *file* diffs are untouched — they already render natively via `@pierre/diffs` `PatchDiff`. PR titles stay plain.

**Tech Stack:** React 19 (CSR only — no SSR), TypeScript, react-markdown, remark-gfm, rehype-raw, rehype-sanitize, highlight.js (installed), Tailwind v4 (`@theme` tokens), Vitest + @testing-library/react (installed), Biome, pnpm, neverthrow.

---

## Why this approach (resolves the round-1 council debate with evidence)

The two contested decisions, settled against the actual code/libs in this repo:

- **` ```diff ` inside markdown → highlight.js, NOT Pierre `PatchDiff`.** Verified in-sandbox: `@pierre/diffs`' `parsePatchFiles()` returns `files: []` (renders nothing) for a bare `+/-` fence or an `@@`-only hunk; it only renders when fed `--- / +++ / @@` headers. Comment ` ```diff ` blocks are bare hunks, so "reuse PatchDiff" actually means **synthesizing fake git headers per fence** — more code, fragile, and it drags in `@pierre/diffs`' virtualization. The `highlight.js` `diff` grammar colors `+`/`-`/context lines directly (GitHub's look) in ~2 lines of registration. This also matches the explicit product decision: PR *file* patches keep rendering through Pierre (`PierreDiffView` → `<PatchDiff patch={file.patch}/>`, unchanged); only *inline* diff fences use the highlighter.
- **The real gap both round-1 plans missed:** `highlightCode` (`utils/highlighter.ts`) is currently **wired to nothing**, the `diff` grammar is **not registered**, and there is **no `hljs-*` theme CSS** anywhere in the project. So naively routing code through it yields *colorless* output and ` ```diff ` falls back to escaped text. This plan fixes all three (Tasks 2 & 3). `highlight.js/styles/github.css` ships `.hljs-addition`/`.hljs-deletion` on disk — confirmed — so the diff coloring is a real, working path once imported.

Non-issues, stated honestly so they don't get re-raised:
- **Virtualized-row height:** none of the three render sites (`CommentThread`, `UnresolvedThreadsView`, `PRDetailPage`) use `@tanstack/react-virtual` — confirmed. So variable markdown height is not a risk *here*. (If a future virtualized comment list renders markdown, revisit with `measureElement`.)
- **Streaming / SSR:** out of scope. Bodies arrive complete over WS; `React.memo` gives parse-once. Electron renderer is CSR.

---

## Requirements

| # | Requirement | Current state |
|---|---|---|
| R1 | GFM (bold, lists, tables, task-lists, strikethrough, autolinks) in **PR description** | `PRDetailPage.tsx:317-318` — `pr.body` in a `whitespace-pre-wrap <p>` |
| R2 | GFM in **inline comment bodies** | `CommentThread.tsx:133` — `{comment.body}` plain text |
| R3 | GFM in **unresolved-thread comments** | `UnresolvedThreadsView.tsx:349` — `{comment.body}` plain text |
| R4 | Raw HTML `<details>`/`<summary>` (GitHub bots emit these) | unsupported |
| R5 | Fenced code blocks with syntax highlighting | `highlightCode` exists but **idle**, no theme CSS |
| R6 | ` ```diff ` fenced blocks rendered as a diff | unsupported (`diff` grammar not registered) |
| R7 | Extensible for Phase-3 custom markdown components | n/a |
| R8 | No `try`/`catch` in committed code (use `neverthrow`) | repo convention |
| R9 | Match design tokens (`text-on-surface`, `bg-surface-container*`, `border-outline`, …) | repo convention |

PR *file* diffs (R: already met) and PR *titles* (stay plain) are explicitly out of scope.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `electron/package.json` | modify | +4 deps |
| `electron/src/renderer/utils/highlighter.ts` | modify | register `diff` grammar |
| `electron/src/renderer/styles/highlight.css` | create | hljs token colors mapped via github theme (light) + `.dark` overrides |
| `electron/src/renderer/styles/globals.css` | modify | `@import "./highlight.css";` |
| `electron/src/renderer/utils/markdownSchema.ts` | create | sanitize schema: `defaultSchema` + `details`/`summary` |
| `electron/src/renderer/utils/markdownComponents.tsx` | create | element overrides: `code`, `a`, `details`, `summary` |
| `electron/src/renderer/components/Markdown.tsx` | create | memoized `<Markdown body compact?>` wrapper |
| `electron/src/renderer/components/Markdown.test.tsx` | create | GFM / code / diff / details / XSS tests |
| `electron/src/renderer/components/CommentThread.tsx` | modify | swap display `{comment.body}` → `<Markdown>` |
| `electron/src/renderer/components/UnresolvedThreadsView.tsx` | modify | swap `{comment.body}` → `<Markdown>` |
| `electron/src/renderer/pages/PRDetailPage.tsx` | modify | swap `pr.body` (`:318`) and `c.body` (`:352`) → `<Markdown>` |

Verification commands (from `electron/package.json`): `pnpm type-check`, `pnpm lint`, `pnpm test <name>` (= `vitest run`).

---

## Task 1: Add dependencies

**Files:** Modify `electron/package.json`

- [ ] **Step 1 — Install**
```bash
cd electron && pnpm add react-markdown remark-gfm rehype-raw rehype-sanitize
```
- [ ] **Step 2 — Verify resolution + react-markdown major version**
```bash
cd electron && pnpm type-check
node -e "console.log('react-markdown', require('./node_modules/react-markdown/package.json').version)"
```
Expected: type-check PASS (these packages ship their own types); react-markdown reports **v9 or higher**. The `code` override API in Task 5 (no `inline` prop; inline detected by absence of a `language-` class) assumes v9+. If it resolves to <9, stop and reconcile the `code` component before continuing.
- [ ] **Step 3 — Commit**
```bash
git add electron/package.json electron/pnpm-lock.yaml
git commit -m "chore: add react-markdown + GFM/raw/sanitize deps"
```

---

## Task 2: Register the `diff` grammar (TDD)

`utils/highlighter.ts` uses `highlight.js/lib/core` (explicit registration) and registers 12 languages — `diff` is not among them, though `highlight.js/lib/languages/diff` exists on disk.

**Files:** Modify `electron/src/renderer/utils/highlighter.ts`; create `electron/src/renderer/utils/highlighter.test.ts`

- [ ] **Step 1 — Failing test**
```ts
// electron/src/renderer/utils/highlighter.test.ts
import { describe, expect, it } from 'vitest';
import { highlightCode } from './highlighter';

describe('highlightCode diff grammar', () => {
  it('emits addition/deletion token classes', () => {
    const html = highlightCode('+added\n-removed\n context', 'diff');
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
});
```
- [ ] **Step 2 — Run; expect FAIL**
```bash
cd electron && pnpm test highlighter
```
Expected: FAIL — `diff` unregistered → `highlightCode` catches and HTML-escapes (no `hljs-addition`).
- [ ] **Step 3 — Register grammar**

Add alongside the other imports in `highlighter.ts`:
```ts
import diff from 'highlight.js/lib/languages/diff';
```
Add alongside the other `registerLanguage` calls:
```ts
hljs.registerLanguage('diff', diff);
```
- [ ] **Step 4 — Run; expect PASS**
```bash
cd electron && pnpm test highlighter
```
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/utils/highlighter.ts electron/src/renderer/utils/highlighter.test.ts
git commit -m "feat: register diff grammar in highlighter"
```

---

## Task 3: hljs theme CSS (the gap both round-1 plans missed)

Without `.hljs-*` color rules, highlighted output is monospace-but-colorless. The app toggles dark mode via a `.dark` class on `<html>` (`globals.css`), so we load the github (light) palette as the base and override with github-dark under `.dark` — `.dark .hljs-x` (specificity 0-2-0) beats base `.hljs-x` (0-1-0) in dark mode; light mode is unaffected because `.dark` is absent.

The complete file is given below verbatim — **paste it whole, do not abridge.** Color values are copied exactly from `node_modules/highlight.js/styles/github.css` and `github-dark.css` (18 color rules each). hljs's structural rules (`padding`/`display` on `code.hljs`) are intentionally omitted — layout comes from the Tailwind classes on the `<code>` element (Task 5) — and `.hljs` `background` is forced `transparent` so the container's `bg-surface-*` token shows through.

> `ponytail: reuses hljs's own theme palette verbatim; hand-map to design tokens only if the github colors clash with the app theme.`

**Files:** Create `electron/src/renderer/styles/highlight.css`; modify `electron/src/renderer/styles/globals.css`

- [ ] **Step 1 — Create `electron/src/renderer/styles/highlight.css` with EXACTLY this content:**
```css
/* highlight.js token colors. Light base = github.css; dark overrides = github-dark.css
   (prefixed .dark — app toggles dark mode via a .dark class on <html>).
   Color values copied verbatim from node_modules/highlight.js/styles/*.css.
   Structural rules (padding/display) intentionally omitted — layout comes from
   the Tailwind classes on the <code> element. .hljs background forced transparent
   so the container's bg-surface token shows through. */

.hljs { color: #24292e; background: transparent }
.hljs-doctag,
.hljs-keyword,
.hljs-meta .hljs-keyword,
.hljs-template-tag,
.hljs-template-variable,
.hljs-type,
.hljs-variable.language_ { color: #d73a49 }
.hljs-title,
.hljs-title.class_,
.hljs-title.class_.inherited__,
.hljs-title.function_ { color: #6f42c1 }
.hljs-attr,
.hljs-attribute,
.hljs-literal,
.hljs-meta,
.hljs-number,
.hljs-operator,
.hljs-variable,
.hljs-selector-attr,
.hljs-selector-class,
.hljs-selector-id { color: #005cc5 }
.hljs-regexp,
.hljs-string,
.hljs-meta .hljs-string { color: #032f62 }
.hljs-built_in,
.hljs-symbol { color: #e36209 }
.hljs-comment,
.hljs-code,
.hljs-formula { color: #6a737d }
.hljs-name,
.hljs-quote,
.hljs-selector-tag,
.hljs-selector-pseudo { color: #22863a }
.hljs-subst { color: #24292e }
.hljs-section { color: #005cc5; font-weight: bold }
.hljs-bullet { color: #735c0f }
.hljs-emphasis { color: #24292e; font-style: italic }
.hljs-strong { color: #24292e; font-weight: bold }
.hljs-addition { color: #22863a; background-color: #f0fff4 }
.hljs-deletion { color: #b31d28; background-color: #ffeef0 }

/* ── Dark mode overrides ── */
.dark .hljs { color: #c9d1d9; background: transparent }
.dark .hljs-doctag,
.dark .hljs-keyword,
.dark .hljs-meta .hljs-keyword,
.dark .hljs-template-tag,
.dark .hljs-template-variable,
.dark .hljs-type,
.dark .hljs-variable.language_ { color: #ff7b72 }
.dark .hljs-title,
.dark .hljs-title.class_,
.dark .hljs-title.class_.inherited__,
.dark .hljs-title.function_ { color: #d2a8ff }
.dark .hljs-attr,
.dark .hljs-attribute,
.dark .hljs-literal,
.dark .hljs-meta,
.dark .hljs-number,
.dark .hljs-operator,
.dark .hljs-variable,
.dark .hljs-selector-attr,
.dark .hljs-selector-class,
.dark .hljs-selector-id { color: #79c0ff }
.dark .hljs-regexp,
.dark .hljs-string,
.dark .hljs-meta .hljs-string { color: #a5d6ff }
.dark .hljs-built_in,
.dark .hljs-symbol { color: #ffa657 }
.dark .hljs-comment,
.dark .hljs-code,
.dark .hljs-formula { color: #8b949e }
.dark .hljs-name,
.dark .hljs-quote,
.dark .hljs-selector-tag,
.dark .hljs-selector-pseudo { color: #7ee787 }
.dark .hljs-subst { color: #c9d1d9 }
.dark .hljs-section { color: #1f6feb; font-weight: bold }
.dark .hljs-bullet { color: #f2cc60 }
.dark .hljs-emphasis { color: #c9d1d9; font-style: italic }
.dark .hljs-strong { color: #c9d1d9; font-weight: bold }
.dark .hljs-addition { color: #aff5b4; background-color: #033a16 }
.dark .hljs-deletion { color: #ffdcd7; background-color: #67060c }
```
- [ ] **Step 2 — Import it** in `electron/src/renderer/styles/globals.css`. The file currently starts with `@import "tailwindcss";` — add this immediately after that line (so the theme loads after Tailwind's reset):
```css
@import "./highlight.css";
```
- [ ] **Step 3 — Verify build & lint**
```bash
cd electron && pnpm lint && pnpm type-check
```
Expected: PASS. (CSS color correctness is verified visually in the final smoke test; there is no meaningful unit test for theme CSS.)
- [ ] **Step 4 — Commit**
```bash
git add electron/src/renderer/styles/highlight.css electron/src/renderer/styles/globals.css
git commit -m "feat: hljs theme CSS for fenced code blocks (light + dark)"
```

---

## Task 4: Sanitize schema (TDD)

`rehype-sanitize`'s `defaultSchema` is GitHub's. It allows `className` matching `/^language-/` on `code` (so language survives to our override) but does **not** allow `details`/`summary`. Extend immutably — never mutate `defaultSchema` (R8/immutability).

**Files:** Create `electron/src/renderer/utils/markdownSchema.ts` + `markdownSchema.test.ts`

- [ ] **Step 1 — Failing test**
```ts
// electron/src/renderer/utils/markdownSchema.test.ts
import { describe, expect, it } from 'vitest';
import { markdownSchema } from './markdownSchema';

describe('markdownSchema', () => {
  it('allows details and summary', () => {
    expect(markdownSchema.tagNames).toContain('details');
    expect(markdownSchema.tagNames).toContain('summary');
  });
  it('does not allow script', () => {
    expect(markdownSchema.tagNames).not.toContain('script');
  });
  it('keeps GitHub protocol restrictions', () => {
    expect(markdownSchema.protocols?.href).toBeDefined();
  });
});
```
- [ ] **Step 2 — Run; expect FAIL** (`pnpm test markdownSchema` — module missing)
- [ ] **Step 3 — Implement**
```ts
// electron/src/renderer/utils/markdownSchema.ts
import { defaultSchema } from 'rehype-sanitize';

/**
 * Sanitize schema for untrusted PR markdown. Starts from rehype-sanitize's
 * GitHub-derived defaultSchema and additively permits <details>/<summary>.
 * Nothing else is loosened. Built immutably — defaultSchema is never mutated.
 */
export const markdownSchema = {
  ...defaultSchema,
  tagNames: [...(defaultSchema.tagNames ?? []), 'details', 'summary'],
} as const;
```
- [ ] **Step 4 — Run; expect PASS**
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/utils/markdownSchema.ts electron/src/renderer/utils/markdownSchema.test.ts
git commit -m "feat: markdown sanitize schema with details/summary"
```

---

## Task 5: Element overrides

react-markdown calls `code` for both inline code (no `language-` class) and fenced blocks (`className="language-x"`, since v9 removed the `inline` prop — absence of a language class is the correct inline signal). Dispatch:
- no `language-` class → inline → plain `<code>`;
- `language-diff` → `highlightCode(text, 'diff')`;
- other known language → `highlightCode(text, lang)`;
- unknown → `highlightCode(text, null)` (escaped).

`highlightCode` returns an HTML string injected via `dangerouslySetInnerHTML`. **Safety:** `hljs.highlight` HTML-escapes the source text, so output is only hljs `<span>`s + escaped text — no user HTML reaches the DOM through this path. (All *other* nodes are guarded by Task 4's sanitize schema. This injection bypasses sanitize but is safe because its input is pre-escaped — tested in Task 7's "XSS inside code fence" case.) No `try/catch` is added; `highlightCode`'s pre-existing internal `try/catch` is out of scope and left as-is.

**Files:** Create `electron/src/renderer/utils/markdownComponents.tsx`

- [ ] **Step 1 — Implement**
```tsx
import type { Components } from 'react-markdown';
import { highlightCode } from './highlighter';

// Languages registered in highlighter.ts (incl. 'diff' from Task 2).
const KNOWN_LANGUAGES = new Set([
  'typescript', 'javascript', 'go', 'python', 'json', 'css',
  'xml', 'bash', 'yaml', 'sql', 'markdown', 'rust', 'diff',
]);

function languageFromClassName(className?: string): string | null {
  const match = className ? /language-([\w-]+)/.exec(className) : null;
  return match?.[1] ?? null;
}

/** Stable module-level constant → keeps React.memo on <Markdown> effective. */
export const markdownComponents: Components = {
  code({ className, children }) {
    const lang = languageFromClassName(className);
    const text = String(children ?? '');
    if (lang === null) {
      return <code className="rounded bg-surface-container px-1 py-0.5 font-mono text-xs">{text}</code>;
    }
    const html = highlightCode(text, KNOWN_LANGUAGES.has(lang) ? lang : null);
    return (
      <code
        className={`hljs language-${lang} block overflow-x-auto rounded bg-surface-container-low p-2 font-mono text-xs`}
        // biome-ignore lint/security/noDangerouslySetInnerHtml: hljs output; source pre-escaped by highlight.js
        dangerouslySetInnerHTML={{ __html: html }}
      />
    );
  },
  a({ href, children }) {
    return <a href={href} target="_blank" rel="noopener noreferrer nofollow">{children}</a>;
  },
  details({ children }) {
    return <details className="my-2 rounded border border-outline/30 px-2 py-1">{children}</details>;
  },
  summary({ children }) {
    return <summary className="cursor-pointer select-none font-medium">{children}</summary>;
  },
};
```
> **Tokens:** `globals.css` `@theme` defines `--color-surface-container`, `--color-surface-container-low`, and `--color-on-surface-variant`, so `bg-surface-container*` / `text-on-surface-variant` resolve. For the border, the defined token is `--color-outline` (not `--color-border-outline`); in Tailwind v4 `border-outline` therefore resolves to `border-color: var(--color-outline)`. All four utilities are valid. Behavior does not depend on the exact classes.
>
> **`String(children)`:** react-markdown passes `children` as a `ReactNode`. For fenced/inline code the content is a string, so `String(children ?? '')` is correct and is the idiomatic ecosystem pattern. In rare nested-node cases it could stringify oddly, but the highlighter then just escapes the text — no crash, no XSS. Acceptable; left simple.
- [ ] **Step 2 — Type-check** (`pnpm type-check` → PASS)
- [ ] **Step 3 — Commit**
```bash
git add electron/src/renderer/utils/markdownComponents.tsx
git commit -m "feat: markdown element overrides (code/diff/links/details)"
```

---

## Task 6: The `<Markdown>` component

**Files:** Create `electron/src/renderer/components/Markdown.tsx`

- [ ] **Step 1 — Implement**
```tsx
import { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeRaw from 'rehype-raw';
import rehypeSanitize from 'rehype-sanitize';
import remarkGfm from 'remark-gfm';
import { markdownComponents } from '../utils/markdownComponents';
import { markdownSchema } from '../utils/markdownSchema';

// Module-level constants → stable refs → React.memo stays effective.
const REMARK_PLUGINS = [remarkGfm];
// Order matters: rehypeRaw must precede rehypeSanitize so raw HTML is parsed
// into the tree BEFORE it is sanitized.
const REHYPE_PLUGINS = [rehypeRaw, [rehypeSanitize, markdownSchema]] as const;

interface MarkdownProps {
  body: string;
  /** Tighter spacing for inline comment contexts vs. roomy PR descriptions. */
  compact?: boolean;
  className?: string;
}

/**
 * Renders one COMPLETE GitHub-style markdown string. Memoized on props so
 * non-virtualized comment lists still parse each body once across re-renders.
 * Pipeline: GFM → raw HTML → re-narrowed by markdownSchema. XSS-safe.
 */
function MarkdownImpl({ body, compact, className }: MarkdownProps): React.JSX.Element {
  const base = compact ? 'text-xs leading-relaxed' : 'text-sm leading-relaxed';
  return (
    <div className={`${base} text-on-surface-variant ${className ?? ''}`}>
      <ReactMarkdown
        remarkPlugins={REMARK_PLUGINS}
        // biome-ignore lint/suspicious/noExplicitAny: rehype plugin-tuple typing
        rehypePlugins={REHYPE_PLUGINS as any}
        components={markdownComponents}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

export const Markdown = memo(MarkdownImpl);
```
- [ ] **Step 2 — Type-check** (PASS). No commit yet — paired with tests in Task 7.

---

## Task 7: Markdown tests (the safety net)

**Files:** Create `electron/src/renderer/components/Markdown.test.tsx`

- [ ] **Step 1 — Failing tests**
```tsx
import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Markdown } from './Markdown';

describe('<Markdown>', () => {
  it('renders a GFM table', () => {
    const { container } = render(<Markdown body={'| a | b |\n|---|---|\n| 1 | 2 |'} />);
    expect(container.querySelector('table')).not.toBeNull();
    expect(container.querySelector('td')?.textContent).toBe('1');
  });
  it('renders task lists and strikethrough', () => {
    const { container } = render(<Markdown body={'- [x] done\n\n~~gone~~'} />);
    expect(container.querySelector('input[type="checkbox"]')).not.toBeNull();
    expect(container.querySelector('del')?.textContent).toBe('gone');
  });
  it('highlights a fenced typescript block', () => {
    const { container } = render(<Markdown body={'```ts\nconst x = 1;\n```'} />);
    expect(container.querySelector('code.language-ts')?.innerHTML).toContain('hljs-');
  });
  it('highlights a fenced diff block with add/remove classes', () => {
    const { container } = render(<Markdown body={'```diff\n+new\n-old\n```'} />);
    const html = container.querySelector('code.language-diff')?.innerHTML ?? '';
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
  it('renders <details>/<summary> instead of stripping them', () => {
    const { container } = render(<Markdown body={'<details><summary>more</summary>hid</details>'} />);
    expect(container.querySelector('details')).not.toBeNull();
    expect(container.querySelector('summary')?.textContent).toBe('more');
  });
  it('renders a ```diff fence nested inside <details>', () => {
    const { container } = render(
      <Markdown body={'<details><summary>diff</summary>\n\n```diff\n+a\n-b\n```\n\n</details>'} />,
    );
    expect(container.querySelector('details code.language-diff')).not.toBeNull();
  });
  it('strips <script> (XSS)', () => {
    const { container } = render(<Markdown body={'<script>alert(1)</script>hello'} />);
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('hello');
  });
  it('strips event-handler attributes (XSS)', () => {
    const { container } = render(<Markdown body={'<img src="x" onerror="alert(1)" />'} />);
    expect(container.querySelector('img')?.getAttribute('onerror')).toBeNull();
  });
  it('neutralizes javascript: links (XSS)', () => {
    const { container } = render(<Markdown body={'[click](javascript:alert(1))'} />);
    expect((container.querySelector('a')?.getAttribute('href') ?? '').startsWith('javascript:')).toBe(false);
  });
  it('does not execute HTML smuggled inside a code fence (XSS)', () => {
    const { container } = render(<Markdown body={'```\n<img src=x onerror=alert(1)>\n```'} />);
    // The tag is shown as escaped text, not a live element.
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('<img');
  });
});
```
- [ ] **Step 2 — Run; iterate to green**
```bash
cd electron && pnpm test Markdown
```
Likely fix points if red: schema tag list (Task 4), language class name on `code`, plugin order (`rehypeRaw` before `rehypeSanitize`). Fix implementation, not tests (unless a test is wrong).
- [ ] **Step 3 — Commit**
```bash
git add electron/src/renderer/components/Markdown.tsx electron/src/renderer/components/Markdown.test.tsx
git commit -m "feat: memoized Markdown renderer with GFM/code/diff/XSS coverage"
```

---

## Task 8: Wire `<Markdown>` into the three render sites

Pure display swap. Edit-mode `<textarea>` paths stay raw text; only the display path changes. Keep existing empty-body guards.

**Files:** `CommentThread.tsx:133`, `UnresolvedThreadsView.tsx:349`, `PRDetailPage.tsx:318` & `:352`

- [ ] **Step 1 — CommentThread**: replace the display `<p>...{comment.body}...</p>` with `<Markdown body={comment.body} compact />`; add `import { Markdown } from './Markdown';`.
- [ ] **Step 2 — UnresolvedThreadsView**: replace `{comment.body}` with `<Markdown body={comment.body} compact />`; add the import (`./Markdown`).
- [ ] **Step 3 — PRDetailPage**: replace `{pr.body}` (`:318`) with `<Markdown body={pr.body} />` and `{c.body}` (`:352`) with `<Markdown body={c.body} compact />`; add `import { Markdown } from '../components/Markdown';`. Keep the `pr.body === null || trim === ''` guard at `:313`.
- [ ] **Step 4 — Full verification**
```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```
Expected: PASS. Existing CommentThread/PRDetail tests assert on text content; markdown wraps text in `<p>`, so `textContent` still matches. If a test asserted exact DOM shape, loosen it to query rendered text (implementation is correct; the old assertion was too tight).
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/components/CommentThread.tsx \
        electron/src/renderer/components/UnresolvedThreadsView.tsx \
        electron/src/renderer/pages/PRDetailPage.tsx
git commit -m "feat: render PR descriptions and comments as markdown"
```

---

## Phase-3 readiness (no work now)

Custom markdown nodes later (`@mention`, issue-ref autolinks, directives) are a `remarkPlugins`/`rehypePlugins` entry + a `components` key — no architecture change. The pipeline is the extension point. Nothing speculative built now (R7).

## Final verification

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test   # Go side untouched
```
Manual smoke: open a PR whose description has a table, a ` ```ts ` block, a ` ```diff ` block, and a `<details><summary>` containing a nested ` ```diff `. Confirm: table renders; code blocks are colored in both light and dark mode; diff lines are green/red; details collapses; a comment containing `<script>` shows the text but executes nothing.
