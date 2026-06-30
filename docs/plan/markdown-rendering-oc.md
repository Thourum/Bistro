# Spec: Markdown Rendering for PR Descriptions & Comments (Round 2)

Status: **DRAFT — awaiting approval**
Owner: UI / renderer
Related: `docs/ROADMAP.md` (Phase 3), `docs/DESIGN.md`

> This revision resolves every open question from the prior draft, corrects a
> factual error about the ` ```diff ` path, folds in the council's recommendations
> (memoization, stable module-level arrays, explicit XSS tests, single shared renderer,
> no dangling decision points), and has been hardened against an adversarial review
> (Electron link-handling boundary, complete 4-site integration inventory, react-markdown
> v10 inline-code semantics, input-size limits, raw-HTML test coverage). All claims are
> grounded in code (`file:line`) and the Pierre docs.

---

## 1. Library decision (settled)

Use **`react-markdown`** (v10) with `remark-gfm` + `rehype-raw` + `rehype-sanitize`. **Reject `streamdown`.**

- We render **complete** comment/description strings over the WebSocket, not in-progress AI token streams — `streamdown`'s incomplete-stream repair is irrelevant.
- `react-markdown` renders **React elements, not `dangerouslySetInnerHTML`** — safe-by-default for untrusted GitHub content in a Chromium renderer.
- The `components` prop is the exact escape hatch for `code`/`details`/`summary`, and remark/rehype is the documented Phase-3 extension path.
- `streamdown` bundles Shiki + Mermaid + Tailwind helpers we'd fight; `react-markdown` is ~10 small unified packages, no native deps, no CSS.

---

## 2. The diff-rendering architecture (the decisive section)

Per your clarification, there are **two distinct diff surfaces**, and they use **different engines**:

### Lane A — App-generated patches → **Pierre** (already in place)

Flow (verified):

```
go-git UnifiedEncoder  (go/internal/git/diff.go: filePatchToString)
  → per-file unified diff: "--- a/x / +++ b/x / @@ …"  (no diff --git yet)
  → stored per file in pr_diffs                     (store/diff_store.go, writer.go:605)
DiffStore.GetUnifiedDiff  synthesizes "diff --git a/x b/x" headers, concatenates
  → multi-file "diff --git …" string                (store/diff_store.go:101–125)
WS REQUEST_DIFF → DIFF_READY { diff, files }        (server/handler/diff.go)
  → DiffPage setDiff(payload.diff)                  (pages/DiffPage.tsx:110–122)
  → <PierreDiffView diff={diff} />                  (splits on "diff --git", feeds
    each per-file chunk to Pierre <PatchDiff>)       (PierreDiffView.tsx:81,351,419)
```

**This already renders generated patches through Pierre.** No change required for the
standard PR diff. Pierre's documented input contract matches: `parsePatchFiles` ingests a
unified-diff string; `PatchDiff` wraps a single-file patch; `CodeView`/`MultiFileDiff` for
multi-file. (Pierre also offers `parseDiffFromFile(oldContents, newContents)` if we ever
render from blobs instead of a patch string.)

**Adjacent observation (OUT OF SCOPE for this spec, flagged for a follow-up):**
range / `sinceLastReview` / force-push diffs are rendered by `RangeDiffView.tsx` with a
**hand-rolled line-prefix `<pre>`**, *not* Pierre, and the non-degraded `git range-diff`
output (`++`/`--` nested format, `types/index.ts` `RangeDiffResult.raw`) is **not**
Pierre-compatible without a transform (`server/handler/rangediff.go:215`). Migrating
RangeDiffView to Pierre is a separate ticket; **this spec does not touch `RangeDiffView`
and does not share code with it.** `RangeDiffView` has a richer classifier (it special-cases
`@@`, `##`, `++`, `--`, `diff `, `index ` and renders a line-numbered table —
`RangeDiffView.tsx:16–29`); collapsing it into Lane B's simpler colorizer would regress that
fidelity. Lane B's `DiffLines` (below) reuses only the **token palette** for visual
consistency, not the component.

