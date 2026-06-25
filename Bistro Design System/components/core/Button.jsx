import React from 'react';

/**
 * Bistro Button — square, technical, keyboard-forward.
 *
 * Zero radius, 1px borders, instant color-only transitions. The default
 * `primary` is Bistro blue; `outline` is the workhorse for secondary actions;
 * `ghost` for tertiary; `danger` for destructive. An optional `keyHint`
 * renders the bound shortcut as a boxed key on the trailing edge.
 */
export function Button({
  children,
  variant = 'primary',
  size = 'md',
  keyHint,
  icon,
  disabled = false,
  type = 'button',
  style,
  ...props
}) {
  const sizes = {
    sm: { height: 28, padding: '0 10px', fontSize: 12 },
    md: { height: 36, padding: '0 16px', fontSize: 13 },
    lg: { height: 44, padding: '0 24px', fontSize: 14 },
  };

  const variants = {
    primary: {
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      border: '1px solid var(--color-primary)',
    },
    outline: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-on-surface)',
      border: '1px solid var(--color-outline-variant)',
    },
    ghost: {
      background: 'transparent',
      color: 'var(--color-on-surface-variant)',
      border: '1px solid transparent',
    },
    danger: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-error)',
      border: '1px solid var(--color-error)',
    },
  };

  const sz = sizes[size] ?? sizes.md;
  const vr = variants[variant] ?? variants.primary;

  return (
    <button
      type={type}
      disabled={disabled}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        height: sz.height,
        padding: sz.padding,
        fontFamily: 'var(--font-mono)',
        fontSize: sz.fontSize,
        fontWeight: 500,
        letterSpacing: '0.05em',
        textTransform: 'uppercase',
        whiteSpace: 'nowrap',
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.45 : 1,
        transition: 'background-color 120ms, color 120ms, border-color 120ms',
        ...vr,
        ...style,
      }}
      onMouseEnter={(e) => {
        if (disabled) return;
        if (variant === 'primary') e.currentTarget.style.background = '#003ea8';
        else if (variant === 'outline' || variant === 'ghost')
          e.currentTarget.style.background = 'var(--color-surface-container)';
        else if (variant === 'danger') {
          e.currentTarget.style.background = 'var(--color-error)';
          e.currentTarget.style.color = 'var(--color-on-error)';
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = vr.background;
        e.currentTarget.style.color = vr.color;
      }}
      {...props}
    >
      {icon && (
        <span
          className="material-symbols-outlined"
          style={{ fontSize: sz.fontSize + 4 }}
          aria-hidden="true"
        >
          {icon}
        </span>
      )}
      {children}
      {keyHint && (
        <kbd
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: sz.fontSize - 2,
            padding: '1px 5px',
            marginLeft: 2,
            border:
              variant === 'primary'
                ? '1px solid rgba(255,255,255,0.4)'
                : '1px solid var(--color-outline-variant)',
            color: 'inherit',
            opacity: 0.85,
            lineHeight: 1.4,
          }}
        >
          {keyHint}
        </kbd>
      )}
    </button>
  );
}
