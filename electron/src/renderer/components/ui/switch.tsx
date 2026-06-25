import { cn } from '@renderer/lib/utils';
import * as React from 'react';

export interface SwitchProps {
  /** Controlled on/off state. */
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
  'aria-label'?: string;
}

/**
 * Bistro-branded Switch primitive.
 *
 * Square track + square thumb (zero radius), color-only state change, 140ms
 * thumb slide. Primary fill when on, neutral when off.
 *
 * Dark-mode fix preserved verbatim: a near-white inset ring
 * (`dark:ring-white/80`) keeps the track edge visible against near-black
 * surfaces. Removing it makes the off-state track disappear in dark mode.
 */
const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  (
    { checked, onCheckedChange, disabled, className, 'aria-label': ariaLabel },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={ariaLabel}
        disabled={disabled}
        onClick={() => onCheckedChange(!checked)}
        className={cn(
          'relative flex h-6 w-12 items-center p-1 ring-1 ring-inset ring-transparent transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-50 dark:ring-white/80',
          checked ? 'bg-primary' : 'bg-outline-variant',
          className,
        )}
      >
        <span
          className={cn(
            'h-4 w-4 bg-white transition-transform duration-150',
            checked ? 'ml-auto' : '',
          )}
        />
      </button>
    );
  },
);
Switch.displayName = 'Switch';

export { Switch };
