import type { Components } from 'react-markdown';
import { ExternalLink } from '../components/ExternalLink';
import { HighlightedCode } from '../components/HighlightedCode';

/** Minimal structural view of a HAST node (avoids a direct `hast` type dep). */
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

/**
 * Markdown element overrides — Minimal Light technical aesthetic.
 *
 * Geist body text, JetBrains Mono for code, square corners, 1px hairlines, the
 * one Bistro blue accent reserved for links and blockquote rails. Headings use
 * tight tracking in the display family; chrome-ish marks (hr, table head) lean
 * mono. No decoration, no rounded corners.
 *
 * Tag structure and highlight classes (`language-*`, `hljs-*`) are preserved
 * verbatim — styling rides on top of the same DOM the renderer already emits.
 */
export const markdownComponents: Components = {
  /**
   * Block code: fenced (```lang, ```, ```diff) AND indented blocks arrive as
   * pre>code in the HAST tree. We extract raw text + language and delegate to
   * HighlightedCode which renders the full <pre><code> block — no extra <pre>.
   */
  pre({ node, children }) {
    const hastNode = node as HastNode | undefined;
    const codeNode = hastNode?.children?.[0];
    if (
      !codeNode ||
      codeNode.type !== 'element' ||
      codeNode.tagName !== 'code'
    ) {
      // Fallback: non-code pre (rare in practice)
      return (
        <pre className="my-3 overflow-x-auto border border-outline-variant/40 bg-surface-container-low p-3">
          {children}
        </pre>
      );
    }
    const lang = languageFromClassName(classNameOf(codeNode));
    return <HighlightedCode code={textOf(codeNode)} lang={lang} />;
  },

  /**
   * True inline code only. Block code elements are handled entirely by the
   * `pre` override above — this branch fires for inline `code` nodes only.
   */
  code({ children, className }) {
    return (
      <code
        className={`border border-outline-variant/30 bg-surface-container px-1 py-0.5 font-mono text-[0.9em] text-on-surface ${className ?? ''}`}
      >
        {children}
      </code>
    );
  },

  /** All links route through ExternalLink (HTTPS-only, always preventDefault). */
  a({ href, children }) {
    return (
      <ExternalLink
        href={typeof href === 'string' ? href : undefined}
        className="text-primary underline decoration-primary/40 underline-offset-2 transition-colors duration-150 hover:decoration-primary"
      >
        {children}
      </ExternalLink>
    );
  },

  h1({ node: _node, children }) {
    return (
      <h1 className="mt-6 mb-3 text-xl font-bold tracking-tight text-on-surface first:mt-0">
        {children}
      </h1>
    );
  },
  h2({ node: _node, children }) {
    return (
      <h2 className="mt-6 mb-3 text-lg font-bold tracking-tight text-on-surface first:mt-0">
        {children}
      </h2>
    );
  },
  h3({ node: _node, children }) {
    return (
      <h3 className="mt-5 mb-2 text-base font-bold tracking-tight text-on-surface first:mt-0">
        {children}
      </h3>
    );
  },
  h4({ node: _node, children }) {
    return (
      <h4 className="mt-4 mb-2 font-mono text-[11px] font-bold uppercase tracking-widest text-outline first:mt-0">
        {children}
      </h4>
    );
  },

  p({ node: _node, children }) {
    return (
      <p className="my-2 leading-relaxed first:mt-0 last:mb-0">{children}</p>
    );
  },

  ul({ node: _node, children }) {
    return (
      <ul className="my-2 ml-5 list-disc space-y-1 marker:text-outline-variant">
        {children}
      </ul>
    );
  },
  ol({ node: _node, children }) {
    return (
      <ol className="my-2 ml-5 list-decimal space-y-1 marker:text-outline-variant">
        {children}
      </ol>
    );
  },
  li({ node: _node, children }) {
    return <li className="leading-relaxed">{children}</li>;
  },

  blockquote({ node: _node, children }) {
    return (
      <blockquote className="my-3 border-l-2 border-primary/50 bg-surface-container-low/60 py-1 pl-4 text-on-surface-variant">
        {children}
      </blockquote>
    );
  },

  hr() {
    return <hr className="my-4 border-0 border-t border-outline-variant/40" />;
  },

  table({ node: _node, children }) {
    return (
      <div className="my-3 overflow-x-auto border border-outline-variant/40">
        <table className="w-full border-collapse text-left">{children}</table>
      </div>
    );
  },
  thead({ node: _node, children }) {
    return (
      <thead className="bg-surface-container-low font-mono text-[11px] uppercase tracking-widest text-outline">
        {children}
      </thead>
    );
  },
  th({ node: _node, children }) {
    return (
      <th className="border-b border-outline-variant/40 px-3 py-1.5 font-bold">
        {children}
      </th>
    );
  },
  td({ node: _node, children }) {
    return (
      <td className="border-b border-outline-variant/20 px-3 py-1.5 align-top">
        {children}
      </td>
    );
  },

  /**
   * details: spread sanitized props (which includes `open` when allowed by
   * the schema) and add styling class. Exclude `node` — it is HAST-internal
   * and not a valid DOM attribute.
   */
  details({ node: _node, children, ...rest }) {
    return (
      <details
        {...rest}
        className="my-3 border border-outline-variant/40 px-3 py-2"
      >
        {children}
      </details>
    );
  },

  summary({ node: _node, children, ...rest }) {
    return (
      <summary
        className="cursor-crosshair select-none font-medium text-on-surface"
        {...rest}
      >
        {children}
      </summary>
    );
  },

  /** Images are blocked at the schema level; this override is defence-in-depth. */
  img() {
    return null;
  },
};
