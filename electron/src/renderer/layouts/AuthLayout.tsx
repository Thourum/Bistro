import type React from 'react';

interface AuthLayoutProps {
  children: React.ReactNode;
}

/**
 * Fullscreen centered layout with ambient crosshair decoration.
 * Matches `bistro_login/code.html` from the Stitch drafts.
 */
export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div
      data-testid="auth-layout"
      className="relative w-full flex-1 flex items-center justify-center px-6 bg-surface text-on-surface overflow-hidden"
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
        className="w-full max-w-md bg-surface-container-lowest border border-outline-variant/40 shadow-ambient"
      >
        {children}
      </div>
    </div>
  );
}
