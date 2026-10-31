import { Expertise } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { appStore } from '../store';
import { callWrite, faultReason } from './engine/call';
import { handleExpertiseError } from './events/expertise';

function reportExpertiseError(repoId: string, fault: Fault): void {
  handleExpertiseError({
    repo_id: repoId,
    code: faultReason(fault) ?? fault.tag ?? 'internal',
    message: fault.message,
  });
}

function clearExpertiseNudge(repoId: string): void {
  appStore.setState((state) => {
    const { [repoId]: _removed, ...expertiseNudges } =
      state.expertiseNudges ?? {};
    return { ...state, expertiseNudges };
  });
}

/** Enables indexing and clears the acknowledged nudge. */
export function enableExpertise(repoId: string): ResultAsync<void, Fault> {
  return callWrite(() => Expertise.Enable({ repo_id: repoId }))
    .map(() => clearExpertiseNudge(repoId))
    .orInspect((fault) => reportExpertiseError(repoId, fault));
}

/** Disables indexing and reports service errors. */
export function disableExpertise(repoId: string): ResultAsync<void, Fault> {
  return callWrite(() => Expertise.Disable({ repo_id: repoId }))
    .map(() => undefined)
    .orInspect((fault) => reportExpertiseError(repoId, fault));
}

/** Dismisses the nudge only after the engine acknowledges it. */
export function dismissExpertiseNudge(
  repoId: string,
): ResultAsync<void, Fault> {
  return callWrite(() => Expertise.DismissNudge({ repo_id: repoId }))
    .map(() => clearExpertiseNudge(repoId))
    .orInspect((fault) => reportExpertiseError(repoId, fault));
}
