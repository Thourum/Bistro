import type React from 'react';

const dragRegionStyle = {
  WebkitAppRegion: 'drag',
} as React.CSSProperties;

/**
 * Integrated Electron title bar.
 *
 * The native frame is hidden in `main/window.ts`; this chrome reserves the
 * draggable region inside the app so the window feels like part of Bistro's
 * system shell instead of a separate OS title bar.
 */
export function AppTitleBar(): React.ReactElement {
  return (
    <header
      className="h-8 shrink-0 border-b border-outline-variant/40 bg-surface-container-lowest/95 text-[10px] font-mono uppercase tracking-widest text-outline backdrop-blur"
      style={dragRegionStyle}
    >
      <div className="flex h-full items-center justify-between pl-24 pr-36">
        <div className="flex min-w-0 items-center gap-3">
          <span className="font-bold text-on-surface">BISTRO</span>
          <span className="text-outline-variant">{'//'}</span>
          <span className="truncate">LOCAL REVIEW CLIENT</span>
        </div>
        <div className="hidden items-center gap-2 sm:flex">
          <span className="h-2 w-2 bg-primary" aria-hidden="true" />
          <span>SYSTEM_STATUS: LOCAL</span>
        </div>
      </div>
    </header>
  );
}
