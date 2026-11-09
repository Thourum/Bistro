import React from 'react';

/**
 * Toolbar — a horizontal command strip with a hairline underline. Holds the
 * breadcrumb / title on the left and actions on the right. The diff-view and
 * inbox top bars are Toolbars.
 */
export function Toolbar({ children, leading, trailing, style, ...props }) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 16,
        minHeight: 56,
        padding: '0 20px',
        borderBottom: '1px solid var(--color-outline-variant)',
        background: 'var(--color-surface-container-lowest)',
        ...style,
      }}
      {...props}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, minWidth: 0 }}>
        {leading}
        {children}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexShrink: 0 }}>
        {trailing}
      </div>
    </div>
  );
}
