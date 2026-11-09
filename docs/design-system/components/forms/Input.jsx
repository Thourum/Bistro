import React from 'react';

/**
 * Input — square text field. Flat fill, no border at rest; a 1px primary
 * underline-free ring on focus. Optional leading Material Symbol.
 */
export function Input({ icon, invalid = false, style, ...props }) {
  const [focused, setFocused] = React.useState(false);
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        height: 36,
        padding: '0 12px',
        background: 'var(--color-surface-container-low)',
        border: `1px solid ${invalid ? 'var(--color-error)' : focused ? 'var(--color-primary)' : 'transparent'}`,
        boxShadow: focused ? 'inset 0 0 0 1px var(--color-primary)' : 'none',
        transition: 'border-color 120ms, box-shadow 120ms',
      }}
    >
      {icon && (
        <span className="material-symbols-outlined" style={{ fontSize: 18, color: 'var(--color-outline)' }} aria-hidden="true">
          {icon}
        </span>
      )}
      <input
        onFocus={(e) => { setFocused(true); props.onFocus?.(e); }}
        onBlur={(e) => { setFocused(false); props.onBlur?.(e); }}
        style={{
          flex: 1,
          minWidth: 0,
          height: '100%',
          border: 'none',
          outline: 'none',
          background: 'transparent',
          color: 'var(--color-on-surface)',
          fontFamily: 'var(--font-sans)',
          fontSize: 14,
          ...style,
        }}
        {...props}
      />
    </div>
  );
}
