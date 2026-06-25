import React from 'react';

/**
 * SystemBar — the fixed bottom chrome strip. Three regions: left legend
 * (key/desc pairs), center slot, right status. Mono, uppercase, 32px tall.
 * The "SYSTEM_STATUS: OPERATIONAL // V.01" footer.
 */
export function SystemBar({ legend = [], center, status, version, statusTone = 'success', style, ...props }) {
  const dot = {
    success: 'var(--color-success)',
    warning: 'var(--color-warning)',
    danger: 'var(--color-danger)',
    idle: 'var(--color-neutral-dot)',
  }[statusTone] ?? 'var(--color-success)';

  return (
    <footer
      style={{
        display: 'flex',
        alignItems: 'center',
        height: 32,
        flexShrink: 0,
        borderTop: '1px solid var(--color-outline-variant)',
        background: 'var(--color-surface-container-high)',
        fontFamily: 'var(--font-mono)',
        fontSize: 10,
        letterSpacing: '0.12em',
        textTransform: 'uppercase',
        ...style,
      }}
      {...props}
    >
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 16, padding: '0 16px', overflow: 'hidden' }}>
        {legend.map((item) => (
          <span key={item.key} style={{ display: 'inline-flex', alignItems: 'center', gap: 8, whiteSpace: 'nowrap' }}>
            <kbd style={{ background: 'var(--color-surface-container-lowest)', border: '1px solid var(--color-outline-variant)', padding: '2px 6px', fontSize: 9, color: 'var(--color-on-surface)' }}>{item.key}</kbd>
            <span style={{ color: 'var(--color-outline)' }}>{item.label}</span>
          </span>
        ))}
      </div>
      <div style={{ flex: 1, display: 'flex', justifyContent: 'center', alignItems: 'center', overflow: 'hidden' }}>
        {center}
      </div>
      <div style={{ flex: 1, display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 10, padding: '0 16px' }}>
        <span style={{ color: 'var(--color-primary)', fontWeight: 500 }}>
          {status ?? 'System_Status: Operational'}
        </span>
        {version && <span style={{ color: 'var(--color-primary)' }}>// {version}</span>}
        <span style={{ width: 8, height: 8, background: dot }} aria-hidden="true" />
      </div>
    </footer>
  );
}
