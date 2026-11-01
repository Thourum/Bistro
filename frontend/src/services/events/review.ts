import type {
  CIStatusChanged,
  DraftsChanged,
  PRApproved,
  PRMetadataChanged,
  PRsReadChanged,
  ThreadStateChanged,
} from '@bindings/services/models';
import { appStore } from '../../store';
import { createLogger } from '../../utils/logger';
import { loadDrafts } from '../reviewDrafts';

const log = createLogger('EngineEvents');

export function handlePRApproved(_data: Readonly<PRApproved>): void {
  appStore.setState((state) => ({
    ...state,
    lastSyncAt: new Date().toISOString(),
  }));
}

export function handleThreadStateChanged(
  _data: Readonly<ThreadStateChanged>,
): void {
  appStore.setState((state) => ({
    ...state,
    threadsRevision: state.threadsRevision + 1,
  }));
}

export function handlePRListChanged(
  _data: Readonly<PRMetadataChanged | PRsReadChanged | CIStatusChanged>,
): void {
  appStore.setState((state) => ({
    ...state,
    prListRevision: state.prListRevision + 1,
  }));
}

export function handleDraftsChanged(data: Readonly<DraftsChanged>): void {
  if (appStore.state.reviewDraftsPrId !== data.pr_id) return;
  void loadDrafts(data.pr_id).mapErr((fault) =>
    log.warn({ err: fault.message }, 'failed to reload review drafts'),
  );
}
