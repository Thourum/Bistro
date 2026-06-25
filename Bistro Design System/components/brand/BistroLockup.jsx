import React from 'react';
import { BistroMark } from './BistroMark.jsx';
import { BistroWordmark } from './BistroWordmark.jsx';

/**
 * BistroLockup — the mark + wordmark, locked together. `horizontal` for bars
 * and headers, `stacked` for splash / centered contexts. Mark and wordmark
 * scale proportionally off `size` (the wordmark cap height).
 */
export function BistroLockup({ size = 32, orientation = 'horizontal', variant = 'default', color, style, ...props }) {
  const markSize = orientation === 'stacked' ? size * 2.4 : size * 1.7;
  const wordColor = color ?? (variant === 'inverse' ? 'var(--color-surface-container-lowest)' : 'var(--color-on-surface)');
  return (
    <span
      style={{
        display: 'inline-flex',
        flexDirection: orientation === 'stacked' ? 'column' : 'row',
        alignItems: 'center',
        gap: orientation === 'stacked' ? size * 0.5 : size * 0.5,
        ...style,
      }}
      {...props}
    >
      <BistroMark size={markSize} variant={variant} />
      <BistroWordmark size={size} color={wordColor} />
    </span>
  );
}
