import React from 'react';
import type { UIState } from '../store/ui';
import { HoverCard, HoverCardContent, HoverCardTrigger } from './ui/hover-card';

interface SyncStatusHoverCardProps {
  statusLabel: string;
  syncStatus: UIState['syncStatus'];
  isOffline: boolean;
  lastSyncAt: string | null;
  autoSync: boolean;
  syncIntervalSeconds: number;
  children: React.ReactElement;
}

interface SyncStatusDetails {
  summary: string;
  nextRefresh: string;
  nextRefreshDetail: string;
  interval: string;
  lastSync: string;
}

export function SyncStatusHoverCard({
  statusLabel,
  syncStatus,
  isOffline,
  lastSyncAt,
  autoSync,
  syncIntervalSeconds,
  children,
}: SyncStatusHoverCardProps): React.ReactElement {
  const [nowMs, setNowMs] = React.useState(() => Date.now());

  React.useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const details = getSyncStatusDetails({
    statusLabel,
    syncStatus,
    isOffline,
    lastSyncAt,
    autoSync,
    syncIntervalSeconds,
    nowMs,
  });

  return (
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild aria-label={`Sync status: ${details.summary}`}>
        {children}
      </HoverCardTrigger>
      <HoverCardContent side="top" align="end" className="space-y-3">
        <div>
          <div className="text-[10px] uppercase tracking-widest text-outline">
            Sync status
          </div>
          <div className="mt-1 text-sm font-semibold text-on-surface">
            {statusLabel}
          </div>
        </div>

        <div className="grid gap-2 text-xs normal-case tracking-normal">
          <StatusRow label="Next refresh" value={details.nextRefresh} />
          <StatusRow label="Interval" value={details.interval} />
          <StatusRow label="Last sync" value={details.lastSync} />
        </div>

        <p className="text-[11px] normal-case leading-snug tracking-normal text-on-surface-variant">
          {details.nextRefreshDetail}
        </p>
      </HoverCardContent>
    </HoverCard>
  );
}

function StatusRow({
  label,
  value,
}: {
  label: string;
  value: string;
}): React.ReactElement {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-outline">{label}</span>
      <span className="text-right font-medium text-on-surface">{value}</span>
    </div>
  );
}

export function getSyncStatusDetails({
  statusLabel,
  syncStatus,
  isOffline,
  lastSyncAt,
  autoSync,
  syncIntervalSeconds,
  nowMs,
}: Omit<SyncStatusHoverCardProps, 'children'> & {
  nowMs: number;
}): SyncStatusDetails {
  const interval = autoSync
    ? `Every ${formatInterval(syncIntervalSeconds)}`
    : 'Disabled';
  const lastSync = formatLastSync(lastSyncAt);

  if (!autoSync) {
    return {
      summary: `${statusLabel} // Auto-sync disabled`,
      nextRefresh: 'Disabled',
      nextRefreshDetail: 'Auto-sync is off.',
      interval,
      lastSync,
    };
  }

  if (isOffline) {
    return {
      summary: `${statusLabel} // Paused while offline`,
      nextRefresh: 'Paused',
      nextRefreshDetail: 'You are offline.',
      interval,
      lastSync,
    };
  }

  if (syncStatus === 'syncing') {
    return {
      summary: `${statusLabel} // Syncing now`,
      nextRefresh: 'Syncing now',
      nextRefreshDetail: 'Sync in progress.',
      interval,
      lastSync,
    };
  }

  const nextSyncAt = getNextSyncAt(lastSyncAt, syncIntervalSeconds);
  if (nextSyncAt === null) {
    return {
      summary: `${statusLabel} // Next refresh pending`,
      nextRefresh: 'Pending',
      nextRefreshDetail: 'Waiting for first sync.',
      interval,
      lastSync,
    };
  }

  const remainingMs = nextSyncAt.getTime() - nowMs;
  const nextRefresh =
    remainingMs <= 0 ? 'Due now' : `in ${formatCountdown(remainingMs)}`;

  return {
    summary: `${statusLabel} // Next refresh ${nextRefresh}`,
    nextRefresh,
    nextRefreshDetail:
      remainingMs <= 0
        ? 'Waiting to refresh.'
        : 'Countdown until next refresh.',
    interval,
    lastSync,
  };
}

function getNextSyncAt(
  lastSyncAt: string | null,
  syncIntervalSeconds: number,
): Date | null {
  if (
    lastSyncAt === null ||
    !Number.isFinite(syncIntervalSeconds) ||
    syncIntervalSeconds <= 0
  ) {
    return null;
  }

  const lastSyncMs = Date.parse(lastSyncAt);
  if (Number.isNaN(lastSyncMs)) return null;

  return new Date(lastSyncMs + syncIntervalSeconds * 1000);
}

function formatCountdown(remainingMs: number): string {
  if (remainingMs < 60_000) {
    const seconds = Math.max(1, Math.ceil(remainingMs / 1000));
    return `${seconds} ${seconds === 1 ? 'sec' : 'secs'}`;
  }

  const minutes = Math.ceil(remainingMs / 60_000);
  return `${minutes} ${minutes === 1 ? 'min' : 'mins'}`;
}

function formatInterval(syncIntervalSeconds: number): string {
  if (!Number.isFinite(syncIntervalSeconds) || syncIntervalSeconds <= 0) {
    return 'unknown';
  }

  if (syncIntervalSeconds < 60) {
    return `${syncIntervalSeconds} ${
      syncIntervalSeconds === 1 ? 'sec' : 'secs'
    }`;
  }

  const minutes = Math.floor(syncIntervalSeconds / 60);
  const seconds = syncIntervalSeconds % 60;
  const minuteLabel = `${minutes} ${minutes === 1 ? 'min' : 'mins'}`;
  if (seconds === 0) return minuteLabel;

  return `${minuteLabel} ${seconds} ${seconds === 1 ? 'sec' : 'secs'}`;
}

function formatLastSync(lastSyncAt: string | null): string {
  if (lastSyncAt === null) return 'Not yet';

  const lastSyncMs = Date.parse(lastSyncAt);
  if (Number.isNaN(lastSyncMs)) return 'Unknown';

  return new Date(lastSyncMs).toLocaleTimeString([], {
    hour: 'numeric',
    minute: '2-digit',
  });
}
