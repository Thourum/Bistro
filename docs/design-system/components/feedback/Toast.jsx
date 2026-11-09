import React from 'react';

/**
 * Toast — terse action feedback. A flat bar: status square, mono message,
 * optional key hint. Bistro never celebrates; it confirms. ("PR approved.")
 */
export function Toast({ message, tone = 'success', keyHint, style, ...props }) {
  const dot = {
    success: 'var(--color-success)',
    error: 'var(--color-danger)',
    info: 'var(--color-primary)',
    warning: 'var(--color-warning)',
  }[tone] ?? 'var(--color-success)';
  return (
    <div
      role="status"
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 12,
        padding: '10px 14px',
        background: 'var(--color-inverse-surface)',
        color: 'var(--color-inverse-on-surface)',
        boxShadow: 'var(--shadow-ambient)',
        ...style,
      }}
      {...props}
    >
      <span style={{ width: 8, height: 8, background: dot, flexShrink: 0 }} aria-hidden="true" />
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, letterSpacing: '0.04em' }}>{message}</span>
      {keyHint && (
        <kbd style={{ fontFamily: 'var(--font-mono)', fontSize: 10, padding: '2px 6px', border: '1px solid rgba(255,255,255,0.25)', color: 'inherit', opacity: 0.8 }}>{keyHint}</kbd>
      )}
    </div>
  );
}
