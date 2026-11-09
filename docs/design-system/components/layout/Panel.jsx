import React from 'react';

/**
 * Panel — the white container plane. A flat surface with a 1px hairline border;
 * no shadow, no radius. Optional `title` (mono eyebrow) header bar.
 */
export function Panel({ children, title, action, padded = true, elevated = false, style, ...props }) {
  return (
    <div
      style={{
        background: 'var(--color-surface-container-lowest)',
        border: '1px solid var(--color-outline-variant)',
        boxShadow: elevated ? 'var(--shadow-ambient)' : 'none',
        ...style,
      }}
      {...props}
    >
      {title && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '10px 16px',
            borderBottom: '1px solid var(--color-outline-variant)',
          }}
        >
          <span
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: 12,
              letterSpacing: '0.15em',
              textTransform: 'uppercase',
              color: 'var(--color-outline)',
            }}
          >
            {title}
          </span>
          {action}
        </div>
      )}
      <div style={{ padding: padded ? 16 : 0 }}>{children}</div>
    </div>
  );
}