### Lane B — ` ```diff ` embedded in markdown → **lightweight line-prefix colorizer** (NOT Pierre)

A ` ```diff ` fence inside a comment / PR description is a **bare snippet** (just
`+`/`-`/context lines, sometimes a `@@` header) — it is **not** a git patch. Three reasons
this must NOT go through Pierre:

1. **Format mismatch.** Pierre's `getSingularPatch` throws unless the blob starts with
   `--- `/`+++ ` headers; a bare fence would require synthesizing a fake `--- a/x\n+++ b/x\n`
   preamble per block — fragile and meaningless for a 5-line snippet.
2. **Weight.** `@pierre/diffs` bundles Shiki (~5.6 MB unpacked) and renders async via a
   worker pool — wrong tradeoff for inline snippets in virtualized comment rows.
3. **Your stated intent.** Pierre is for real generated patches; markdown-embedded diffs
   should use lightweight highlighting.

**Correction of the prior draft / CC plan:** both assumed ` ```diff ` could ride
highlight.js's `diff` grammar. **`diff` is NOT registered** in
`utils/highlighter.ts` (only 12 langs: ts, js, go, py, json, css, xml, bash, yaml, sql,
markdown, rust — `highlighter.ts:1–26`). Passing `'diff'` to `highlightCode` throws → the
`catch` returns escaped plaintext → **no coloring**. So the highlight.js-diff path is a
silent no-op as written.

**Decision (D1, settled):** render ` ```diff ` with a **new, self-contained, zero-dependency
line-prefix colorizer** — the same line-prefix model GitHub uses. It is used in **two**
markdown-adjacent places only:
1. markdown ` ```diff ` fences (Lane B), and
2. the `diff_hunk` `<pre>` in `UnresolvedThreadsView.tsx:345` (a standard unified hunk:
   `@@`/`+`/`-`/context — fully covered by the classifier below).

It is **not** wired into `RangeDiffView` (see Lane A note). `diff_hunk` may legitimately be
empty/absent — render nothing when blank (guarded today at `UnresolvedThreadsView.tsx:343`).

```tsx
// components/diff/DiffLines.tsx — self-contained, 0 deps, synchronous, token-styled
const DIFF_LINE = (line: string): string =>
  line.startsWith('+') ? 'text-primary bg-primary/10'
  : line.startsWith('-') ? 'text-error bg-error/10'
  : line.startsWith('@@') ? 'text-on-surface-variant'
  : 'text-on-surface-variant';

// maxLines is opt-in: markdown fences pass a cap; diff_hunk passes none (full context).
export function DiffLines({ patch, maxLines }: { patch: string; maxLines?: number }) {
  const all = patch.replace(/\n$/, '').split('\n');
  const [expanded, setExpanded] = useState(false);
  const cap = maxLines ?? Infinity;
  const truncated = !expanded && all.length > cap;
  const lines = truncated ? all.slice(0, cap) : all;
  return (
    <pre className="font-mono text-[11px] leading-snug overflow-x-auto …tokens…">
      {lines.map((l, i) => (
        <div key={i} className={DIFF_LINE(l)}>{l || '\u00A0'}</div>
      ))}
      {truncated && (
        <button type="button" onClick={() => setExpanded(true)}
          className="text-on-surface-variant hover:text-on-surface">
          … show {all.length - cap} more lines
        </button>
      )}
    </pre>
  );
}
```

