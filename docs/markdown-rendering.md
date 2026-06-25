# Markdown Rendering for PR Descriptions & Comments — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Render GitHub-Flavored Markdown — incl. raw `<details open>`/`<summary>`, fenced code with syntax highlighting, and ` ```diff ` blocks — in PR descriptions and all comment-body surfaces, replacing today's plain `whitespace-pre-wrap` text. External links and navigation are hardened for the Electron renderer.

**Architecture:** One memoized `<Markdown>` component on `react-markdown` + `remark-gfm` + `rehype-raw` + `rehype-sanitize`. Raw HTML is enabled then re-narrowed by a sanitize schema (GitHub `defaultSchema` + `details`/`summary` + `open`) → XSS-safe by construction. **Block code is handled at the `pre` override** (fenced *and* indented blocks), **inline code at the `code` override** — never confused. Fenced code (incl. ` ```diff ` and the `diff_hunk` snippet) reuses the already-installed-but-idle `highlightCode` (`highlight.js`) via a shared `HighlightedCode` component. Markdown links route through a shared `ExternalLink` (HTTPS-only, opens in the OS browser), backed by a main-process navigation deny-guard. PR *file* diffs are untouched — they already render natively via `@pierre/diffs` `PatchDiff`. PR titles stay plain.

**Tech Stack:** React 19 (CSR only — no SSR), TypeScript, react-markdown (v9+), remark-gfm, rehype-raw, rehype-sanitize, highlight.js (installed), Electron (`window.electron.openExternal` IPC bridge, already present), Tailwind v4 (`@theme` tokens), Vitest + @testing-library/react (installed), Biome, pnpm, neverthrow.

---

## Repo test conventions (READ FIRST — every test task depends on these)

Verified against `electron/vitest.config.ts` and existing tests:

- **All tests live in `electron/tests/unit/`** and import source via relative paths (`../../src/renderer/...`). The config's `include` is `tests/**/*.{test,spec}.{ts,tsx}` — a colocated `Foo.test.tsx` next to source **will not run**. Do not colocate.
- **Default `environment` is `node`.** Any test that calls `render()` (DOM) MUST start with the docblock `// @vitest-environment jsdom` on line 1.
- **jest-dom matchers are opt-in per file:** `import * as matchers from '@testing-library/jest-dom/matchers';` then `expect.extend(matchers);`.
- `setupFiles: ['./tests/setup.ts']` (auto-applied) shims `ResizeObserver`/`scrollIntoView`.
- **`pnpm test` = `vitest run` (no coverage gate).** The 80% threshold runs only under `pnpm test:coverage`, and its `include` is `src/main/**` + `src/shared/**` only (renderer code is not gated). `src/main/window.ts` has zero test coverage today; Task 9's edit is small and does not regress `pnpm test` (the gate isn't in the verification path). Flagged, not over-engineered.
- **`tests/unit/highlighter.test.ts` already exists** (env `node`) — Task 2 **appends** to it.
- **`tests/unit/StatusSidebar.test.tsx` already exists** and asserts `getByRole('link', { name: 'ci-build' })` + `toHaveAttribute('href', 'https://ci.example.com/1')` (`:76-77`). `ExternalLink` still renders `<a href>`, so this passes unchanged after Task 10 — verify, don't rewrite.
- **`tests/unit/PRDetailPage.test.tsx`** asserts on the PR *title* and "view diff", NOT on `pr.body`/comment text — so Task 11's body swaps don't break it.

---

## Key findings (grounded in code — these shaped the plan)

- **` ```diff ` inside markdown → highlight.js, NOT Pierre `PatchDiff`.** Verified in-sandbox: `@pierre/diffs`' `parsePatchFiles()` returns `files: []` (renders nothing) for a bare `+/-` fence or an `@@`-only hunk; it only renders with `--- / +++ / @@` headers. Comment diff fences are bare hunks, so "reuse PatchDiff" means synthesizing fake git headers per fence — fragile, and drags in Pierre's worker-pool/Shiki weight. The `highlight.js` `diff` grammar colors `+`/`-`/context directly. PR *file* patches keep rendering through Pierre (`PierreDiffView` → `<PatchDiff patch={file.patch}/>`, `PierreDiffView.tsx:351`, unchanged). The same highlight path covers the `diff_hunk` snippet above review comments (`UnresolvedThreadsView.tsx:345`).
- **`diff` grammar is NOT registered today.** `utils/highlighter.ts` uses `highlight.js/lib/core` and registers 12 languages — `diff` is absent, though `highlight.js/lib/languages/diff.js` is on disk. Without registration, `highlightCode(x,'diff')` hits its catch and returns escaped plaintext (silent no-op). Task 2 fixes it.
- **`highlightCode` is idle + there is NO hljs theme CSS.** `highlightCode`/`detectLanguage` are imported by nothing in the renderer; no `.hljs-*` rules exist. So highlighting renders *colorless* until a theme is added. Task 3 adds it (`github.css`/`github-dark.css` ship `.hljs-addition`/`.hljs-deletion` — confirmed).
- **Block vs. inline code cannot be told from the language class.** react-markdown v9+ removed the `inline` prop. An *unlabeled* fence (` ``` `) and an *indented* code block also have no `language-` class — a `code`-only override would render them as inline `<code>`, losing the `<pre>`. We handle block code at the **`pre`** override and reserve **`code`** for true inline — so unlabeled/indented blocks keep their `<pre>`.
- **Electron link safety is in scope.** `main/window.ts:createWindow` has **no** `setWindowOpenHandler`/`will-navigate` guard today (verified). A `window.electron.openExternal` bridge already exists and validates **HTTPS-only** (`main/index.ts:208-220`, preload `main/preload.ts:113`, typed `renderer/types/window.d.ts:41`, return shape `{ ok: true } | { err: string }`). We route markdown links through it and add the main-process deny-guard. The SPA uses a **hash router** (`router.tsx:304`), and hash-only changes are same-document → never emit `will-navigate`, so in-app routing is unaffected.
- **FOUR body sites + one `diff_hunk` `<pre>`** (not three). Inventory in Requirements.

