import React from 'react';

/**
 * LineFocusChip — the small dark inverse chip from the diff view that reports
 * cursor position ("LINE_FOCUS: 44" + path). Also serves as Bistro's tooltip:
 * dark inverse surface, mono, square.
 */
export function LineFocusChip({ label, sub, style, ...props }) {
  return (
    <span
      style={{
        display: 'inline-flex',
        flexDirection: 'column',
        alignItems: 'flex-start',
        ...style,
      }}
      {...props}
    >
      <span
        style={{
          padding: '4px 10px',
          background: 'var(--color-inverse-surface)',
          color: 'var(--color-inverse-on-surface)',
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
        }}
      >
        {label}
      </span>
      {sub && (
        <span
          style={{
            padding: '3px 10px',
            background: 'var(--color-surface-container-high)',
            color: 'var(--color-outline)',
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            letterSpacing: '0.1em',
            textTransform: 'uppercase',
            borderTop: '1px solid var(--color-outline-variant)',
          }}
        >
          {sub}
        </span>
      )}
    </span>
  );
}
