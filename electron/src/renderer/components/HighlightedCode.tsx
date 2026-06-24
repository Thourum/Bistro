// biome-ignore-all lint/security/noDangerouslySetInnerHtml: this file is the reviewed hljs rendering boundary
import { highlightCode, REGISTERED_LANGUAGES } from '../utils/highlighter';

/**
 * Alias map: common fence shorthands → canonical hljs names.
 * Resolution is internal — callers pass the raw fence label (e.g. "ts", "diff").
 */
const FENCE_ALIAS: Record<string, string> = {
  ts: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  sh: 'bash',
  shell: 'bash',
  yml: 'yaml',
  md: 'markdown',
  py: 'python',
  rs: 'rust',
  golang: 'go',
};

interface HighlightedCodeProps {
  code: string;
  /** A language label (e.g. "ts", "diff"); null/unknown → plain escaped block. */
  lang: string | null;
}

/**
 * Renders a `<pre><code>` block. When `lang` resolves to a registered
 * highlight.js grammar the token markup is applied via dangerouslySetInnerHTML
 * (the single such site in the markdown stack); otherwise plain escaped text.
 *
 * SAFETY: `code` is always plain text — never raw HTML. `highlightCode` escapes
 * its input before returning; the null-path renders via React (auto-escaped).
 * `lang` is validated against REGISTERED_LANGUAGES so a hostile label cannot
 * inject markup.
 *
 * Used by the markdown `pre` override AND directly for `diff_hunk` snippets.
 */
export function HighlightedCode({
  code,
  lang,
}: HighlightedCodeProps): React.JSX.Element {
  const preClass =
    'overflow-x-auto rounded bg-surface-container-low p-2 font-mono text-xs';

  // Normalise alias → canonical name for highlighting; keep original for class
  const lower = lang?.toLowerCase() ?? null;
  const resolved = lower !== null ? (FENCE_ALIAS[lower] ?? lower) : null;
  const validLang =
    resolved !== null && REGISTERED_LANGUAGES.has(resolved) ? resolved : null;

  if (validLang === null) {
    return (
      <pre className={preClass}>
        <code>{code}</code>
      </pre>
    );
  }

  const html = highlightCode(code, validLang);
  return (
    <pre className={preClass}>
      <code
        className={`hljs language-${lang}`}
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </pre>
  );
}
