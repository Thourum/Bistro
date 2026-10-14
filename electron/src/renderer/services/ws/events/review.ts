import { Fault, fromThrowable, type Result } from '@thourum/fault';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import { createLogger } from '../../../utils/logger';

const log = createLogger('WebSocket');

export function handlePRApproved({
  data,
}: Readonly<WSInboundMessage<'PR_APPROVED'>>): void {
  appStore.setState((state) => ({ ...state, lastSyncAt: data.timestamp }));
}

/** Invalidates open threads and merge readiness without a full sync cycle. */
export function handleThreadStateChanged(): void {
  appStore.setState((state) => ({
    ...state,
    threadsRevision: state.threadsRevision + 1,
  }));
}

function bumpPRListRevision(): Result<void, Fault> {
  return fromThrowable(
    () => {
      appStore.setState((state) => ({
        ...state,
        prListRevision: state.prListRevision + 1,
      }));
    },
    (cause) =>
      new Fault(cause instanceof Error ? cause : 'Unknown error')
        .withTag('INTERNAL_ERROR')
        .withCause(cause),
  )();
}

export function handlePRListChanged(
  message: Readonly<
    WSInboundMessage<'PR_METADATA_CHANGED' | 'PRS_READ_CHANGED'>
  >,
): void {
  bumpPRListRevision().orInspect((fault) =>
    log.error(
      { type: message.type, id: message.id, message: fault.message },
      'failed to invalidate PR list',
    ),
  );
}

export function handleCIStatusChanged(): void {
  appStore.setState((state) => ({
    ...state,
    prListRevision: state.prListRevision + 1,
  }));
}
