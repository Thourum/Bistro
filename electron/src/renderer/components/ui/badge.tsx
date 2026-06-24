import { cn } from '@renderer/lib/utils';
import { cva, type VariantProps } from 'class-variance-authority';
import type * as React from 'react';

/**
 * Bistro-branded Badge primitive (shadcn base, restyled).
 *
 * Zero radius, mono uppercase, 1px hairlines. Used for branch tags, file lang
 * markers, and thread lifecycle states. Status badges read as a hard square +
 * a word, so the badge itself is just the bordered chrome around the label.
 */
const badgeVariants = cva(
  'inline-flex items-center gap-1.5 rounded-none border px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-widest transition-colors duration-150',
  {
    variants: {
      variant: {
        default: 'border-primary/50 text-primary',
        neutral: 'border-outline-variant/50 text-outline',
        surface: 'border-transparent bg-surface-container text-on-surface',
        outline: 'border-outline-variant/50 text-on-surface',
        // Thread / CI lifecycle tones — open is the only "loud" state.
        open: 'border-yellow-600/50 text-yellow-600',
        resolved: 'border-primary/50 text-primary',
        outdated: 'border-outline-variant/40 text-outline',
        unknown: 'border-outline-variant/40 text-outline-variant',
        danger: 'border-error/50 text-error',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <span className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
