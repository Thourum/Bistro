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

function MarkdownImpl({
  body,
  compact,
  className,
}: MarkdownProps): React.JSX.Element {
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
