import React from 'react';

interface MainLayoutProps {
  children: React.ReactNode;
  toolbar?: React.ReactNode;
  sidebar?: React.ReactNode;
  detail?: React.ReactNode;
}

export function MainLayout({
  children,
  toolbar,
  sidebar,
  detail,
}: MainLayoutProps) {
  return (
    <div
      data-testid="main-layout"
      className="w-full h-screen bg-surface-container-lowest flex flex-col"
    >
      {toolbar && (
        <div className="border-b border-outline-variant bg-surface-container-lowest px-4 py-3 shadow-ambient">
          <div className="flex items-center justify-between">{toolbar}</div>
        </div>
      )}

      <div className="flex flex-1 overflow-hidden">
        {sidebar && (
          <div className="w-64 border-r border-outline-variant bg-surface-container-low overflow-y-auto">
            {sidebar}
          </div>
        )}

        <div className="flex-1 overflow-y-auto bg-surface-container-lowest p-4">
          {children}
        </div>

        {detail && (
          <div className="w-80 border-l border-outline-variant bg-surface-container-low overflow-y-auto p-4 hidden lg:block">
            {detail}
          </div>
        )}
      </div>
    </div>
  );
}
