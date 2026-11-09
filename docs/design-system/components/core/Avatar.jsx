import React from 'react';

/**
 * Avatar — square initials chip. Bistro avatars are sharp squares, never round,
 * usually the Bistro-blue identity tile (e.g. "JD" in the diff comment box).
 */
export function Avatar({ name = '', size = 32, tone = 'primary', style, ...props }) {
  const initials = name
    .split(/[\s_/-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join('');
  const tones = {
    primary: { bg: 'var(--color-primary)', fg: 'var(--color-on-primary)' },
    neutral: { bg: 'var(--color-surface-container-highest)', fg: 'var(--color-on-surface-variant)' },
    inverse: { bg: 'var(--color-inverse-surface)', fg: 'var(--color-inverse-on-surface)' },
  };
  const t = tones[tone] ?? tones.primary;
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        background: t.bg,
        color: t.fg,
        fontFamily: 'var(--font-mono)',
        fontSize: Math.round(size * 0.34),
        fontWeight: 700,
        letterSpacing: '0.02em',
        flexShrink: 0,
        ...style,
      }}
      {...props}
    >
      {initials || '?'}
    </span>
  );
}
