import type {
  ExpertiseError,
  ExpertiseNudge,
  ExpertiseProgress,
  ExpertiseReady,
} from '@bindings/services/models';
import { appStore } from '../../store';
import type { ExpertiseRepoStatus } from '../../types';
import { addToast } from '../toasts';

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

export function handleExpertiseProgress(
  data: Readonly<ExpertiseProgress>,
): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'indexing',
    files_done: data.files_done,
    files_total: data.files_total,
  });
}

export function handleExpertiseReady(data: Readonly<ExpertiseReady>): void {
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

export function handleExpertiseError(data: Readonly<ExpertiseError>): void {
  const message = data.message || data.code;
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'error',
    code: data.code,
    message,
  });
  addToast({
    type: 'error',
    message: `Expertise indexing failed: ${message}`,
    dismissible: true,
  });
}

export function handleExpertiseNudge(data: Readonly<ExpertiseNudge>): void {
  appStore.setState((state) => ({
    ...state,
    expertiseNudges: { ...state.expertiseNudges, [data.repo_id]: data },
  }));
}
