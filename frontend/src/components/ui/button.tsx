import { Slot } from '@radix-ui/react-slot';
import { cn } from '@renderer/lib/utils';
import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';

/**
 * Bistro-branded Button primitive (shadcn base, restyled).
 *
 * Design language: zero radius, mono uppercase chrome, 1px hairlines, the one
 * functional Bistro blue, ~120ms color-only transitions (no shrink/bounce).
 * Focus is a 2px primary ring, offset, never removed.
 *
 * Variants:
 *  - `default`  — filled primary CTA; darkens to `on-primary-fixed-variant`.
 *  - `destructive` — hairline danger that inverts to a red fill on hover.
 *  - `outline`  — hairline neutral; hover steps one surface rung up.
 *  - `secondary`— tinted surface fill.
 *  - `ghost`    — text-only chrome control (Reply/Cancel/Save rows).
 *  - `link`     — primary underline.
 */
const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-none border font-mono uppercase tracking-widest transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:pointer-events-none disabled:opacity-50 [&_.material-symbols-outlined]:text-[1.1em]',
  {
    variants: {
      variant: {
        default:
          'border-primary bg-primary text-on-primary hover:bg-on-primary-fixed-variant hover:border-on-primary-fixed-variant',
        destructive:
          'border-error/60 text-error hover:bg-error hover:text-on-error hover:border-error',
        outline:
          'border-outline-variant bg-transparent text-on-surface hover:bg-surface-container-high hover:border-outline',
        secondary:
          'border-transparent bg-surface-container text-on-surface hover:bg-surface-container-high',
        ghost:
          'border-transparent bg-transparent text-outline hover:text-on-surface',
        link: 'border-transparent text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-9 px-4 text-[11px]',
        sm: 'h-7 px-3 text-[10px]',
        lg: 'h-11 px-8 text-xs',
        icon: 'h-9 w-9',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, type, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp
        // Default to type="button" so chrome controls never submit a form.
        type={asChild ? undefined : (type ?? 'button')}
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Button.displayName = 'Button';

export { Button };
