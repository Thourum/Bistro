import React from 'react';

/**
 * Checkbox — square, of course. Checked state fills Bistro blue with a
 * Material check glyph. Includes an optional inline label.
 */
export function Checkbox({ checked = false, onChange, label, disabled = false, id, style, ...props }) {
  const reactId = React.useId();
  const cid = id ?? reactId;
  return (
    <label
      htmlFor={cid}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 8,
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.5 : 1,
        fontFamily: 'var(--font-sans)',
        fontSize: 14,
        color: 'var(--color-on-surface)',
        ...style,
      }}
    >
      <input
        id={cid}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={onChange}
        style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }}
        {...props}
      />
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          width: 18,
          height: 18,
          flexShrink: 0,
          background: checked ? 'var(--color-primary)' : 'var(--color-surface-container-lowest)',
          border: `1px solid ${checked ? 'var(--color-primary)' : 'var(--color-outline-variant)'}`,
          color: 'var(--color-on-primary)',
          transition: 'background-color 120ms, border-color 120ms',
        }}
      >
        {checked && (
          <span className="material-symbols-outlined" style={{ fontSize: 16, fontVariationSettings: '"wght" 600' }} aria-hidden="true">
            check
          </span>
        )}
      </span>
      {label && <span>{label}</span>}
    </label>
  );
}
