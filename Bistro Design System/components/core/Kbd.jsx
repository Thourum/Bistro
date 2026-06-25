import React from 'react';

/**
 * Kbd — a single bound key, rendered as a boxed cap.
 *
 * The atomic unit of Bistro's keyboard-first chrome. Appears in legends,
 * command rows, help overlays, and inline in prose ("press [?]").
 */
export function Kbd({ children, tone = 'default', style, ...props }) {
  const tones = {
    default: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-on-surface)',
      border: '1px solid var(--color-outline-variant)',
    },
    muted: {
      background: 'var(--color-surface-container)',
      color: 'var(--color-on-surface-variant)',
      border: '1px solid var(--color-outline-variant)',
    },
    primary: {
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      border: '1px solid var(--color-primary)',
    },
  };
  return (
    <kbd
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        minWidth: 18,
        padding: '2px 6px',
        fontFamily: 'var(--font-mono)',
        fontSize: 10,
        lineHeight: 1.2,
        letterSpacing: '0.02em',
        ...(tones[tone] ?? tones.default),
        ...style,
      }}
      {...props}
    >
      {children}
    </kbd>
  );
}
