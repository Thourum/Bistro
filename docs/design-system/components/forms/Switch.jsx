import React from 'react';

/**
 * Switch — square track + square thumb. No pills in Bistro. Settings toggles.
 */
export function Switch({ checked = false, onChange, label, disabled = false, id, style, ...props }) {
  const reactId = React.useId();
  const cid = id ?? reactId;
  return (
    <label
      htmlFor={cid}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 10,
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
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={onChange}
        style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }}
        {...props}
      />
      <span
        style={{
          position: 'relative',
          width: 36,
          height: 18,
          flexShrink: 0,
          background: checked ? 'var(--color-primary)' : 'var(--color-surface-container-highest)',
          border: `1px solid ${checked ? 'var(--color-primary)' : 'var(--color-outline-variant)'}`,
          transition: 'background-color 140ms',
        }}
      >
        <span
          style={{
            position: 'absolute',
            top: 1,
            left: checked ? 19 : 1,
            width: 14,
            height: 14,
            background: checked ? 'var(--color-on-primary)' : 'var(--color-outline)',
            transition: 'left 140ms cubic-bezier(0.4,0,0.2,1)',
          }}
        />
      </span>
      {label && <span>{label}</span>}
    </label>
  );
}