Non-issues, stated honestly: none of the body sites use `@tanstack/react-virtual` (verified) — variable markdown height is not a desync risk here. Streaming/SSR are out of scope (bodies arrive complete over WS; Electron renderer is CSR).

---

## Requirements

| # | Requirement | Current state |
|---|---|---|
| R1 | GFM (bold, lists, tables, task-lists, strikethrough, autolinks) in **PR description** | `PRDetailPage.tsx:317-318` — `pr.body` in `whitespace-pre-wrap <p>` |
| R2 | GFM in **inline comment bodies** | `CommentThread.tsx:133` — `{comment.body}` plain text |
| R3 | GFM in **unresolved-thread comments** | `UnresolvedThreadsView.tsx:349` — `{comment.body}` plain text |
| R4 | GFM in **PR-detail "Recent Comments" card** (the easily-missed 4th site) | `PRDetailPage.tsx:351-352` — `{c.body}` plain text |
| R5 | Raw HTML `<details open>`/`<summary>` (GitHub bots emit these) | unsupported |
| R6 | Fenced code blocks with syntax highlighting (block ≠ inline) | `highlightCode` exists but **idle**, no theme CSS |
| R7 | ` ```diff ` fenced blocks **and** the review `diff_hunk` rendered as a colored diff | `diff` grammar unregistered; `diff_hunk` is a raw `<pre>` (`UnresolvedThreadsView.tsx:345`) |
| R8 | Markdown links are **Electron-safe** (HTTPS-only, OS browser, no in-app nav) + main-process backstop | `createWindow` has no nav guard; `StatusSidebar.tsx:92` uses raw `target="_blank"` |
| R9 | No **new** `try`/`catch` in committed code (use `neverthrow`/result shape) — pre-existing ones (e.g. `highlighter.ts`, `useWebSocket.ts`) are baseline, untouched | repo convention |
| R10 | Match design tokens; extensible for Phase-3 custom components | repo convention |

**Explicit non-goals (v1):** GitHub reference autolinking — `#123`, `owner/repo#123`, `GH-123`, bare commit SHAs, `@mentions` — render as **plain text**. `remark-gfm` does not implement these; they need a future repo-aware remark plugin (the documented Phase-3 extension point, routed through the same `ExternalLink`). PR *file* diffs (already met via Pierre) and PR *titles* (stay plain) are out of scope.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `electron/package.json` | modify | +4 deps |
| `electron/src/renderer/utils/highlighter.ts` | modify | register `diff`; export `REGISTERED_LANGUAGES` set (single source of truth) |
| `electron/src/renderer/styles/highlight.css` | create | hljs token colors: github (light) + `.dark` overrides |
| `electron/src/renderer/styles/globals.css` | modify | `@import "./highlight.css";` |
| `electron/src/renderer/utils/markdownSchema.ts` | create | sanitize schema: `defaultSchema` + `details`/`summary` + `open` attr |
| `electron/src/renderer/components/HighlightedCode.tsx` | create | shared `<pre><code>` highlight renderer (the one `dangerouslySetInnerHTML`); used by markdown `pre` AND `diff_hunk` |
| `electron/src/renderer/components/ExternalLink.tsx` | create | HTTPS-only link → `window.electron.openExternal`; one renderer-owned external-link primitive |
| `electron/src/renderer/utils/markdownComponents.tsx` | create | overrides: `pre` (block), `code` (inline), `a` (→ExternalLink), `details`, `summary` |
| `electron/src/renderer/components/Markdown.tsx` | create | memoized `<Markdown body compact?>` wrapper |
| `electron/src/main/window.ts` | modify | `setWindowOpenHandler` deny + `will-navigate` guard |
| `electron/src/renderer/components/StatusSidebar.tsx` | modify | migrate CI `target="_blank"` link → `ExternalLink` |
| `electron/src/renderer/components/CommentThread.tsx` | modify | swap `{comment.body}` → `<Markdown>` |
| `electron/src/renderer/components/UnresolvedThreadsView.tsx` | modify | swap `{comment.body}` → `<Markdown>`; `diff_hunk <pre>` → `<HighlightedCode lang="diff">` |
| `electron/src/renderer/pages/PRDetailPage.tsx` | modify | swap `pr.body` (`:318`) and `c.body` (`:352`) → `<Markdown>` |
| `electron/tests/unit/highlighter.test.ts` | modify | append diff-grammar + allowlist cases |
| `electron/tests/unit/markdownSchema.test.ts`, `ExternalLink.test.tsx`, `Markdown.test.tsx`, `externalLinks.guard.test.ts` | create | per-task tests (see conventions above) |

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
Expected: type-check PASS (packages ship their own types); react-markdown reports **v9 or higher**. The `pre`/`code` override split in Task 6 assumes the v9+ component model (no `inline` prop; `node` hast Element passed to components). If it resolves to <9, stop and reconcile before continuing.
- [ ] **Step 3 — Commit**
```bash
git add electron/package.json electron/pnpm-lock.yaml
git commit -m "chore: add react-markdown + GFM/raw/sanitize deps"
```

