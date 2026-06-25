import * as CheckboxPrimitive from '@radix-ui/react-checkbox';
import { cn } from '@renderer/lib/utils';
import * as React from 'react';

/**
 * Bistro-branded Checkbox primitive (shadcn base, restyled).
 *
 * Zero radius, 1px hairline, primary fill when checked. The check glyph is a
 * Material Symbol (`check`) rather than a Lucide SVG — the brand's icon system
 * is Material Symbols Outlined.
 */
const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      'peer grid h-4 w-4 shrink-0 place-content-center rounded-none border border-outline-variant transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-on-primary',
      className,
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator
      className={cn('grid place-content-center text-current')}
    >
      <span className="material-symbols-outlined text-[14px] leading-none">
        check
      </span>
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
));
Checkbox.displayName = CheckboxPrimitive.Root.displayName;

export { Checkbox };
