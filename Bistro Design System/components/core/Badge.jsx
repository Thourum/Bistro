import React from 'react';

/**
 * Badge — a small bordered label. Two register: `outline` (the boxed mono tag,
 * e.g. "GROUP", "RUST", "ACTIVE_DRAFT") and `solid` (filled accent).
 *
 * Status badges (`tone`) tint border + text for review/CI semantics.
 */
export function Badge({ children, variant = 'outline', tone = 'neutral', style, ...props }) {
  const tones = {
    neutral: { fg: 'var(--color-on-surface-variant)', bd: 'var(--color-outline-variant)', bg: 'var(--color-surface-container-low)' },
    primary: { fg: 'var(--color-primary)', bd: 'var(--color-primary)', bg: 'var(--color-primary-fixed)' },
    success: { fg: 'var(--color-success-strong)', bd: 'var(--color-success)', bg: 'var(--color-diff-add-bg)' },
    danger: { fg: 'var(--color-error)', bd: 'var(--color-error)', bg: 'var(--color-error-container)' },
    warning: { fg: 'var(--color-tertiary)', bd: 'var(--color-tertiary-container)', bg: 'var(--color-tertiary-fixed)' },
  };
  const t = tones[tone] ?? tones.neutral;
  const solid = variant === 'solid';
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        padding: '2px 6px',
        fontFamily: 'var(--font-mono)',
        fontSize: 10,
        fontWeight: 500,
        letterSpacing: '0.1em',
        textTransform: 'uppercase',
        lineHeight: 1.3,
        whiteSpace: 'nowrap',
        color: solid ? 'var(--color-on-primary)' : t.fg,
        background: solid ? (tone === 'neutral' ? 'var(--color-on-surface)' : t.bd) : 'transparent',
        border: `1px solid ${solid ? 'transparent' : t.bd}`,
        ...style,
      }}
      {...props}
    >
      {children}
    </span>
  );
}