- Markdown ` ```diff ` fences pass `maxLines={MAX_DIFF_LINES}` (e.g. 200) → pathological
  fences collapse with an expander.
- `comment.diff_hunk` passes **no** `maxLines` → GitHub review hunks render in full (no
  silent loss of inline-comment context).

Uses Bistro design tokens (`text-primary` add, `text-error` delete) — matches the diff
palette already used in `PierreDiffView` (`+{additions}` primary / `-{deletions}` error).

---

## 3. Current state (grounded)

- No markdown deps installed (`electron/package.json`: no `react-markdown`/`remark`/`rehype`/sanitizer).
- Raw-text body sites today (the full inventory — **4 sites**):
  - `pages/PRDetailPage.tsx:318` — `{pr.body}` (PR description)
  - `pages/PRDetailPage.tsx:351–352` — `{c.body}` (**Recent Comments** card, `comments.slice(0,5)` — easy to miss; renders raw text in the PR-detail status rail)
  - `components/UnresolvedThreadsView.tsx:349` — `{comment.body}`; `:345` — `<pre>{comment.diff_hunk}</pre>`
  - `components/CommentThread.tsx:133` — `<p>{comment.body}</p>`
- `utils/highlighter.ts` — `highlightCode(code, lang)` over `highlight.js/lib/core`, 12 langs registered, **idle** (no JSX caller). `diff` NOT registered.

---

## 4. Dependencies to add

```bash
cd electron && pnpm add react-markdown remark-gfm rehype-raw rehype-sanitize
```

No new highlighter (reuse `highlighter.ts`). No Shiki. No `@tailwindcss/typography`.

---

## 5. Design

### New files

```
electron/src/renderer/components/markdown/
  Markdown.tsx           # shared renderer: memoized, module-level plugin/component arrays
  markdownComponents.tsx # code / a / details / summary overrides
  markdownSchema.ts      # rehype-sanitize schema (extends defaultSchema)
electron/src/renderer/components/diff/
  DiffLines.tsx          # shared line-prefix colorizer (Lane B)
```

### `Markdown.tsx` — performance-hardened (council fold-in)

```tsx
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import rehypeSanitize from 'rehype-sanitize';
import { mdComponents } from './markdownComponents';
import { githubSchema } from './markdownSchema';

// Module-level constants → stable references → React.memo actually holds.
const REMARK_PLUGINS = [remarkGfm];
const REHYPE_PLUGINS = [rehypeRaw, [rehypeSanitize, githubSchema]] as const;

