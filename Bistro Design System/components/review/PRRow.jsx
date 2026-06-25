import React from 'react';

function relTime(label) {
  return label;
}

/**
 * PRRow — one pull request in the inbox. 5-region grid: CI square · #number ·
 * title · @author · relative time. Selection paints a left primary accent +
 * tinted fill. Crosshair cursor — the technical signature.
 */
export function PRRow({
  number,
  title,
  author,
  time,
  ci = 'unknown',
  selected = false,
  onClick,
  style,
  ...props
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'grid',
        gridTemplateColumns: '24px 60px 1fr 120px 100px',
        alignItems: 'center',
        gap: 16,
        width: '100%',
        padding: '12px 16px',
        textAlign: 'left',
        cursor: 'crosshair',
        background: selected ? 'var(--color-surface-container-low)' : 'transparent',
        borderLeft: `2px solid ${selected ? 'var(--color-primary)' : 'transparent'}`,
        transition: 'background-color 120ms, border-color 120ms',
        ...style,
      }}
      onMouseEnter={(e) => {
        if (selected) return;
        e.currentTarget.style.background = 'var(--color-surface-container-low)';
        e.currentTarget.style.borderLeftColor = 'var(--color-primary)';
      }}
      onMouseLeave={(e) => {
        if (selected) return;
        e.currentTarget.style.background = 'transparent';
        e.currentTarget.style.borderLeftColor = 'transparent';
      }}
      {...props}
    >
      <span
        style={{
          width: 12,
          height: 12,
          background: {
            success: 'var(--color-success)',
            failure: 'var(--color-danger)',
            running: 'var(--color-warning)',
            pending: 'var(--color-warning)',
            unknown: 'var(--color-neutral-dot)',
          }[ci] ?? 'var(--color-neutral-dot)',
        }}
        aria-hidden="true"
      />
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--color-outline)' }}>
        #{number}
      </span>
      <span
        style={{
          fontFamily: 'var(--font-sans)',
          fontSize: 14,
          fontWeight: 500,
          color: 'var(--color-on-surface)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {title}
      </span>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--color-outline-variant)', textAlign: 'right', overflow: 'hidden', textOverflow: 'ellipsis' }}>
        @{author}
      </span>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, textAlign: 'right', color: selected ? 'var(--color-primary)' : 'var(--color-outline-variant)' }}>
        {relTime(time)}
      </span>
    </button>
  );
}
