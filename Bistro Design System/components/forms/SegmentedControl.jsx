import React from 'react';

/**
 * SegmentedControl — the boxed filter row from the inbox ("Open / Draft / All /
 * Needs Action"). Each option carries an optional count. Active = blue fill.
 */
export function SegmentedControl({ options = [], value, onChange, style, ...props }) {
  return (
    <div
      role="tablist"
      style={{
        display: 'inline-flex',
        border: '1px solid var(--color-outline-variant)',
        ...style,
      }}
      {...props}
    >
      {options.map((opt, i) => {
        const val = typeof opt === 'string' ? opt : opt.value;
        const labelTxt = typeof opt === 'string' ? opt : opt.label;
        const count = typeof opt === 'string' ? undefined : opt.count;
        const active = val === value;
        return (
          <button
            key={val}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange?.(val)}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              padding: '6px 12px',
              fontFamily: 'var(--font-mono)',
              fontSize: 11,
              letterSpacing: '0.08em',
              textTransform: 'uppercase',
              whiteSpace: 'nowrap',
              borderLeft: i === 0 ? 'none' : '1px solid var(--color-outline-variant)',
              background: active ? 'var(--color-primary)' : 'transparent',
              color: active ? 'var(--color-on-primary)' : 'var(--color-on-surface-variant)',
              transition: 'background-color 120ms, color 120ms',
            }}
            onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = 'var(--color-surface-container)'; }}
            onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = 'transparent'; }}
          >
            {labelTxt}
            {count !== undefined && (
              <span style={{ opacity: 0.7, fontSize: 10 }}>{count}</span>
            )}
          </button>
        );
      })}
    </div>
  );
}
