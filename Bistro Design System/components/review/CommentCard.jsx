import React from 'react';

/**
 * CommentCard — an inline review comment. Square avatar, mono author handle,
 * a status tag (e.g. ACTIVE_DRAFT), body, and optional action row. Matches the
 * diff-view comment composer / thread entry.
 */
export function CommentCard({ author, tag, time, children, actions, style, ...props }) {
  return (
    <div
      style={{
        background: 'var(--color-surface-container-lowest)',
        border: '1px solid var(--color-outline-variant)',
        ...style,
      }}
      {...props}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '12px 16px',
          borderBottom: '1px solid var(--color-outline-variant)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
          <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 28, height: 28, background: 'var(--color-primary)', color: 'var(--color-on-primary)', fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, flexShrink: 0 }}>
            {(author ?? '?').split(/[\s_/-]+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join('')}
          </span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--color-on-surface)', textTransform: 'uppercase' }}>
            {author}
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          {time && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--color-outline-variant)' }}>{time}</span>}
          {tag && (
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: '0.12em', textTransform: 'uppercase', color: 'var(--color-outline)' }}>
              {tag}
            </span>
          )}
        </div>
      </div>
      <div style={{ padding: 16, fontFamily: 'var(--font-sans)', fontSize: 14, lineHeight: 1.5, color: children ? 'var(--color-on-surface)' : 'var(--color-outline)' }}>
        {children ?? 'Type_comment_here…'}
      </div>
      {actions && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 8, padding: '12px 16px', borderTop: '1px solid var(--color-outline-variant)' }}>
          {actions}
        </div>
      )}
    </div>
  );
}