---

## Task 2: Register the `diff` grammar + export the language allowlist (TDD)

`utils/highlighter.ts` registers 12 languages — not `diff`. We add it and export the registered-language set so the overrides validate against one source of truth. **`tests/unit/highlighter.test.ts` already exists (env `node`) — append, do not recreate.**

**Files:** Modify `electron/src/renderer/utils/highlighter.ts`; modify `electron/tests/unit/highlighter.test.ts`

- [ ] **Step 1 — Append the failing cases** to the existing `tests/unit/highlighter.test.ts` (keep its `// @vitest-environment node` and existing import; add `REGISTERED_LANGUAGES` to the import):
```ts
import { detectLanguage, highlightCode, REGISTERED_LANGUAGES } from '../../src/renderer/utils/highlighter';

describe('diff grammar', () => {
  it('emits addition/deletion token classes', () => {
    const html = highlightCode('+added\n-removed\n context', 'diff');
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
  it('exposes diff in the registered-language allowlist', () => {
    expect(REGISTERED_LANGUAGES.has('diff')).toBe(true);
  });
});
```
- [ ] **Step 2 — Run; expect FAIL** (`pnpm test highlighter` — `diff` unregistered + `REGISTERED_LANGUAGES` undefined).
- [ ] **Step 3 — Implement** in `highlighter.ts`: add the import + registration, and export the allowlist (list the exact 12 already registered + `diff`).
```ts
import diff from 'highlight.js/lib/languages/diff';
// ...alongside the other registerLanguage calls:
hljs.registerLanguage('diff', diff);

/** Languages registered above — the single source of truth for the markdown overrides. */
export const REGISTERED_LANGUAGES = new Set([
  'typescript', 'javascript', 'go', 'python', 'json', 'css',
  'xml', 'bash', 'yaml', 'sql', 'markdown', 'rust', 'diff',
]);
```
- [ ] **Step 4 — Run; expect PASS** (`pnpm test highlighter`).
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/utils/highlighter.ts electron/tests/unit/highlighter.test.ts
git commit -m "feat: register diff grammar and export language allowlist"
```

---

## Task 3: hljs theme CSS (light + dark)

Without `.hljs-*` color rules, highlighted output is monospace-but-colorless. The app toggles dark mode via a `.dark` class on `<html>`, so we load github (light) as base and override with github-dark under `.dark` — `.dark .hljs-x` (specificity 0-2-0) beats base `.hljs-x` (0-1-0); light mode is unaffected because `.dark` is absent.

**Paste the file below verbatim — do not abridge.** Values copied exactly from `node_modules/highlight.js/styles/github.css` and `github-dark.css` (18 color rules each). hljs's structural `padding`/`display` rules are intentionally omitted (layout comes from the Tailwind classes on the `<pre>`), and `.hljs` `background` is forced `transparent` so the container's `bg-surface-*` shows through.

**Files:** Create `electron/src/renderer/styles/highlight.css`; modify `electron/src/renderer/styles/globals.css`

- [ ] **Step 1 — Create `electron/src/renderer/styles/highlight.css` with EXACTLY this content:**
```css
/* highlight.js token colors. Light base = github.css; dark overrides = github-dark.css
   (prefixed .dark — app toggles dark mode via a .dark class on <html>).
   Color values copied verbatim from node_modules/highlight.js/styles/*.css.
   Structural rules (padding/display) intentionally omitted — layout comes from
   the Tailwind classes on the <pre> element. .hljs background forced transparent
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
- [ ] **Step 2 — Import it** in `globals.css`. The file currently starts with `@import "tailwindcss";` — add immediately after that line:
```css
@import "./highlight.css";
```
- [ ] **Step 3 — Verify build & lint** (`pnpm lint && pnpm type-check` → PASS). CSS color correctness is verified visually in the final smoke test.
- [ ] **Step 4 — Commit**
```bash
git add electron/src/renderer/styles/highlight.css electron/src/renderer/styles/globals.css
git commit -m "feat: hljs theme CSS for fenced code blocks (light + dark)"
```

---

## Task 4: Sanitize schema — details/summary + `open` (TDD)

`rehype-sanitize`'s `defaultSchema` is GitHub's. It allows `className` matching `/^language-/` on `code` (so language survives) but not `details`/`summary`, nor the `open` attribute. `defaultSchema.attributes` is a `Record<tagName, allowed[]>`. Extend immutably — never mutate `defaultSchema`. The shipped schema is stricter than this minimal TDD step: it also removes image-bearing tags (`img`/`picture`/`source`) and narrows markdown links to `https` hrefs only.

**Files:** Create `electron/src/renderer/utils/markdownSchema.ts`; create `electron/tests/unit/markdownSchema.test.ts`

- [ ] **Step 1 — Failing test** (env `node` — no DOM needed):
```ts
// electron/tests/unit/markdownSchema.test.ts
// @vitest-environment node
import { describe, expect, it } from 'vitest';
import { markdownSchema } from '../../src/renderer/utils/markdownSchema';

describe('markdownSchema', () => {
  it('allows details and summary', () => {
    expect(markdownSchema.tagNames).toContain('details');
    expect(markdownSchema.tagNames).toContain('summary');
  });
  it('allows the open attribute on details', () => {
    expect(markdownSchema.attributes?.details).toContain('open');
  });
  it('does not allow script', () => {
    expect(markdownSchema.tagNames).not.toContain('script');
  });
  it('keeps GitHub protocol restrictions', () => {
    expect(markdownSchema.protocols?.href).toBeDefined();
  });
});
```
- [ ] **Step 2 — Run; expect FAIL** (`pnpm test markdownSchema` — module missing).
- [ ] **Step 3 — Implement**
```ts
// electron/src/renderer/utils/markdownSchema.ts
import { defaultSchema } from 'rehype-sanitize';

/**
 * Sanitize schema for untrusted PR markdown. Starts from rehype-sanitize's
 * GitHub-derived defaultSchema and additively permits <details>/<summary> and
 * the `open` attribute on <details>. Nothing else is loosened. Built immutably —
 * defaultSchema is never mutated.
 */
