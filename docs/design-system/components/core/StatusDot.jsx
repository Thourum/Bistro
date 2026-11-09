import React from 'react';

const STATUS = {
  success: { color: 'var(--color-success)', label: 'CI SUCCESS' },
  failure: { color: 'var(--color-danger)', label: 'CI FAILED' },
  running: { color: 'var(--color-warning)', label: 'CI RUNNING' },
  pending: { color: 'var(--color-warning)', label: 'CI PENDING' },
  unknown: { color: 'var(--color-neutral-dot)', label: 'CI UNKNOWN' },
};

/**
 * StatusDot — a hard square status signal. Never a circle.
 *
 * Bistro encodes CI / sync / review state as flat colored squares. `pulse`
 * adds the running/connecting heartbeat.
 */
export function StatusDot({ status = 'unknown', size = 12, pulse = false, title, style, ...props }) {
  const s = STATUS[status] ?? STATUS.unknown;
  const animate = pulse || status === 'running' || status === 'pending';
  return (
    <span
      role="img"
      aria-label={title ?? s.label}
      title={title ?? s.label}
      style={{
        display: 'inline-block',
        width: size,
        height: size,
        background: s.color,
        flexShrink: 0,
        animation: animate ? 'bistro-pulse 1.4s ease-in-out infinite' : undefined,
        ...style,
      }}
      {...props}
    >
      <style>{'@keyframes bistro-pulse{0%,100%{opacity:1}50%{opacity:0.35}}'}</style>
    </span>
  );
}
