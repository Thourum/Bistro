import { cn } from '@renderer/lib/utils';
import type * as React from 'react';

export type KbdProps = React.HTMLAttributes<HTMLElement>;

/**
 * Bistro-branded Kbd primitive — a boxed keyboard key.
 *
 * Keys are nouns in Bistro's voice and are always boxed when written inline.
 * Zero radius, 1px hairline, JetBrains Mono. Renders a `<kbd>` so it keeps its
 * semantic meaning for assistive tech.
 */
function Kbd({ className, ...props }: KbdProps) {
  return (
    <kbd
      className={cn(
        'inline-flex min-w-[1.5rem] items-center justify-center rounded-none border border-outline-variant/50 bg-surface-container-high px-1.5 py-0.5 text-center font-mono text-[11px] leading-none text-on-surface-variant',
        className,
      )}
      {...props}
    />
  );
}

export { Kbd };