export const markdownSchema = {
  ...defaultSchema,
  tagNames: [...(defaultSchema.tagNames ?? []), 'details', 'summary'],
  attributes: {
    ...defaultSchema.attributes,
    details: [...(defaultSchema.attributes?.details ?? []), 'open'],
  },
} as const;
```
- [ ] **Step 4 — Run; expect PASS.**
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/utils/markdownSchema.ts electron/tests/unit/markdownSchema.test.ts
git commit -m "feat: markdown sanitize schema with details/summary/open"
```

---

## Task 5: Shared `HighlightedCode` + `ExternalLink` primitives

Two small, reused primitives. `HighlightedCode` is the **only** place a `dangerouslySetInnerHTML` lives (safety reviewed once); used by the markdown `pre` override (Task 6) and the `diff_hunk` site (Task 11). `ExternalLink` is the one renderer-owned external-link primitive (markdown `a` + migrated `StatusSidebar`).

**Files:** Create `electron/src/renderer/components/HighlightedCode.tsx`, `electron/src/renderer/components/ExternalLink.tsx`; create `electron/tests/unit/ExternalLink.test.tsx`

- [ ] **Step 1 — `HighlightedCode.tsx`**
```tsx
import { highlightCode, REGISTERED_LANGUAGES } from '../utils/highlighter';

interface HighlightedCodeProps {
  code: string;
  /** A language label (e.g. "ts", "diff"); null/unknown → plain escaped block. */
  lang: string | null;
}

/**
 * Renders a block of code inside <pre><code>. When `lang` is a registered
 * highlight.js grammar, applies hljs token markup; otherwise renders escaped
 * plaintext. The single dangerouslySetInnerHTML in the markdown stack.
 *
 * SAFETY: `code` is always a plain string (text only — never raw HTML), and
 * `highlightCode` escapes its input (hljs.highlight escapes; the null/error path
 * calls escapeHtml — highlighter.ts). Emitted HTML therefore contains only hljs
 * <span> markup over escaped text. `lang` is validated against REGISTERED_LANGUAGES
 * before it reaches the className, so a hostile language label cannot inject markup.
 */
export function HighlightedCode({ code, lang }: HighlightedCodeProps): React.JSX.Element {
  const preClass = 'overflow-x-auto rounded bg-surface-container-low p-2 font-mono text-xs';
  if (lang === null || !REGISTERED_LANGUAGES.has(lang)) {
    return (
      <pre className={preClass}>
        <code>{code}</code>
      </pre>
    );
  }
  const html = highlightCode(code, lang);
  return (
    <pre className={preClass}>
      {/* biome-ignore lint/security/noDangerouslySetInnerHtml: hljs output over pre-escaped text; lang allowlisted */}
      <code className={`hljs language-${lang}`} dangerouslySetInnerHTML={{ __html: html }} />
    </pre>
  );
}
```
- [ ] **Step 2 — `ExternalLink.tsx`** (no `try/catch`; `openExternal` resolves `{ ok } | { err }`, never rejects)
```tsx
interface ExternalLinkProps {
  href?: string;
  children: React.ReactNode;
  className?: string;
}

/**
 * The single renderer-owned external-link primitive. Prevents in-app navigation
 * and opens HTTPS URLs in the OS browser via the preload bridge (main validates
 * HTTPS-only — main/index.ts). Non-https / relative / javascript: hrefs are inert.
 * `window.electron` is optional so jsdom tests render without a bridge.
 */
export function ExternalLink({ href, children, className }: ExternalLinkProps): React.JSX.Element {
  function handleClick(event: React.MouseEvent<HTMLAnchorElement>): void {
    event.preventDefault();
    if (href === undefined || !href.startsWith('https://')) return;
    void window.electron?.openExternal(href);
  }
  return (
    <a href={href} onClick={handleClick} rel="noopener noreferrer" className={className}>
      {children}
    </a>
  );
}
```
- [ ] **Step 3 — `tests/unit/ExternalLink.test.tsx`** (DOM → jsdom docblock + matchers)
```tsx
// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { fireEvent, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ExternalLink } from '../../src/renderer/components/ExternalLink';

expect.extend(matchers);
afterEach(() => { vi.unstubAllGlobals(); });

function stubBridge() {
  const openExternal = vi.fn().mockResolvedValue({ ok: true });
  vi.stubGlobal('window', { ...window, electron: { openExternal } });
  return openExternal;
}

describe('<ExternalLink>', () => {
  it('opens https links via the bridge and prevents navigation', () => {
    const openExternal = stubBridge();
    const { getByText } = render(<ExternalLink href="https://example.com">go</ExternalLink>);
    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    fireEvent(getByText('go'), event);
    expect(openExternal).toHaveBeenCalledWith('https://example.com');
    expect(event.defaultPrevented).toBe(true);
  });
  it.each(['http://example.com', '/relative/path', 'javascript:alert(1)'])(
    'does not open non-https href %s', (href) => {
      const openExternal = stubBridge();
      const { getByText } = render(<ExternalLink href={href}>x</ExternalLink>);
      fireEvent.click(getByText('x'));
      expect(openExternal).not.toHaveBeenCalled();
    },
  );
});
```
- [ ] **Step 4 — Run; expect PASS** (`pnpm test ExternalLink`).
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/components/HighlightedCode.tsx \
        electron/src/renderer/components/ExternalLink.tsx \
        electron/tests/unit/ExternalLink.test.tsx
