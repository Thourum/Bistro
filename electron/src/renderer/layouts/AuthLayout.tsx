import type React from 'react';

interface AuthLayoutProps {
  children: React.ReactNode;
}

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div
      data-testid="auth-layout"
      className="w-full h-screen bg-surface flex items-center justify-center p-4"
    >
      <div
        data-testid="auth-card"
        className="bg-surface-container-lowest border border-outline-variant shadow-ambient max-w-md w-full"
      >
        {children}
      </div>
    </div>
  );
}
