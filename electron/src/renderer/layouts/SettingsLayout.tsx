import type React from 'react';

interface SettingsLayoutProps {
  title: string;
  description: string;
  children: React.ReactNode;
}

export function SettingsLayout({
  title,
  description,
  children,
}: SettingsLayoutProps) {
  return (
    <div className="w-full h-screen bg-surface-container-lowest flex flex-col">
      <div
        data-testid="settings-header"
        className="border-b border-outline-variant bg-surface-container-lowest px-6 py-6 shadow-ambient"
      >
        <h1 className="text-3xl font-extrabold text-on-surface uppercase tracking-tighter mb-2">
          {title}
        </h1>
        <p className="text-sm text-on-surface-variant font-medium tracking-tight">
          {description}
        </p>
      </div>

      <div className="flex-1 overflow-y-auto p-6 max-w-4xl mx-auto w-full">
        {children}
      </div>
    </div>
  );
}