export const Markdown = React.memo(function Markdown(
  { body, compact = false }: { body: string; compact?: boolean },
) {
  return (
    <div className={compact ? 'md md--compact' : 'md'}>
      <ReactMarkdown
        remarkPlugins={REMARK_PLUGINS}
        rehypePlugins={REHYPE_PLUGINS}
        components={mdComponents}
      >{body}</ReactMarkdown>
    </div>
  );
});
```

- `React.memo` keyed on `body`/`compact` — comment rows mount/unmount in the virtualized
  thread list; we parse each body once.
- Plugins/components/schema are **module-level** so memo isn't defeated by fresh array
  identities each render.

### `markdownComponents.tsx` — element overrides

**react-markdown v10 note:** the `inline` prop on the `code` component was **removed** in
v9+. "No `language-*` class" does **not** mean inline — unlabeled fenced blocks and indented
code blocks also lack a language class. Block vs. inline must therefore be distinguished by
**position in the tree**, not by the class. We handle **block** code at the `pre` override
and reserve the `code` override for **true inline** code.

- **`pre`** (block code): the override receives the HAST `node` (react-markdown passes
  `node` to every component). **Do not rely on rendered React children** — with `rehype-raw`
  enabled a raw `<pre><code class="language-js"><span>…</span></code></pre>` can yield
  element children, not a single text node. The override must:
  1. read `node` and verify its first child is a `code` element; otherwise fall back to a
     plain `<pre>{children}</pre>`;
  2. **recursively extract text only** from the `code` element's descendants (concatenate
     `value` of text nodes; ignore element wrappers) → a single `string`;
  3. read the language from the `code` element's `className` (`language-<lang>`).
  Then dispatch on language:
  - `language-diff` → `<DiffLines patch={text} maxLines={MAX_DIFF_LINES} />`.
  - `language-<lang>` where `<lang>` is registered in `highlighter.ts` → `highlightCode(text, lang)`
    rendered inside `<pre><code dangerouslySetInnerHTML />`.
  - unlabeled fence / indented block / unregistered lang → plain `<pre><code>{text}</code></pre>`
    (escaped plaintext; preserves `<pre>` whitespace/scroll — never collapses to inline).
  - **Safety invariant for the one `dangerouslySetInnerHTML`:** the highlighted input is the
    text-only string extracted in step 2 (never raw HTML), and `highlightCode` escapes all
    content (`hljs.highlight` escapes; the `null`/error path calls `escapeHtml`,
    `highlighter.ts:60–79`). Emitted HTML contains only `hljs` `<span class="hljs-…">` markup;
    `lang` is validated against the registered-language allowlist before use, so a hostile
    `className` cannot inject a tag or an unescaped value. Covered by explicit tests (§8).
- **`code`** (true inline only, i.e. a `code` node whose parent is not `pre`) → plain `<code>`.
- **`a`** → render a shared **`ExternalLink`** component (see below), NOT a bare browser
  anchor. It `preventDefault`s and routes through `window.electron?.openExternal(href)`
  (preload bridge `preload.ts:113`, typed `renderer/types/window.d.ts:41`; main validates
  **HTTPS-only** `main/index.ts:208–220`, same pattern as `useWebSocket.ts:312–321`). The
  result is neverthrow-shaped `{ ok }|{ err }` — no `try/catch`. Non-https / relative /
  `javascript:` hrefs: `preventDefault` + no-op. Keep `rel="noopener noreferrer"`.
- **`details` / `summary`** → styled collapsible using design tokens; the `open` attribute is
  **explicitly allowed** in the sanitize schema so GitHub bots' default-open sections keep
  their state. Children recurse through the same renderer, so nested ` ```diff ` inside
  `<details>` (the exact payload in the request) composes for free.

### Shared `ExternalLink` + main-process hardening (required, ships with this PR)

The link fix is incomplete without (a) a single reusable link primitive and (b) a
main-process backstop — **and** it must not regress existing non-markdown external links.

1. **New `components/ExternalLink.tsx`** — the one renderer-owned external-link primitive:
   `preventDefault` → validate `https://` → `window.electron?.openExternal(href)` (guarded
   for the jsdom/test case where `window.electron` is absent, like `useWebSocket.ts:312`).
2. **Migrate the existing browser anchor**: `StatusSidebar.tsx:90–97` currently renders CI
   check links as `<a href={check.url} target="_blank">`. A global window-open deny (below)
   would break these — migrate it to `<ExternalLink>` in this PR. Add a lint/grep guard so no
   renderer-owned `target="_blank"` / raw external anchor reappears.
3. **Main-process guards** in `window.ts:12–40` (`createWindow` currently has none):

```ts
mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
mainWindow.webContents.on('will-navigate', (e, url) => {
  // SPA uses createHashHistory (router.tsx:304) → in-app routing is hash-only, never a
  // top-level navigation, so this never blocks legitimate routing. Deny everything that
  // is not the dev-server URL (untrusted markdown links, CI links, etc.).
  if (url !== MAIN_WINDOW_VITE_DEV_SERVER_URL) e.preventDefault();
});
```

External opens then go **exclusively** through the HTTPS-validated `open-external` IPC
handler, for both markdown links and migrated app links.

### `markdownSchema.ts` — sanitize allowlist