git commit -m "feat: HighlightedCode + ExternalLink primitives"
```

---

## Task 6: Element overrides (block `pre` vs inline `code`, links, details)

The correctness core. react-markdown renders fenced **and** indented code blocks as `pre > code`, inline code as a bare `code`. Override **`pre`** for blocks, **`code`** for inline. The `pre` override reads the HAST `node` (not rendered React children — with `rehype-raw` a raw `<pre><code><span>…` yields element children, not one text node), extracts **text only**, reads the language from the `code` element's class, and delegates to `HighlightedCode`. Because the `pre` override builds its own output and ignores `children`, react-markdown's `code` output for block code is discarded — so `code` effectively fires for inline only.

**Files:** Create `electron/src/renderer/utils/markdownComponents.tsx`

- [ ] **Step 1 — Implement**
```tsx
import type { Components } from 'react-markdown';
import { ExternalLink } from '../components/ExternalLink';
import { HighlightedCode } from '../components/HighlightedCode';

/** Minimal structural view of a hast node (avoids a direct `hast` type dep). */
interface HastNode {
  type: string;
  tagName?: string;
  value?: string;
  properties?: { className?: unknown };
  children?: HastNode[];
}

function languageFromClassName(className: string): string | null {
  const match = /language-([\w-]+)/.exec(className);
  return match?.[1] ?? null;
}

function classNameOf(node: HastNode): string {
  const value = node.properties?.className;
  if (Array.isArray(value)) return value.join(' ');
  return typeof value === 'string' ? value : '';
}

/** Concatenate text-node values only — element wrappers (e.g. hljs spans) are skipped. */
function textOf(node: HastNode): string {
  if (node.type === 'text') return node.value ?? '';
  if (!node.children) return '';
  return node.children.map(textOf).join('');
}

