import { describe, expect, it } from 'vitest';
import { getSyncStatusDetails } from '../../src/components/SyncStatusHoverCard';

const baseStatus = {
  statusLabel: 'OPERATIONAL',
  syncStatus: 'ready' as const,
  isOffline: false,
  lastSyncAt: '2026-06-12T15:58:00.000Z',
  autoSync: true,
  syncIntervalSeconds: 15 * 60,
};

describe('getSyncStatusDetails', () => {
  it('shows minutes until the next refresh when at least one minute remains', () => {
    expect(
      getSyncStatusDetails({
        ...baseStatus,
        nowMs: Date.parse('2026-06-12T15:58:00.000Z'),
      }).nextRefresh,
    ).toBe('in 15 mins');

    expect(
      getSyncStatusDetails({
        ...baseStatus,
        nowMs: Date.parse('2026-06-12T16:10:00.000Z'),
      }).nextRefresh,
    ).toBe('in 3 mins');
  });

  it('switches to seconds when less than one minute remains', () => {
    expect(
      getSyncStatusDetails({
        ...baseStatus,
        nowMs: Date.parse('2026-06-12T16:12:30.000Z'),
      }).nextRefresh,
    ).toBe('in 30 secs');
  });
});
