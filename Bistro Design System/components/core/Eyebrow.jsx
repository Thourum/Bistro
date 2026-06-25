import React from 'react';

/**
 * Eyebrow — the uppercase monospace label that captions everything in Bistro.
 *
 * Wide tracking, JetBrains Mono. `tone="primary"` is the blue section eyebrow
 * (e.g. "WORKSPACE / GLOBAL"); `tone="muted"` is the gray chrome label.
 */
export function Eyebrow({ children, tone = 'muted', as = 'span', style, ...props }) {
  const Tag = as;
  const colors = {
    muted: 'var(--color-outline)',
    primary: 'var(--color-primary)',
    strong: 'var(--color-on-surface)',
  };
  return (
    <Tag
      style={{
        fontFamily: 'var(--font-mono)',
        fontSize: 12,
        fontWeight: 400,
        letterSpacing: '0.15em',
        textTransform: 'uppercase',
        lineHeight: 1.33,
        color: colors[tone] ?? colors.muted,
        ...style,
      }}
      {...props}
    >
      {children}
    </Tag>
  );
}