export const markdownComponents: Components = {
  // Block code: fenced (```lang, ```, ```diff) AND indented blocks arrive as pre>code.
  pre({ node, children }) {
    const codeNode = (node as HastNode | undefined)?.children?.[0];
    if (!codeNode || codeNode.type !== 'element' || codeNode.tagName !== 'code') {
      return <pre className="overflow-x-auto rounded bg-surface-container-low p-2">{children}</pre>;
    }
    const lang = languageFromClassName(classNameOf(codeNode));
    return <HighlightedCode code={textOf(codeNode)} lang={lang} />;
  },
  // True inline code only (a `code` node not wrapped by the `pre` override above).
  code({ children, className }) {
    return (
      <code className={`rounded bg-surface-container px-1 py-0.5 font-mono text-xs ${className ?? ''}`}>
        {children}
      </code>
    );
  },
  a({ href, children }) {
    return <ExternalLink href={typeof href === 'string' ? href : undefined}>{children}</ExternalLink>;
  },
  details({ children }) {
    return <details className="my-2 rounded border border-outline/30 px-2 py-1">{children}</details>;
  },
  summary({ children }) {
    return <summary className="cursor-pointer select-none font-medium">{children}</summary>;
  },
};
```
> **Tokens:** `globals.css` `@theme` defines `--color-surface-container`, `--color-surface-container-low`, `--color-on-surface-variant`; for the border the token is `--color-outline`, so `border-outline` resolves to `border-color: var(--color-outline)` in Tailwind v4. All valid; behavior doesn't depend on exact classes.
> **Whitespace caveat:** if raw HTML has a space between `<pre>` and `<code>` (`<pre> <code>…`), `node.children[0]` is a text node and the override falls back to the plain `<pre>{children}</pre>` branch (unhighlighted but correct). Negligible for real GitHub content; the test in Task 8 uses the standard no-space form.
- [ ] **Step 2 — Type-check** (`pnpm type-check` → PASS).
- [ ] **Step 3 — Commit**
```bash
git add electron/src/renderer/utils/markdownComponents.tsx
git commit -m "feat: markdown overrides — block/inline code split, links, details"
```

---

## Task 7: The `<Markdown>` component (memoized)

A pure memoized wrapper. `React.memo` avoids re-parsing the same body across re-renders; module-level plugin/component arrays keep memo effective.

> No body-size guard: GitHub hard-caps PR/issue/comment bodies at 65,536 chars, bodies arrive complete over WS (no streaming), and the render sites are non-virtualized — so there is no oversized-body or first-parse-stall case to guard against. `ponytail: parsing ~60KB is sub-10ms; if a real scroll problem ever appears, add a CSS max-height clamp on the wrapper — not a JS placeholder.`

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
// Order matters: rehypeRaw parses raw HTML into the tree BEFORE rehypeSanitize narrows it.
const REHYPE_PLUGINS = [rehypeRaw, [rehypeSanitize, markdownSchema]] as const;

interface MarkdownProps {
  body: string;
  /** Tighter spacing for inline comment contexts vs. roomy PR descriptions. */
  compact?: boolean;
  className?: string;
}

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
- [ ] **Step 2 — Type-check** (PASS). No commit yet — paired with tests in Task 8.

---

## Task 8: Markdown tests (the safety net)

**Files:** Create `electron/tests/unit/Markdown.test.tsx` (DOM → jsdom docblock + matchers)

- [ ] **Step 1 — Failing tests**
```tsx
// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Markdown } from '../../src/renderer/components/Markdown';

expect.extend(matchers);

