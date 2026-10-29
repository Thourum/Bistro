import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { ExpertiseRepoStatus } from '../../../types';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import type { WsEventContext } from './context';

function mergeExpertiseStatus(status: Readonly<ExpertiseRepoStatus>): void {
  appStore.setState((state) => ({
    ...state,
    expertiseRepoStatuses: {
      ...state.expertiseRepoStatuses,
      [status.repo_id]: {
        ...(state.expertiseRepoStatuses[status.repo_id] ?? {}),
        ...status,
      },
    },
  }));
}

export function handleExpertiseIndexProgress({
  data,
}: Readonly<WSInboundMessage<'EXPERTISE_INDEX_PROGRESS'>>): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'indexing',
    files_done: data.files_done,
    files_total: data.files_total,
  });
}

export function handleExpertiseReady({
  data,
}: Readonly<WSInboundMessage<'EXPERTISE_READY'>>): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'ready',
    indexed_sha: data.indexed_sha,
    files_total: data.files_total,
    skipped_files: data.skipped_files,
  });
  appStore.setState((state) => ({
    ...state,
    prListRevision: state.prListRevision + 1,
  }));
}

export function handleExpertiseError(
  message: Readonly<WSInboundMessage<'EXPERTISE_ERROR'>>,
  context: WsEventContext,
): void {
  const { data } = message;
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'error',
    code: data.code,
    message: data.message,
  });
  addToast({
    type: 'error',
    message: `Expertise indexing failed: ${data.message}`,
    dismissible: true,
  });
  context.resolveResponse(message.id, { error: data.message || data.code });
}

export function handleExpertiseNudge({
  data,
}: Readonly<WSInboundMessage<'EXPERTISE_NUDGE'>>): void {
  appStore.setState((state) => ({
    ...state,
    expertiseNudges: { ...state.expertiseNudges, [data.repo_id]: data },
  }));
}

export function handleExpertiseSettings(
  message: Readonly<
    WSInboundMessage<'EXPERTISE_SETTINGS' | 'EXPERTISE_SETTINGS_UPDATED'>
  >,
  context: WsEventContext,
): void {
  appStore.setState((state) => ({ ...state, expertiseSettings: message.data }));
  context.resolveResponse(message.id, message.data);
}
