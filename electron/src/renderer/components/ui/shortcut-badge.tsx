import { cn } from "@renderer/lib/utils";
import { formatForDisplay } from "@tanstack/react-hotkeys";
import type React from "react";

export interface ShortcutBadgeProps {
  hotkey: string;
  className?: string;
}

/**
 * Compact shortcut hint for keyboard-forward buttons.
 *
 * Mirrors the Bistro design-system Button `keyHint` treatment: a trailing,
 * boxed key label rendered inside the button. Formatting is delegated to
 * TanStack Hotkeys so `Mod+Enter` becomes platform-appropriate.
 */
export function ShortcutBadge({
  hotkey,
  className,
}: ShortcutBadgeProps): React.ReactElement {
  return (
    <kbd
      className={cn(
        "ml-0.5 inline-flex items-center justify-center rounded-none border border-current/35 px-1.5 py-0.5 font-mono text-[9px] leading-none text-current",
        className,
      )}
    >
      {formatForDisplay(hotkey)}
    </kbd>
  );
}
