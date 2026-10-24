import type React from 'react';
import { cn } from '../lib/utils';

interface AuthLayoutProps {
  children: React.ReactNode;
  /** Two-column card; scrolls from the top when taller than the window. */
  isWide?: boolean;
}

/**
 * Fullscreen centered layout with ambient crosshair decoration.
 * Matches `bistro_login/code.html` from the Stitch drafts.
 */
export function AuthLayout({ children, isWide = false }: AuthLayoutProps) {
  return (
    <div
      data-testid="auth-layout"
      className={cn(
        'relative w-full min-h-0 flex-1 flex justify-center px-6 bg-surface text-on-surface',
        isWide
          ? 'items-start overflow-y-auto py-6'
          : 'items-center overflow-hidden',
      )}
    >
      {/* Ambient crosshair background */}
      <div
        aria-hidden
        className="absolute inset-0 -z-10 pointer-events-none opacity-30"
      >
        <div className="absolute top-[10%] left-[5%] w-[40%] h-[1px] bg-gradient-to-r from-transparent via-outline-variant/40 to-transparent" />
        <div className="absolute top-0 right-[20%] w-[1px] h-full bg-gradient-to-b from-transparent via-outline-variant/40 to-transparent" />
      </div>

      <div
        data-testid="auth-card"
        className={cn(
          'w-full bg-surface-container-lowest border border-outline-variant/40 shadow-ambient',
          isWide ? 'max-w-4xl my-auto shrink-0' : 'max-w-md',
        )}
      >
        {children}
      </div>
    </div>
  );
}
