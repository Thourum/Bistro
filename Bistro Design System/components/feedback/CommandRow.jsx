import React from 'react';

/**
 * CommandRow — one result in the command palette. Leading Material icon,
 * title + mono subtitle, trailing shortcut keys. Selection = left primary
 * accent + highest container fill (cmdk data-selected parity).
 */
export function CommandRow({ icon, title, subtitle, shortcut = [], selected = false, onClick, style, ...props }) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        width: '100%',
        padding: '12px',
        textAlign: 'left',
        cursor: 'pointer',
        borderLeft: `2px solid ${selected ? 'var(--color-primary)' : 'transparent'}`,
        background: selected ? 'var(--color-surface-container-highest)' : 'transparent',
        transition: 'background-color 120ms, border-color 120ms',
        ...style,
      }}
      onMouseEnter={(e) => { if (!selected) e.currentTarget.style.background = 'var(--color-surface-container-highest)'; }}
      onMouseLeave={(e) => { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      {...props}
    >
      <span style={{ display: 'flex', alignItems: 'center', gap: 12, minWidth: 0 }}>
        {icon && (
          <span className="material-symbols-outlined" style={{ fontSize: 20, color: selected ? 'var(--color-primary)' : 'var(--color-on-surface-variant)', flexShrink: 0 }} aria-hidden="true">
            {icon}
          </span>
        )}
        <span style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 14, fontWeight: 500, color: 'var(--color-on-surface)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{title}</span>
          {subtitle && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--color-outline)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{subtitle}</span>}
        </span>
      </span>
      {shortcut.length > 0 && (
        <span style={{ display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0, marginLeft: 16, opacity: selected ? 1 : 0.5 }}>
          {shortcut.map((k) => (
            <kbd key={k} style={{ fontFamily: 'var(--font-mono)', fontSize: 10, padding: '2px 6px', background: 'var(--color-surface)', border: '1px solid var(--color-outline-variant)', color: 'var(--color-on-surface-variant)' }}>{k}</kbd>
          ))}
        </span>
      )}
    </button>
  );
}