Start from `rehype-sanitize` `defaultSchema` (GitHub's own schema), then narrowly extend:
- `tagNames`: add `details`, `summary`.
- `attributes`: allow `open` on `details`; allow the `className` `language-*` pattern on `code`.

Nothing else loosened. Raw HTML is opt-in via `rehype-raw` but always re-narrowed by sanitize
→ **XSS-safe by construction.** Note `rehype-raw` runs **before** `rehype-sanitize` in the
pipeline (raw HTML is parsed into HAST, then sanitized) — order matters and is fixed by the
`REHYPE_PLUGINS` array order above.

### Input-size limits (required — guards first-parse cost, not just re-render)

`React.memo` only avoids re-parsing the *same* body; it does nothing for a single huge body.
Add hard guards so one pathological PR description/comment cannot block the renderer or
desync virtualized rows:
- **Body cap:** if `body.length` exceeds a threshold (e.g. 100 KB), render a collapsed
  placeholder with an explicit "Show full content" expander instead of parsing eagerly.
- **DiffLines cap:** `MAX_DIFF_LINES` (200) with a truncation footer (shown above).
- **compact max-height:** in `compact` mode, clamp rendered height with a "Show more"
  expander so a tall comment can't produce an unusably tall card or desync row measurement.
These thresholds are tunable constants; values above are the proposed defaults.

### Error handling (neverthrow)

`react-markdown` does not throw on malformed markdown (renders best-effort) — no `Result`
wrapper needed around parsing. No new `try/catch` is introduced. The single pre-existing
`try/catch` in `highlighter.ts:64` is **left as-is** (out of scope; see §7 D3).

---

## 6. Integration (replace raw text, no logic change)

One shared `<Markdown>` at **all four** body sites:

1. `pages/PRDetailPage.tsx:318` (PR description) → `<Markdown body={pr.body} />`
2. `pages/PRDetailPage.tsx:351–352` (Recent Comments card) → `<Markdown body={c.body} compact />`
3. `components/UnresolvedThreadsView.tsx:349` → `<Markdown body={comment.body} compact />`;
   and `:345` `diff_hunk` `<pre>` → `<DiffLines patch={comment.diff_hunk} />`
4. `components/CommentThread.tsx:133` → `<Markdown body={comment.body} compact />`

Ship all four sites + the `ExternalLink` primitive + `StatusSidebar` link migration + the
main-process navigation guards together (single shared component; council-endorsed simpler
scope).

### Non-goals (explicitly scoped out)

- **GitHub reference autolinking** — `#123`, `owner/repo#123`, `GH-123`, bare commit SHAs,
  and `@mentions` are **not** rendered as links. `remark-gfm` does **not** implement these
  (it covers tables, task lists, strikethrough, literal-URL autolinks, footnotes only). They
  render as plain text in v1. This is a deliberate scope cut, not an oversight; a competing
  plan claiming "GitHub-fidelity" must still add a custom remark plugin to achieve it.
- **Phase-3 hook:** when refs/mentions are wanted, add a small repo-aware **remark plugin**
  (the documented unified extension point) that emits link nodes routed through the same
  `ExternalLink` component — no renderer/architecture change. Tests would then cover `#123`,
  `owner/repo#123`, `@user`, SHA refs.

**Virtualized-row caveat:** `PierreDiffView` virtual rows estimate height
(`PierreDiffView.tsx:279`). Markdown rendered inside virtualized comment contexts has
variable height — use `measureElement` (already wired) and/or a `max-height`+expand on
`compact` so unmeasured tall markdown doesn't desync row offsets.

---

## 7. Resolved decisions (formerly open)

- **D1 — ` ```diff ` engine:** RESOLVED → shared `DiffLines` colorizer (Lane B). Pierre is
  reserved for app-generated patches (Lane A). See §2.
- **D2 — typography:** RESOLVED → keep `font-mono` body to honor the terminal design
  system (`docs/DESIGN.md`); no `@tailwindcss/typography`. Style headings/lists/code via the
  `.md` scope mapped to existing tokens. (Revisit only if design asks for proportional prose.)
- **D3 — `highlighter.ts` try/catch:** RESOLVED → leave as-is; pre-existing, out of scope,
  not a new violation.
- **D4 — rollout scope:** RESOLVED → one shared renderer, **all four** sites + `ExternalLink`
  primitive + `StatusSidebar` migration + main-process navigation guards in one PR.
- **D5 — link handling:** RESOLVED → all renderer-owned external links (markdown + migrated
  app links) route through the HTTPS-only `window.electron.openExternal` bridge + main-process
  `setWindowOpenHandler`/`will-navigate` denial. Hash router (`router.tsx:304`) means the
  navigation guard never blocks in-app routing.
- **D6 — GitHub refs/mentions:** RESOLVED → explicitly a **non-goal** for v1 (see §6); future
  remark-plugin extension documented.

No open questions remain for the implementer.

---

## 8. Test plan (`Markdown.test.tsx`, Vitest)

Rendering:
- GFM: table, task list (`- [x]`), strikethrough render correctly.
- Fenced ` ```ts ` block → carries `hljs` token classes; rendered as a `<pre>` block.
- **Unlabeled fence** (``` ``` ```) and **indented code block** → render as `<pre><code>`
  block, NOT collapsed to inline `<code>` (guards the v10 inline-detection trap).
- Fenced ` ```diff ` block → `DiffLines` applies add/remove classes (explicit assertion;
  guards against the silent-plaintext regression both prior plans would have shipped).
- `DiffLines` with `maxLines` exceeded → expander shown; clicking expands to full content.
- `DiffLines` for `diff_hunk` (no `maxLines`) → renders **full** hunk, no truncation.
- **`pre` text extraction:** a raw `<pre><code class="language-js"><span>x</span></code></pre>`
  yields correct highlighted text (extraction walks HAST text nodes, not React children);
  malformed/unexpected child shape falls back to plain `<pre><code>` without crashing.
- `<details open><summary>…</summary>…</details>` → renders, is NOT stripped, retains
  `open`, and **nested ` ```diff ` inside it** colorizes. Test the common GitHub form with
  blank lines around the nested fence (the exact payload in the request).
- GitHub refs (`#123`, `owner/repo#123`, `@user`, SHA) → render as **plain text** (documents
  the v1 non-goal; flips to link assertions if/when the Phase-3 remark plugin lands).

Links (Electron boundary — D5):
- `https://` link → click calls `window.electron.openExternal`, `preventDefault` fires, no
  in-app navigation.
- `http:`, relative, and `javascript:` links → `preventDefault`, `openExternal` not invoked
  (or invoked and rejected by main); no navigation.
- **Regression:** migrated `StatusSidebar` CI link uses `ExternalLink` (no `target="_blank"`);
  grep guard asserts no renderer-owned raw external anchor remains.
- (main-process) `setWindowOpenHandler` returns `deny`; `will-navigate` blocks non-dev URLs
  but does NOT block hash-router in-app navigation.

XSS (raw-HTML path via `rehype-raw` is the highest-risk surface):
- `<script>alert(1)</script>` → stripped, not executed.
- `<img src=x onerror=alert(1)>` → `onerror` stripped.
- `[x](javascript:alert(1))` and raw `<a href="javascript:…" target="_blank">` → neutralized.
- Raw `<code class="language-js">…</code>` and **malformed/nested** raw HTML → sanitized, no
  tag injection; the `lang` allowlist check holds.
- Fenced code containing `</code><script>…` → escaped as text inside the `pre`/`code`
  override (verifies the one `dangerouslySetInnerHTML` cannot break out).

Performance:
- Re-render with identical `body` does not re-parse (memo holds; assert render count or
  stable child identity).
- Oversized `body` (> cap) renders the collapsed placeholder, not an eager full parse.
- Large table / many task-list items / large ` ```diff ` fence render within budget
  (smoke benchmark; no renderer stall).

---

## 9. Verification

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

This change is **Electron-renderer + main only** (new components + a navigation-guard tweak
in `main/window.ts`). The Go engine is untouched — no `go vet`/`go test` needed.
