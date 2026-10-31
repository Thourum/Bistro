import * as React from 'react';
import { cn } from '../../lib/utils';

type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement>;

/**
 * Bistro-branded Textarea primitive.
 *
 * Zero radius, 1px hairline, mono text, 2px primary focus border. Text wraps
 * naturally (soft wrap) — the browser default for `<textarea>` — so reply and
 * comment bodies never scroll horizontally. Vertical resize only.
 */
const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => {
    return (
      <textarea
        ref={ref}
        // `whitespace-pre-wrap break-words` guarantees soft wrapping even if a
        // consumer overrides `white-space`; no horizontal scrollbar.
        className={cn(
          'flex w-full resize-none whitespace-pre-wrap break-words rounded-none border border-outline-variant/60 bg-surface px-3 py-2 font-mono text-sm text-on-surface transition-colors duration-150 placeholder:text-outline-variant focus-visible:border-primary focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        {...props}
      />
    );
  },
);
Textarea.displayName = 'Textarea';

export { Textarea };
