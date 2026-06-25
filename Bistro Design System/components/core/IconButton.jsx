import React from 'react';

/**
 * IconButton — a square, icon-only control built on Material Symbols.
 * Used for toolbar actions, close buttons, row affordances.
 */
export function IconButton({ icon, label, size = 'md', variant = 'ghost', disabled = false, style, ...props }) {
  const sizes = { sm: 28, md: 36, lg: 44 };
  const dim = sizes[size] ?? sizes.md;
  const variants = {
    ghost: { background: 'transparent', color: 'var(--color-on-surface-variant)', border: '1px solid transparent' },
    outline: { background: 'var(--color-surface-container-lowest)', color: 'var(--color-on-surface)', border: '1px solid var(--color-outline-variant)' },
    primary: { background: 'var(--color-primary)', color: 'var(--color-on-primary)', border: '1px solid var(--color-primary)' },
  };
  const vr = variants[variant] ?? variants.ghost;
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: dim,
        height: dim,
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.45 : 1,
        transition: 'background-color 120ms, color 120ms',
        ...vr,
        ...style,
      }}
      onMouseEnter={(e) => {
        if (disabled || variant === 'primary') return;
        e.currentTarget.style.background = 'var(--color-surface-container)';
        e.currentTarget.style.color = 'var(--color-on-surface)';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = vr.background;
        e.currentTarget.style.color = vr.color;
      }}
      {...props}
    >
      <span className="material-symbols-outlined" style={{ fontSize: Math.round(dim * 0.55) }} aria-hidden="true">
        {icon}
      </span>
    </button>
  );
}