describe('<Markdown> rendering', () => {
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
  it('highlights a fenced typescript block as a <pre> block', () => {
    const { container } = render(<Markdown body={'```ts\nconst x = 1;\n```'} />);
    const code = container.querySelector('pre code.language-ts');
    expect(code).not.toBeNull();
    expect(code?.innerHTML).toContain('hljs-');
  });
  it('highlights a fenced diff block with add/remove classes', () => {
    const { container } = render(<Markdown body={'```diff\n+new\n-old\n```'} />);
    const html = container.querySelector('pre code.language-diff')?.innerHTML ?? '';
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
  it('renders an UNLABELED fence as a <pre><code> block, not inline', () => {
    const { container } = render(<Markdown body={'```\nplain text\n```'} />);
    expect(container.querySelector('pre')).not.toBeNull();
    expect(container.querySelector('pre code')?.textContent).toContain('plain text');
  });
  it('renders an INDENTED code block as a <pre><code> block, not inline', () => {
    const { container } = render(<Markdown body={'    indented code line'} />);
    expect(container.querySelector('pre code')?.textContent).toContain('indented code line');
  });
  it('renders inline code as a bare <code> (no <pre>)', () => {
    const { container } = render(<Markdown body={'use `npm test` now'} />);
    const inline = container.querySelector('code');
    expect(inline).not.toBeNull();
    expect(inline?.closest('pre')).toBeNull();
  });
  it('extracts text from a raw <pre><code><span> structure (HAST, not React children)', () => {
    // NOTE: lang label must be a REGISTERED language — use `language-typescript`,
    // NOT `language-js` (only `javascript`/`typescript` etc. are registered; `js` is not an alias).
    const { container } = render(
      <Markdown body={'<pre><code class="language-typescript"><span>const</span> y</code></pre>'} />,
    );
    const code = container.querySelector('pre code.language-typescript');
    expect(code?.innerHTML).toContain('hljs-'); // re-highlighted from extracted text
    expect(code?.textContent).toContain('const');
  });
  it('renders <details open><summary> and retains open; nested ```diff colorizes', () => {
    const { container } = render(
      <Markdown body={'<details open><summary>more</summary>\n\n```diff\n+a\n-b\n```\n\n</details>'} />,
    );
    const details = container.querySelector('details');
    expect(details).not.toBeNull();
    expect(details?.hasAttribute('open')).toBe(true);
    expect(container.querySelector('summary')?.textContent).toBe('more');
    expect(container.querySelector('details pre code.language-diff')).not.toBeNull();
  });
  it('keeps GitHub refs/mentions as plain text (v1 non-goal)', () => {
    const { container } = render(<Markdown body={'see #123 and @octocat'} />);
    expect(container.querySelectorAll('a').length).toBe(0);
    expect(container.textContent).toContain('#123');
    expect(container.textContent).toContain('@octocat');
  });
});

describe('<Markdown> security (raw-HTML path is the highest-risk surface)', () => {
  it('strips <script>', () => {
    const { container } = render(<Markdown body={'<script>alert(1)</script>hello'} />);
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('hello');
  });
  it('strips event-handler attributes', () => {
    const { container } = render(<Markdown body={'<img src="x" onerror="alert(1)" />'} />);
    expect(container.querySelector('img')?.getAttribute('onerror')).toBeNull();
  });
  it('neutralizes javascript: links', () => {
    const { container } = render(<Markdown body={'[click](javascript:alert(1))'} />);
    expect((container.querySelector('a')?.getAttribute('href') ?? '').startsWith('javascript:')).toBe(false);
  });
  it('does not break out of a code fence via </code><script>', () => {
    const { container } = render(<Markdown body={'```\n</code><script>alert(1)</script>\n```'} />);
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('<script>');
  });
});
```
- [ ] **Step 2 — Run; iterate to green** (`pnpm test Markdown`). Likely fix points if red: schema tags/attrs (Task 4), language class on `code`, plugin order (`rehypeRaw` before `rehypeSanitize`), `pre` text extraction. Fix implementation, not tests (unless a test is wrong).
- [ ] **Step 3 — Commit**
```bash
git add electron/src/renderer/components/Markdown.tsx electron/tests/unit/Markdown.test.tsx
git commit -m "feat: memoized Markdown renderer with block/inline, details, size-guard, XSS coverage"
```

---

## Task 9: Main-process navigation hardening

`createWindow` has no navigation guards. Add a window-open deny and a `will-navigate` guard so untrusted links can never navigate the renderer or spawn windows. The hash router (`router.tsx:304`) makes in-app routing same-document → no `will-navigate`, so legitimate routing is unaffected; the dev-server URL is allowed so Vite HMR full-reloads work in dev.

> Coverage note: `src/main/window.ts` is in the `test:coverage` `include`, but `pnpm test` (the verification command) does not run the coverage gate, and `window.ts` has no coverage today — so this edit does not fail verification. Electron `BrowserWindow`/`webContents` is not mocked anywhere in the repo; a unit test would require new infrastructure out of proportion to a 4-line guard. Verified manually in Step 3 instead.

**Files:** Modify `electron/src/main/window.ts`

- [ ] **Step 1 — Add guards** right after `mainWindow = new BrowserWindow({...})` (before `loadURL`/`loadFile`):
```ts
// Block renderer-initiated navigation and window.open. External links go through
// the HTTPS-validated open-external IPC handler instead (see main/index.ts).
// Hash-router route changes are same-document and never trigger will-navigate.
mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
mainWindow.webContents.on('will-navigate', (event, url) => {
  if (url !== MAIN_WINDOW_VITE_DEV_SERVER_URL) {
    event.preventDefault();
  }
});
```
- [ ] **Step 2 — Verify build** (`pnpm type-check && pnpm lint` → PASS). `MAIN_WINDOW_VITE_DEV_SERVER_URL` is already declared in this file (`window.ts:7`); in a packaged build it is `undefined`, so every navigation is denied (the initial `loadFile` is not a `will-navigate`).
- [ ] **Step 3 — Manual verification** (no Electron test harness in this repo): `cd electron && pnpm start`; click a markdown link → opens in OS browser, app does not navigate; in-app keyboard routing still works.
- [ ] **Step 4 — Commit**
```bash
git add electron/src/main/window.ts
git commit -m "feat: deny renderer navigation and window.open in main process"
```

---

## Task 10: Migrate StatusSidebar CI links + add the regression guard

`StatusSidebar.tsx:89-97` renders CI-check links as raw `<a href={check.url} target="_blank">`. The Task 9 deny-guard would otherwise break these — migrate to `ExternalLink`, and add a grep guard so no renderer-owned `target="_blank"` reappears.

> The existing `tests/unit/StatusSidebar.test.tsx:76-77` asserts `getByRole('link', { name: 'ci-build' })` + `toHaveAttribute('href', 'https://ci.example.com/1')`. `ExternalLink` still renders `<a href>`, so this passes unchanged — run it to confirm, don't rewrite it.

**Files:** Modify `electron/src/renderer/components/StatusSidebar.tsx`; create `electron/tests/unit/externalLinks.guard.test.ts`

- [ ] **Step 1 — Write the failing guard test** (env `node`):
```ts
// electron/tests/unit/externalLinks.guard.test.ts
// @vitest-environment node
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return walk(full);
    return /\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry) ? [full] : [];
  });
}

