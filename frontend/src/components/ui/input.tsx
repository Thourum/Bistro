import * as React from 'react';
import { cn } from '../../lib/utils';

type InputProps = React.InputHTMLAttributes<HTMLInputElement>;

/**
 * Bistro-branded Input primitive (shadcn base, restyled).
 *
 * Zero radius, 1px hairline, mono text/placeholder, 2px primary focus border.
 */
const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          'flex h-9 w-full rounded-none border border-outline-variant/60 bg-surface px-3 py-1 font-mono text-sm text-on-surface transition-colors duration-150 file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-outline-variant focus-visible:border-primary focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        ref={ref}
        {...props}
      />
    );
  },
);
Input.displayName = 'Input';

export { Input };
