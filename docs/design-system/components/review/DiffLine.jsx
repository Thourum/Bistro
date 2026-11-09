import React from 'react';

/**
 * DiffLine — one row of a unified diff. Old/new line-number gutters, a +/-/·
 * marker, and monospace code. `type` tints the row: add (green), remove (red),
 * context (plain), focus (primary-outlined active line).
 */
export function DiffLine({ oldNo, newNo, type = 'context', children, focused = false, style, ...props }) {
  const bg = {
    add: 'var(--color-diff-add-bg)',
    remove: 'var(--color-diff-remove-bg)',
    context: 'transparent',
  }[type] ?? 'transparent';
  const marker = { add: '+', remove: '-', context: '' }[type] ?? '';
  const codeColor = {
    add: 'var(--color-on-surface)',
    remove: 'var(--color-on-surface)',
    context: 'var(--color-on-surface)',
  }[type];
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '52px 52px 1fr',
        alignItems: 'stretch',
        background: bg,
        boxShadow: focused ? 'inset 0 0 0 1px var(--color-primary)' : 'none',
        fontFamily: 'var(--font-mono)',
        fontSize: 13,
        lineHeight: '24px',
        ...style,
      }}
      {...props}
    >
      <span style={{ textAlign: 'center', color: 'var(--color-outline-variant)', userSelect: 'none', background: type === 'remove' ? 'var(--color-diff-remove-gutter)' : type === 'add' ? 'transparent' : 'transparent' }}>
        {oldNo ?? (type === 'add' ? '' : '')}
      </span>
      <span style={{ textAlign: 'center', color: 'var(--color-outline-variant)', userSelect: 'none', background: type === 'add' ? 'var(--color-diff-add-gutter)' : 'transparent' }}>
        {newNo ?? (type === 'remove' ? '' : '')}
      </span>
      <code style={{ paddingLeft: 12, paddingRight: 12, color: codeColor, whiteSpace: 'pre', overflow: 'hidden' }}>
        <span style={{ display: 'inline-block', width: 14, color: type === 'add' ? 'var(--color-diff-add-text)' : type === 'remove' ? 'var(--color-diff-remove-text)' : 'transparent' }}>{marker}</span>
        {children}
      </code>
    </div>
  );
}