describe('renderer external-link hygiene', () => {
  it('has no raw target="_blank" anchors (use ExternalLink instead)', () => {
    const rendererDir = join(__dirname, '../../src/renderer');
    const offenders = walk(rendererDir).filter((file) =>
      readFileSync(file, 'utf8').includes('target="_blank"'),
    );
    expect(offenders).toEqual([]);
  });
});
```
- [ ] **Step 2 — Run; expect FAIL** (`StatusSidebar.tsx` still has `target="_blank"`).
- [ ] **Step 3 — Migrate** the CI-check anchor in `StatusSidebar.tsx` to `<ExternalLink href={check.url} className={...}>…</ExternalLink>` (preserve existing classes/children/accessible name; add `import { ExternalLink } from './ExternalLink';`). Remove the `target="_blank"`/`rel` props (ExternalLink owns those).
- [ ] **Step 4 — Run; expect PASS** (`pnpm test externalLinks.guard` AND `pnpm test StatusSidebar`). Then `pnpm type-check && pnpm lint`.
- [ ] **Step 5 — Commit**
```bash
git add electron/src/renderer/components/StatusSidebar.tsx \
        electron/tests/unit/externalLinks.guard.test.ts
git commit -m "refactor: route StatusSidebar CI links through ExternalLink + guard"
```

---

## Task 11: Wire markdown into the four body sites + the `diff_hunk`

Pure display swaps. Edit-mode `<textarea>` paths stay raw text; only display paths change. Preserve existing empty/blank guards. The `diff_hunk` is a raw unified hunk (not markdown) → render with `HighlightedCode lang="diff"`, **no truncation** (review context must stay intact).

**Files:** `CommentThread.tsx:133`, `UnresolvedThreadsView.tsx:345` & `:349`, `PRDetailPage.tsx:318` & `:352`

- [ ] **Step 1 — CommentThread**: replace the display `<p>…{comment.body}…</p>` with `<Markdown body={comment.body} compact />`; add `import { Markdown } from './Markdown';`.
- [ ] **Step 2 — UnresolvedThreadsView (body)**: replace `{comment.body}` with `<Markdown body={comment.body} compact />`; add the import.
- [ ] **Step 3 — UnresolvedThreadsView (diff_hunk)**: replace the `<pre>{comment.diff_hunk}</pre>` at `:345` with `<HighlightedCode code={comment.diff_hunk} lang="diff" />`; add `import { HighlightedCode } from './HighlightedCode';`. **Keep the existing blank guard** (`comment.diff_hunk !== undefined && comment.diff_hunk !== ''`, `:343`). No cap — full hunk.
- [ ] **Step 4 — PRDetailPage**: replace `{pr.body}` (`:318`) with `<Markdown body={pr.body} />` and `{c.body}` (`:352`) with `<Markdown body={c.body} compact />`; add `import { Markdown } from '../components/Markdown';`. Keep the `pr.body === null || trim === ''` guard at `:313`.
- [ ] **Step 5 — Full verification**
```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```
Expected: PASS. `tests/unit/PRDetailPage.test.tsx` asserts on the PR title / "view diff" (not body text), so the swaps don't break it; if any future assertion checks exact body DOM, query rendered text instead.
- [ ] **Step 6 — Commit**
```bash
git add electron/src/renderer/components/CommentThread.tsx \
        electron/src/renderer/components/UnresolvedThreadsView.tsx \
        electron/src/renderer/pages/PRDetailPage.tsx
git commit -m "feat: render PR descriptions, comments, and diff hunks as markdown"
```

---

## Phase-3 readiness (no work now)

GitHub refs/mentions (`#123`, `owner/repo#123`, `@user`, SHAs) are a deliberate **v1 non-goal** (plain text). When wanted, add a small repo-aware **remark plugin** (the documented unified extension point) that emits link nodes routed through the existing `ExternalLink` — no renderer/architecture change. Tests would then flip the "refs stay plain text" assertion to link assertions. Nothing speculative is built now.

## Final verification

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test   # Go side untouched — no go vet/test needed
```
Manual smoke: open a PR whose description has a table, a ` ```ts ` block, an unlabeled fence, and a `<details open>` containing a nested ` ```diff `. Confirm: table renders; code blocks are colored in light AND dark mode; the unlabeled fence is a `<pre>` block (not inline); diff lines are green/red; `<details>` starts open; a review comment shows its `diff_hunk` colored above the body; a markdown link opens in the OS browser (app does not navigate); a comment containing `<script>` shows the text but executes nothing.
