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
        <pre className="overflow-x-auto rounded bg-surface-container-low p-2">
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
        className={`rounded bg-surface-container px-1 py-0.5 font-mono text-xs ${className ?? ''}`}
      >
        {children}
      </code>
    );
  },

  /** All links route through ExternalLink (HTTPS-only, always preventDefault). */
  a({ href, children }) {
    return (
      <ExternalLink href={typeof href === 'string' ? href : undefined}>
        {children}
      </ExternalLink>
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
        className="my-2 rounded border border-outline/30 px-2 py-1"
      >
        {children}
      </details>
    );
  },

  summary({ node: _node, children, ...rest }) {
    return (
      <summary {...rest} className="cursor-pointer select-none font-medium">
        {children}
      </summary>
    );
  },

  /** Images are blocked at the schema level; this override is defence-in-depth. */
  img() {
    return null;
  },
};
