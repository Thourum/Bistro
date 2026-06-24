import React from 'react';

/**
 * BistroWordmark — the typographic logotype: "BISTRO" set in Geist 800,
 * uppercase, tight negative tracking. This is the established wordmark; do not
 * substitute a monospace face. Renders as plain text so it inherits crisp
 * font rendering at any size.
 */
export function BistroWordmark({ size = 32, color = 'var(--color-on-surface)', as = 'span', style, ...props }) {
  const Tag = as;
  return (
    <Tag
      style={{
        fontFamily: 'var(--font-sans)',
        fontWeight: 800,
        fontSize: size,
        lineHeight: 1,
        letterSpacing: '-0.04em',
        textTransform: 'uppercase',
        color,
        display: 'inline-block',
        ...style,
      }}
      {...props}
    >
      Bistro
    </Tag>
  );
}
